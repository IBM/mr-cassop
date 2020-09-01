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
	"fmt"
	"github.com/go-logr/logr"
	"github.com/gocql/gocql"
	config2 "github.com/ibm/cassandra-operator/controllers/config"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
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
	Log        logr.Logger
	Scheme     *runtime.Scheme
	Config     *config2.Config
	Clientset  *kubernetes.Clientset
	RESTConfig *rest.Config
}

// +kubebuilder:rbac:groups=db.ibm.com,resources=cassandraclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=db.ibm.com,resources=cassandraclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;update

func (r *CassandraClusterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	_ = r.Log.WithValues("cassandracluster", req.NamespacedName)

	cc := &dbv1alpha1.CassandraCluster{}
	err := r.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, cc)
	if err != nil {
		r.Log.V(1).Info("Can't get cassandracluster")
		return ctrl.Result{}, err
	}

	readyAllDCs, err := r.proberReadyAllDCs(ctx, cc)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !readyAllDCs {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	r.Log.V(1).Info("All DCs are ready")

	cassCfg := gocql.NewCluster("127.0.0.1")
	cassCfg.Authenticator = &gocql.PasswordAuthenticator{
		Username: "cassandra",
		Password: "cassandra",
	}
	cassCfg.Timeout = 6 * time.Second
	cassCfg.ConnectTimeout = 6 * time.Second
	cassCfg.ProtoVersion = 4
	cassCfg.Consistency = gocql.LocalQuorum

	//cassSession, err := cassCfg.CreateSession()
	cassSession, err := newCassandraSession(cc)
	if err != nil {
		r.Log.V(1).Info(fmt.Sprintf("Can't create session. Error: %s", err.Error()))
		return ctrl.Result{}, err
	}
	queryDCs := ""
	for _, dc := range cc.Spec.SystemKeyspaces.DCs {
		if queryDCs != "" {
			queryDCs = queryDCs + ","
		}
		queryDCs = queryDCs + fmt.Sprintf("'%s': %d", dc.Name, dc.RF)
	}
	query := fmt.Sprintf("ALTER KEYSPACE system_auth WITH replication = { 'class': 'NetworkTopologyStrategy' , %s  } ;", queryDCs)
	r.Log.V(1).Info(fmt.Sprintf("QUERY: %s", query))
	if err = cassSession.Query(query).Exec(); err != nil {
		r.Log.V(1).Info(fmt.Sprintf("ALTER KEYSPACE query failed. Error: %s", err.Error()))
		return ctrl.Result{}, err
	}

	cmd := []string{
		"sh",
		"-c",
		"nodetool -h c-tomash-cassandra-dc1-0 repair -full -dcpar -- system_auth",
	}
	err = r.execOnPod(cc, cmd)
	if err != nil {
		return ctrl.Result{}, err
	}

	cmList := &v1.ConfigMapList{}
	err = r.List(ctx, cmList, client.HasLabels{cc.Spec.CQLConfigMapLabelKey}, client.InNamespace(cc.Namespace))
	if err != nil {
		r.Log.V(1).Info(fmt.Sprintf("Can't get list of config maps.\n err: %s\n", err))
	}

	if len(cmList.Items) == 0 {
		r.Log.V(1).Info(fmt.Sprintf("No configmaps found with label %q", cc.Spec.CQLConfigMapLabelKey))

	}

	for cmKey, cm := range cmList.Items {
		lastChecksum := cm.Annotations["cql-checksum"]
		checksum := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%v", cm.Data)))
		if lastChecksum != checksum {
			for queryKey, cqlQuery := range cm.Data {
				r.Log.V(1).Info(fmt.Sprintf("Executing query with queryKey %q", queryKey))
				if err = cassSession.Query(cqlQuery).Exec(); err != nil {
					r.Log.V(1).Info(fmt.Sprintf("Query with queryKey %q failed. Err: %s", queryKey, err.Error()))
					return ctrl.Result{}, nil
				}
			}
		}

		cmd := []string{
			"sh",
			"-c",
			"nodetool -h c-tomash-cassandra-dc1-0 repair -full -dcpar -- reaper_db",
		}

		if err = r.execOnPod(cc, cmd); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.Update(ctx, &cm); err != nil {
			r.Log.V(1).Info(fmt.Sprintf("Can't update CM %q. Err: %s", cmKey, err.Error()))
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *CassandraClusterReconciler) proberReadyAllDCs(ctx context.Context, cc *dbv1alpha1.CassandraCluster) (bool, error) {
	proberReq, err := http.NewRequest(http.MethodGet, cc.Spec.ProberHost+"/readyalldcs", nil)
	if err != nil {
		r.Log.V(1).Info("Can't create request" + err.Error())
		return false, err
	}

	resp, err := http.DefaultClient.Do(proberReq)
	if err != nil {
		r.Log.V(1).Info("Request to prober failed" + err.Error())
		return false, err
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		r.Log.V(1).Info("Can't read body" + err.Error())
		return false, err
	}

	if resp.StatusCode != http.StatusOK {
		r.Log.V(1).Info(fmt.Sprintf("Not all DCs are ready. Prober response. Status: %d, body: %s", resp.StatusCode, string(b)))
		r.Log.V(1).Info("Not all DCs are ready. Trying again in 10 seconds...")
		return false, nil
	}

	return true, nil
}

func (r *CassandraClusterReconciler) execOnPod(cc *dbv1alpha1.CassandraCluster, cmd []string) error {
	nodetoolReq := r.Clientset.CoreV1().RESTClient().Post().Resource("pods").Name("c-tomash-cassandra-dc1-0").
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
		r.Log.V(1).Info(fmt.Sprintf("Exec failed.\n stdout: %s\n, stderr: %s\n", stdout.String(), stderr.String()))

		return err
	}

	r.Log.V(1).Info(fmt.Sprintf("Exec out.\n stdout: %s\n", stdout.String()))
	return err
}

func newCassandraSession(cluster *dbv1alpha1.CassandraCluster) (*gocql.Session, error) {
	cassCfg := gocql.NewCluster(cluster.Name + "_" + cluster.Spec.DCs[0].Name)
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

func (r *CassandraClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1alpha1.CassandraCluster{}).
		Complete(r)
}
