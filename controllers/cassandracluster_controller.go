/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gocql/gocql"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"net/http"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
)

// CassandraClusterReconciler reconciles a CassandraCluster object
type CassandraClusterReconciler struct {
	client.Client
	Log        *zap.SugaredLogger
	Scheme     *runtime.Scheme
	Clientset  *kubernetes.Clientset
	RESTConfig *rest.Config
}

// +kubebuilder:rbac:groups=db.ibm.com,resources=cassandraclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=db.ibm.com,resources=cassandraclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=create

func (r *CassandraClusterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()

	res, err := r.reconcileWithContext(ctx, req)
	if err != nil {
		if statusErr, ok := errors.Cause(err).(*apierrors.StatusError); ok && statusErr.ErrStatus.Reason == metav1.StatusReasonConflict {
			r.Log.Info("Conflict occurred. Retrying...", zap.Error(err))
			return ctrl.Result{Requeue: true}, nil //retry but do not treat conflicts as errors
		}

		r.Log.Errorf("%+v", err)
		return ctrl.Result{}, err
	}

	return res, nil
}

func (r *CassandraClusterReconciler) reconcileWithContext(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cc := &dbv1alpha1.CassandraCluster{}
	err := r.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, cc)
	if err != nil {
		if apierrors.IsNotFound(err) { //do not react to CRD delete events
			return ctrl.Result{}, nil
		}
		r.Log.With(zap.Error(err)).Error("Can't get cassandracluster")
		return ctrl.Result{}, err
	}

	readyAllDCs, err := r.proberReadyAllDCs(ctx, cc)
	if err != nil {
		r.Log.Warnf("Prober request failed: %s. Trying again in 5 seconds...", err.Error())
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if !readyAllDCs {
		r.Log.Info("Not all DCs are ready. Trying again in 10 seconds...")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	r.Log.Debug("All DCs are ready")

	cassSession, err := newCassandraSession(cc)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Can't create cassandra session")
	}

	r.Log.Debug("Executing query to update RF info")
	if err = cassSession.Query(updateRFQuery(cc)).Exec(); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Can't update RF info")
	}

	cmd := []string{
		"sh",
		"-c",
		"nodetool -h " + getFirstPodName(cc) + " repair -full -dcpar -- system_auth",
	}

	r.Log.Debug("Repairing system_auth keyspace")
	err = r.execOnPod(cc, cmd)
	if err != nil {
		return ctrl.Result{}, err
	}

	if cc.Spec.InternalAuth && cc.Spec.KwatcherEnabled {
		r.Log.Debug("Internal auth and kwatcher are enabled. Checking if all users are created.")
		created, err := r.usersCreated(ctx, cc, cassSession)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "Can't get created users info")
		}

		if !created {
			r.Log.Info("Users hasn't been created yet. Checking again in 10 seconds...")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		r.Log.Info("Users has been created")
	}

	r.Log.Debug("Executing queries from CQL ConfigMaps")
	if err = r.reconcileCQLConfigMaps(ctx, cc, cassSession); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile CQL config maps")
	}

	return ctrl.Result{}, nil
}

func getFirstPodName(cc *dbv1alpha1.CassandraCluster) string {
	return fmt.Sprintf("%s-cassandra-%s-0", cc.Name, cc.Spec.DCs[0].Name)
}

func (r *CassandraClusterReconciler) reconcileCQLConfigMaps(ctx context.Context, cc *dbv1alpha1.CassandraCluster, cassSession *gocql.Session) error {
	cmList := &v1.ConfigMapList{}
	err := r.List(ctx, cmList, client.HasLabels{cc.Spec.CQLConfigMapLabelKey}, client.InNamespace(cc.Namespace))
	if err != nil {
		return errors.Wrap(err, "Can't get list of config maps")
	}

	if len(cmList.Items) == 0 {
		r.Log.Debug(fmt.Sprintf("No configmaps found with label %q", cc.Spec.CQLConfigMapLabelKey))
		return nil
	}

	for _, cm := range cmList.Items {
		lastChecksum := cm.Annotations["cql-checksum"]
		checksum := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%v", cm.Data)))
		if lastChecksum == checksum {
			continue
		}

		for queryKey, cqlQuery := range cm.Data {
			r.Log.Debugf("Executing query with queryKey %q from ConfigMap %q", queryKey, cm.Name)
			if err = cassSession.Query(cqlQuery).Exec(); err != nil {
				return errors.Wrapf(err, "Query with queryKey %q failed", queryKey)
			}
		}

		keyspaceToRepair := cm.Annotations["cql-repairKeyspace"]
		if len(keyspaceToRepair) > 0 {
			cmd := []string{
				"sh",
				"-c",
				"nodetool -h " + getFirstPodName(cc) + " repair -full -dcpar -- reaper_db",
			}

			r.Log.Debugf("Repairing %q keyspace", keyspaceToRepair)
			if err = r.execOnPod(cc, cmd); err != nil {
				return errors.Wrapf(err, "Failed to repair 'reaper_db' keyspace")
			}
		} else {
			r.Log.Warnf("Keyspace for ConfigMap %q is not set. Skipping repair.", cm.Name)
		}

		r.Log.Debugf("Updating checksum for ConfigMap %q", cm.Name)
		if err := r.Update(ctx, &cm); err != nil {
			return errors.Wrapf(err, "Failed to update CM %q", cm.Name)
		}
	}

	return nil
}

func (r *CassandraClusterReconciler) proberReadyAllDCs(ctx context.Context, cc *dbv1alpha1.CassandraCluster) (bool, error) {
	proberReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+cc.Spec.ProberHost+"/readyalldcs", nil)
	if err != nil {
		return false, errors.Wrap(err, "Can't create request")
	}

	resp, err := http.DefaultClient.Do(proberReq)
	if err != nil {
		return false, errors.Wrap(err, "Request to prober failed")
	}

	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, errors.Wrap(err, "Can't read body")
	}

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	return true, nil
}

func (r *CassandraClusterReconciler) execOnPod(cc *dbv1alpha1.CassandraCluster, cmd []string) error {
	nodetoolReq := r.Clientset.CoreV1().RESTClient().Post().Resource("pods").Name(getFirstPodName(cc)).
		Namespace(cc.Namespace).SubResource("exec")
	option := &v1.PodExecOptions{
		Command: cmd,
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
		TTY:     false,
	}

	nodetoolReq.VersionedParams(
		option,
		scheme.ParameterCodec,
	)

	exec, err := remotecommand.NewSPDYExecutor(r.RESTConfig, "POST", nodetoolReq.URL())
	if err != nil {
		return err
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	})
	if err != nil {
		return errors.Wrapf(err, "Exec failed.\n stdout: %s\n, stderr: %s\n", stdout, stderr)
	}

	return err
}

func newCassandraSession(cluster *dbv1alpha1.CassandraCluster) (*gocql.Session, error) {
	cassCfg := gocql.NewCluster(cluster.Name + "-cassandra-" + cluster.Spec.DCs[0].Name)
	cassCfg.Authenticator = &gocql.PasswordAuthenticator{
		Username: cluster.Spec.CassandraUser,
		Password: cluster.Spec.CassandraPassword,
	}

	cassCfg.Timeout = 6 * time.Second
	cassCfg.ConnectTimeout = 6 * time.Second
	cassCfg.ProtoVersion = 4
	cassCfg.Consistency = gocql.LocalQuorum

	cassSession, err := cassCfg.CreateSession()
	return cassSession, err
}

func updateRFQuery(cc *dbv1alpha1.CassandraCluster) string {
	queryDCs := ""
	for _, dc := range cc.Spec.SystemKeyspaces.DCs {
		if queryDCs != "" {
			queryDCs = queryDCs + ","
		}
		queryDCs = queryDCs + fmt.Sprintf("'%s': %d", dc.Name, dc.RF)
	}
	return fmt.Sprintf("ALTER KEYSPACE system_auth WITH replication = { 'class': 'NetworkTopologyStrategy' , %s  } ;", queryDCs)
}

func (r *CassandraClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1alpha1.CassandraCluster{}).
		Complete(r)
}

func (r *CassandraClusterReconciler) usersCreated(ctx context.Context, cc *dbv1alpha1.CassandraCluster, cassSession *gocql.Session) (bool, error) {
	iter := cassSession.Query("SELECT role,is_superuser FROM system_auth.roles").Iter()
	type cassUser struct {
		role        string
		isSuperuser bool
	}
	cassUsers := make([]cassUser, 0, iter.NumRows())
	var role string
	var isSuperuser bool
	for iter.Scan(&role, &isSuperuser) {
		cassUsers = append(cassUsers, cassUser{role: role, isSuperuser: isSuperuser})
	}

	if err := iter.Close(); err != nil {
		return false, errors.Wrapf(err, "Can't close iterator: %s")
	}

	usersSecret := &v1.Secret{}
	usersSecretName := types.NamespacedName{
		Namespace: cc.Namespace,
		Name:      cc.Name + "-users-secret",
	}

	err := r.Get(ctx, usersSecretName, usersSecret)
	if err != nil {
		return false, errors.Wrapf(err, "Can't get users secret")
	}

	type user struct {
		NodetoolUser bool   `json:"nodetoolUser"`
		Password     string `json:"password"`
		Super        bool   `json:"super"`
		Username     string `json:"username"`
	}

	usersFromSecret := make([]user, 0, len(usersSecret.Data))
	for _, userFromSecretData := range usersSecret.Data {
		user := user{}
		err := json.Unmarshal(userFromSecretData, &user)
		if err != nil {
			return false, errors.Wrapf(err, "Can't unmarshal user data")
		}
		usersFromSecret = append(usersFromSecret, user)
	}

	if len(usersFromSecret) != len(cassUsers) {
		return false, nil
	}

	for _, userFromSecret := range usersFromSecret {
		found := false
		for _, cassandraUser := range cassUsers {
			if userFromSecret.Username == cassandraUser.role {
				found = true
				break
			}
		}
		if !found {
			r.Log.Debugf("Didn't find user %s", userFromSecret.Username)
			return false, nil
		}
	}

	return true, nil
}
