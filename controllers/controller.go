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

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/config"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/eventhandler"
	"github.com/ibm/cassandra-operator/controllers/events"
	"github.com/ibm/cassandra-operator/controllers/jobs"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/nodectl"
	"github.com/ibm/cassandra-operator/controllers/prober"
	"github.com/ibm/cassandra-operator/controllers/reaper"

	"github.com/gocql/gocql"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	maintenanceDir               = "/etc/maintenance"
	cassandraCommitLogDir        = "/var/lib/cassandra-commitlog"
	cassandraServerTLSDir        = "/etc/cassandra-server-tls"
	cassandraServerTLSVolumeName = "server-keystore"
	cassandraClientTLSDir        = "/etc/cassandra-client-tls"
	cassandraClientTLSVolumeName = "client-keystore"
	defaultCQLConfigMapLabelKey  = "cql-scripts"
	retryAttempts                = 3
	initialRetryDelaySeconds     = 5

	repairCauseKeyspacesInit = "keyspaces-init"
	repairCauseCQLConfigMap  = "cql-configmap"
	repairCauseReaperInit    = "reaper-init"

	jmxAuthenticationInternal   = "internal"
	jmxAuthenticationLocalFiles = "local_files"
)

// CassandraClusterReconciler reconciles a CassandraCluster object
type CassandraClusterReconciler struct {
	client.Client
	Log           *zap.SugaredLogger
	Scheme        *runtime.Scheme
	Cfg           config.Config
	Events        *events.EventRecorder
	Jobs          *jobs.JobManager
	ProberClient  func(url *url.URL, user, password string) prober.ProberClient
	CqlClient     func(cluster *gocql.ClusterConfig) (cql.CqlClient, error)
	ReaperClient  func(url *url.URL, clusterName string, defaultRepairThreadCount int32) reaper.ReaperClient
	NodectlClient func(jolokiaAddr, jmxUser, jmxPassword string, logr *zap.SugaredLogger) nodectl.Nodectl
}

// +kubebuilder:rbac:groups=db.ibm.com,resources=cassandraclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=db.ibm.com,resources=cassandraclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups="",resources=configmaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;patch;create;update;deletecollection;
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get;patch;update
// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=list;get;watch;create;update;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses;networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;serviceaccounts,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch
// +kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=list;watch;get;create;update;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=get;create;update;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=list;watch;get;create;update;delete;patch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=list;watch;get;create;update;delete

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

	if err = r.cleanupNetworkPolicies(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to cleanup network policies")
	}

	clusterReady := false
	ccStatus := cc.DeepCopy()
	defer func() {
		if ccStatus.Status.Ready != clusterReady {
			ccStatus.Status.Ready = clusterReady
			statusErr := r.Status().Update(ctx, ccStatus)
			if statusErr != nil {
				r.Log.Errorf("Failed to update cluster readiness state: %#v", statusErr)
			}
		}
	}()
	err = r.reconcileCassandraRBAC(ctx, cc)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err = r.reconcileTLSSecrets(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling TLS Secrets")
	}

	defer r.cleanupClientTLSDir(cc)

	baseAdminSecret := &v1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: cc.Spec.AdminRoleSecretName, Namespace: cc.Namespace}, baseAdminSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			errMsg := fmt.Sprintf("admin secret %s not found", cc.Spec.AdminRoleSecretName)
			r.Log.Warn(errMsg)
			r.Events.Warning(cc, events.EventAdminRoleSecretNotFound, errMsg)
			return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
		}
		return ctrl.Result{}, errors.Wrapf(err, "Failed to get secret: %s", names.ActiveAdminSecret(cc.Name))
	}

	err = r.reconcileAnnotations(ctx, baseAdminSecret, map[string]string{v1alpha1.CassandraClusterInstance: cc.Name})
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to reconcile annotations")
	}

	desiredAdminRole, desiredAdminPassword, err := extractCredentials(baseAdminSecret)
	if err != nil {
		errMsg := fmt.Sprintf("admin secret %q is invalid: %s", cc.Spec.AdminRoleSecretName, err.Error())
		r.Log.Warn(errMsg)
		r.Events.Warning(cc, events.EventAdminRoleSecretInvalid, errMsg)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}

	auth, err := r.reconcileAdminAuth(ctx, cc, desiredAdminRole, desiredAdminPassword)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling Admin Auth Secrets")
	}

	if err = r.reconcileMaintenanceConfigMap(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling maintenance configmap")
	}

	if err = r.reconcilePrometheusConfigMap(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling prometheus configmap")
	}

	if err = r.reconcileCollectdConfigMap(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile collectd configmap")
	}

	if err = r.reconcileCassandraServiceMonitor(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile service monitor")
	}

	if err = r.reconcileProber(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling prober")
	}

	proberClient := r.ProberClient(proberURL(cc), auth.desiredRole, auth.desiredPassword)

	proberReady, err := proberClient.Ready(ctx)
	if err != nil {
		r.Log.Warnf("Prober ping request failed: %s. Trying again in %s...", err.Error(), r.Cfg.RetryDelay)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}

	if !proberReady {
		r.Log.Warnf("Prober is not ready. Err: %#v. Trying again in %s...", err, r.Cfg.RetryDelay)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}

	err = proberClient.UpdateDCs(ctx, cc.Spec.DCs)
	if err != nil {
		return ctrl.Result{}, err
	}

	restartChecksum := checksumContainer{}
	if err = r.reconcileCassandraConfigMap(ctx, cc, restartChecksum); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling cassandra configmaps")
	}

	podList, err := r.getCassandraPods(ctx, cc)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Cannot get Cassandra pods list")
	}

	nodeList := &v1.NodeList{}
	//optimization - node info needed only if hostport or zoneAsRacks enabled
	if cc.Spec.HostPort.Enabled || cc.Spec.Cassandra.ZonesAsRacks {
		err = r.List(ctx, nodeList)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "can't get list of nodes")
		}
	}

	// although the pods don't exist yet on the first run, we still need to create the configmap (even empty)
	// so that the pods won't fail trying to mount an empty configmap
	if err = r.reconcileCassandraPodsConfigMap(ctx, cc, podList, nodeList, proberClient); err != nil {
		if errors.Cause(err) == ErrPodNotScheduled || errors.Cause(err) == errors.Cause(ErrRegionNotReady) {
			r.Log.Warnf("%s. Trying again in %s...", err.Error(), r.Cfg.RetryDelay)
			return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
		}
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling cassandra pods configmap")
	}

	if err = r.reconcileCassandra(ctx, cc, restartChecksum); err != nil {
		if errors.Cause(err) == errTLSSecretNotFound || errors.Cause(err) == errTLSSecretInvalid {
			return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
		}
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling statefulsets")
	}

	allDCs, err := r.getAllDCs(ctx, cc, proberClient)
	if err != nil {
		if errors.Cause(err) == ErrRegionNotReady {
			r.Log.Warnf("%s. Trying again in %s...", err.Error(), r.Cfg.RetryDelay)
			return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
		}
		return ctrl.Result{}, errors.Wrap(err, "can't get all dcs across regions")
	}

	scalingInProgress, err := r.reconcileCassandraScaling(ctx, cc, podList, nodeList, allDCs, baseAdminSecret)
	if err != nil {
		if errors.Cause(err) == errDCDecommissionBlocked {
			return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
		}
		return ctrl.Result{}, err
	}

	if scalingInProgress {
		r.Log.Info("Scaling in progress, not proceeding")
		return ctrl.Result{}, nil
	}

	if err = r.reconcileMaintenance(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile maintenance")
	}

	// Try to reconcile `system_auth` keyspace with current DCs config. Needed on new region init.
	// The existing ready region should set the replication settings correctly for the new region to come up.
	// We need to do it before the check if the whole cluster is ready because it never will be unless we
	// set the correct replication settings and run a repair.
	if len(cc.Spec.ExternalRegions.Managed) > 0 {
		err = r.reconcileSystemAuthIfReady(ctx, cc, allDCs)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	clusterReady, err = r.clusterReady(ctx, cc, proberClient)
	if err != nil {
		clusterReady = false
		return ctrl.Result{}, err
	}

	if !clusterReady {
		r.Log.Infof("Cluster not ready. Trying again in %s...", r.Cfg.RetryDelay)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}

	cqlClient, err := r.reconcileAdminRole(ctx, cc, auth, allDCs)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile Admin Role")
	}
	defer cqlClient.CloseSession()

	err = r.reconcileRoles(ctx, cc, cqlClient)
	if err != nil {
		return ctrl.Result{}, err
	}

	waitForFirstRegionReaper, err := r.waitForFirstRegionReaper(ctx, cc, proberClient)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error checking for first region reaper readiness")
	}

	if waitForFirstRegionReaper {
		r.Log.Infof("Reaper is not ready in the first region. Trying again in %s...", r.Cfg.RetryDelay)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}

	if err := r.reconcileReaperKeyspace(cc, cqlClient, allDCs); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling reaper keyspace")
	}

	if res, err := r.reconcileReaper(ctx, cc, podList, nodeList); needsRequeue(res, err) {
		return res, err
	}

	reaperClient := r.ReaperClient(reaperServiceURL(cc), cc.Name, cc.Spec.Reaper.RepairThreadCount)
	isRunning, err := reaperClient.IsRunning(ctx)
	if err != nil {
		if updErr := proberClient.UpdateReaperStatus(ctx, false); updErr != nil {
			return ctrl.Result{}, updErr
		}
		r.Log.Warnf("Reaper ping request failed: %s. Trying again in %s...", err.Error(), r.Cfg.RetryDelay)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}
	if !isRunning {
		if err = proberClient.UpdateReaperStatus(ctx, false); err != nil {
			return ctrl.Result{}, err
		}
		r.Log.Infof("Reaper is not ready. Trying again in %s...", r.Cfg.RetryDelay)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}

	if err = r.reaperInitialization(ctx, cc, reaperClient); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to initialize reaper")
	}

	if err = proberClient.UpdateReaperStatus(ctx, true); err != nil {
		return ctrl.Result{}, err
	}

	if err = r.reconcileRepairSchedules(ctx, cc, reaperClient); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile repair schedules")
	}

	err = r.reconcileKeyspaces(ctx, cc, cqlClient, reaperClient, allDCs)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile keyspaces")
	}

	if err = r.reconcileCQLConfigMaps(ctx, cc, cqlClient, reaperClient); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile CQL configmaps")
	}

	if err = r.removeDefaultRoleIfExists(ctx, cc, cqlClient); err != nil {
		return ctrl.Result{}, err
	}

	err = r.reconcileNetworkPolicies(ctx, cc, proberClient, podList)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling network policies")
	}

	return ctrl.Result{RequeueAfter: time.Minute * 1}, nil
}

func needsRequeue(result ctrl.Result, err error) bool {
	return result.Requeue || result.RequeueAfter.Nanoseconds() > 0 || err != nil
}

func SetupCassandraReconciler(r reconcile.Reconciler, mgr manager.Manager, logr *zap.SugaredLogger, reconcileChan chan event.GenericEvent) error {
	builder := ctrl.NewControllerManagedBy(mgr).
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
		Watches(&source.Kind{Type: &v1.Secret{}}, eventhandler.NewAnnotationEventHandler()).
		Watches(&source.Kind{Type: &v1.ConfigMap{}}, eventhandler.NewAnnotationEventHandler()).
		Watches(&source.Channel{Source: reconcileChan}, &handler.EnqueueRequestForObject{})

	// WithEventFilter(predicate.NewPredicate(logr)) // uncomment to see kubernetes events in the logs, e.g. ConfigMap updates

	serviceMonitorList := &unstructured.UnstructuredList{}
	serviceMonitorList.SetGroupVersionKind(serviceMonitorListGVK)
	if err := mgr.GetClient().List(context.Background(), serviceMonitorList); err != nil {
		if _, ok := errors.Cause(err).(*meta.NoKindMatchError); ok {
			logr.Warn("Can't watch ServiceMonitor. ServiceMonitor Kind is not found. Prometheus operator needs to be installed first.", err)
		} else {
			logr.Errorf("Can't watch ServiceMonitor: %s", err)
			// Return is intentionally omitted. Better work without the watch than not at all
		}
	} else {
		serviceMonitor := &unstructured.Unstructured{}
		serviceMonitor.SetGroupVersionKind(serviceMonitorGVK)
		builder.Owns(serviceMonitor)
	}
	return builder.Complete(r)
}
