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
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/gocql/gocql"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/config"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/nodetool"
	"github.com/ibm/cassandra-operator/controllers/prober"
	"github.com/ibm/cassandra-operator/controllers/reaper"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	maintenanceDir              = "/etc/maintenance"
	cassandraCommitLogDir       = "/var/lib/cassandra-commitlog"
	defaultCQLConfigMapLabelKey = "cql-scripts"
	retryAttempts               = 3
	initialRetryDelaySeconds    = 5
)

// CassandraClusterReconciler reconciles a CassandraCluster object
type CassandraClusterReconciler struct {
	client.Client
	Log            *zap.SugaredLogger
	Scheme         *runtime.Scheme
	Cfg            config.Config
	Clientset      *kubernetes.Clientset
	RESTConfig     *rest.Config
	ProberClient   func(url *url.URL) prober.ProberClient
	NodetoolClient func(clientset *kubernetes.Clientset, config *rest.Config, roleName, password string) nodetool.NodetoolClient
	CqlClient      func(cluster *gocql.ClusterConfig) (cql.CqlClient, error)
	ReaperClient   func(url *url.URL) reaper.ReaperClient
}

// +kubebuilder:rbac:groups=db.ibm.com,resources=cassandraclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=db.ibm.com,resources=cassandraclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups="",resources=configmaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;patch;create;update;
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get;patch;update
// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=list;get;watch;create;update;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;serviceaccounts,verbs=list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=list;watch;get;create;update;delete

func (r *CassandraClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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
	cc := &v1alpha1.CassandraCluster{}
	err := r.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, cc)
	if err != nil {
		if apierrors.IsNotFound(err) { //do not react to CRD delete events
			return ctrl.Result{}, nil
		}
		r.Log.With(zap.Error(err)).Error("Can't get cassandracluster")
		return ctrl.Result{}, err
	}

	r.defaultCassandraCluster(cc)

	activeAdminSecret, err := r.reconcileAdminAuth(ctx, cc)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling Admin Auth Secrets")
	}

	if err := r.reconcileScriptsConfigMap(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling scripts configmap")
	}

	if err := r.reconcileMaintenanceConfigMap(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling maintenance configmap")
	}

	if err := r.reconcileProber(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling prober")
	}

	proberUrl, err := url.Parse(fmt.Sprintf("http://%s.%s.svc.cluster.local", names.ProberService(cc.Name), cc.Namespace))
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error parsing prober client url")
	}
	proberClient := r.ProberClient(proberUrl)

	proberReady, err := proberClient.Ready(ctx)
	if err != nil {
		r.Log.Warnf("Prober ping request failed: %s. Trying again in %s...", err.Error(), r.Cfg.RetryDelay)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}

	if !proberReady {
		r.Log.Warnf("Prober is not ready. Err: %#v. Trying again in %s...", err, r.Cfg.RetryDelay)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}

	if err := r.reconcileCassandraConfigMap(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling cassandra config configmaps")
	}

	if err := r.reconcileCassandra(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling statefulsets")
	}

	if err := r.reconcileCassandraPodsConfigMap(ctx, cc, proberClient); err != nil {
		if errors.Cause(err) == ErrPodNotScheduled || err == errors.Cause(ErrRegionNotReady) {
			r.Log.Warnf("%s. Trying again in %s...", err.Error(), r.Cfg.RetryDelay)
			return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
		}
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling Cassandra pods configmap")
	}

	if err := r.reconcileMaintenance(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile maintenance")
	}

	ready, err := r.clusterReady(ctx, cc, proberClient)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !ready {
		r.Log.Infof("Cluster not ready. Trying again in %s...", r.Cfg.RetryDelay)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}

	cqlClient, err := r.reconcileAdminRole(ctx, cc, activeAdminSecret)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile Admin Role")
	}
	defer cqlClient.CloseSession()

	// Get the up-to-date version of the secret as it could have changed during admin role reconcile
	err = r.Get(ctx, types.NamespacedName{Name: names.ActiveAdminSecret(cc.Name), Namespace: cc.Namespace}, activeAdminSecret)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Can't get secret "+names.ActiveAdminSecret(cc.Name))
	}

	ntClient := r.NodetoolClient(r.Clientset, r.RESTConfig, v1alpha1.CassandraOperatorAdminRole, string(activeAdminSecret.Data[v1alpha1.CassandraOperatorAdminRole]))

	err = r.reconcileKeyspaces(cc, cqlClient, ntClient)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile keyspaces")
	}

	// Todo: reimplement Roles logic in next PR
	//err = r.reconcileRoles(cqlClient)
	//if err != nil {
	//	return ctrl.Result{}, err
	//}

	if err = r.reconcileCQLConfigMaps(ctx, cc, cqlClient, ntClient, activeAdminSecret); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile CQL config maps")
	}

	if err = r.reconcileReaper(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling reaper")
	}

	for index, dc := range cc.Spec.DCs {
		if err = r.reconcileReaperDeployment(ctx, cc, dc, activeAdminSecret); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile reaper deployment")
		}

		if err := r.reconcileReaperService(ctx, cc); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile reaper service")
		}

		if index == 0 { // Wait for 1st reaper deployment to finish, otherwise we can get an error 'Schema migration is locked by another instance'
			reaperDeployment := &appsv1.Deployment{}
			err = r.Get(ctx, types.NamespacedName{Name: names.ReaperDeployment(cc.Name, dc.Name), Namespace: cc.Namespace}, reaperDeployment)
			if err != nil {
				return ctrl.Result{}, errors.Wrap(err, "Failed to get reaper deployment")
			}

			if reaperDeployment.Status.ReadyReplicas != v1alpha1.ReaperReplicasNumber {
				r.Log.Infof("Waiting for the first reaper deployment to be ready. Trying again in %s...", r.Cfg.RetryDelay)
				return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
			}
		}
	}

	reaperServiceUrl, err := url.Parse(fmt.Sprintf("http://%s.%s.svc.cluster.local:8080", names.ReaperService(cc.Name), cc.Namespace))
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error parsing reaper service url")
	}
	reaperClient := r.ReaperClient(reaperServiceUrl)
	isRunning, err := reaperClient.IsRunning(ctx)
	if err != nil {
		r.Log.Warnf("Reaper ping request failed: %s. Trying again in %s...", err.Error(), r.Cfg.RetryDelay)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}
	if !isRunning {
		r.Log.Infof("Reaper is not ready. Trying again in %s...", r.Cfg.RetryDelay)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}
	if err := r.reaperInitialization(ctx, cc, reaperClient); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to initialize reaper")
	}

	if cc.Spec.Reaper.ScheduleRepairs.Enabled {
		if err := r.reconcileScheduleRepairs(ctx, cc, reaperClient); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "Failed to schedule reaper reapairs")
		}
	}

	return ctrl.Result{}, nil
}

func (r *CassandraClusterReconciler) clusterReady(ctx context.Context, cc *v1alpha1.CassandraCluster, proberClient prober.ProberClient) (bool, error) {
	unreadyDCs, err := r.unreadyDCs(ctx, cc)
	if err != nil {
		return false, errors.Wrap(err, "Failed to check DCs readiness")
	}

	if len(unreadyDCs) > 0 {
		err := proberClient.UpdateDCStatus(ctx, false)
		if err != nil {
			return false, errors.Wrap(err, "Failed to set DC status to not ready for prober")
		}
		r.Log.Warnf("Not all DCs are ready: %q.", unreadyDCs)
		return false, nil
	}
	r.Log.Debug("All DCs are ready")
	err = proberClient.UpdateDCStatus(ctx, true)
	if err != nil {
		return false, errors.Wrap(err, "Failed to set DC's status to ready")
	}

	var unreadyRegions []string
	if unreadyRegions, err = r.unreadyRegions(ctx, cc, proberClient); err != nil {
		return false, errors.Wrap(err, "Failed to check region readiness")
	}

	if cc.Spec.HostPort.Enabled && len(unreadyRegions) != 0 {
		r.Log.Warnf("Not all regions are ready: %q.", unreadyDCs)
		return false, nil
	}

	return true, err
}

func (r *CassandraClusterReconciler) unreadyDCs(ctx context.Context, cc *v1alpha1.CassandraCluster) ([]string, error) {
	unreadyDCs := make([]string, 0)
	for _, dc := range cc.Spec.DCs {
		sts := &appsv1.StatefulSet{}
		if err := r.Get(ctx, types.NamespacedName{Name: names.DC(cc.Name, dc.Name), Namespace: cc.Namespace}, sts); err != nil {
			return nil, errors.Wrap(err, "failed to get statefulset: "+sts.Name)
		}

		if *dc.Replicas != sts.Status.ReadyReplicas {
			unreadyDCs = append(unreadyDCs, dc.Name)
		}
	}
	return unreadyDCs, nil
}

func (r *CassandraClusterReconciler) unreadyRegions(ctx context.Context, cc *v1alpha1.CassandraCluster, proberClient prober.ProberClient) ([]string, error) {
	unreadyRegions := make([]string, 0)
	for _, domain := range cc.Spec.Prober.ExternalDCsIngressDomains {
		proberHost := names.ProberIngressDomain(cc.Name, domain, cc.Namespace)
		dcsReady, err := proberClient.DCsReady(ctx, proberHost)
		if err != nil {
			r.Log.Warnf(fmt.Sprintf("Unable to get DC's status from prober %q. Err: %#v", proberHost, err))
			return nil, ErrRegionNotReady
		}

		if !dcsReady {
			unreadyRegions = append(unreadyRegions, proberHost)
		}
	}

	return unreadyRegions, nil
}

func newCassandraConfig(cc *v1alpha1.CassandraCluster, adminRole string, adminPwd string) *gocql.ClusterConfig {
	cassCfg := gocql.NewCluster(fmt.Sprintf("%s.%s.svc.cluster.local", names.DCService(cc.Name, cc.Spec.DCs[0].Name), cc.Namespace))
	cassCfg.Authenticator = &gocql.PasswordAuthenticator{
		Username: adminRole,
		Password: adminPwd,
	}

	cassCfg.Timeout = 6 * time.Second
	cassCfg.ConnectTimeout = 6 * time.Second
	cassCfg.ProtoVersion = 4
	cassCfg.Consistency = gocql.LocalQuorum
	cassCfg.ReconnectionPolicy = &gocql.ConstantReconnectionPolicy{
		MaxRetries: 3,
		Interval:   time.Second * 1,
	}

	return cassCfg
}

func (r *CassandraClusterReconciler) doWithRetry(retryFunc func() error) error {
	for currentAttempt := 1; currentAttempt <= retryAttempts; currentAttempt++ {
		err := retryFunc()
		if err != nil {
			delay := initialRetryDelaySeconds * time.Second * time.Duration(currentAttempt)
			r.Log.Warnf("Attempt %d of %d failed. Error: %s. Trying again in %s.", currentAttempt, retryAttempts, err.Error(), delay.String())
			time.Sleep(delay)
			continue
		}
		break
	}

	return nil
}

func SetupCassandraReconciler(r reconcile.Reconciler, mgr manager.Manager, logr *zap.SugaredLogger) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("cassandracluster").
		For(&v1alpha1.CassandraCluster{}).
		Owns(&appsv1.Deployment{}).
		Owns(&v1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&v1.Secret{}).
		Owns(&v1.ConfigMap{}).
		Owns(&rbac.Role{}).
		Owns(&rbac.RoleBinding{}).
		Owns(&v1.ServiceAccount{}).
		//WithEventFilter(predicate.NewPredicate(logr)). //uncomment to see kubernetes events in the logs, e.g. ConfigMap updates
		Complete(r)
}
