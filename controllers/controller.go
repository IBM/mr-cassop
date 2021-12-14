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
	"github.com/ibm/cassandra-operator/controllers/eventhandler"
	"github.com/ibm/cassandra-operator/controllers/events"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/prober"
	"github.com/ibm/cassandra-operator/controllers/reaper"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	internodeEncryptionNone     = "none"
	jmxAuthenticationInternal   = "internal"
	jmxAuthenticationLocalFiles = "local_files"

	tlsRsaAes128 = "TLS_RSA_WITH_AES_128_CBC_SHA"
	tlsRsaAes256 = "TLS_RSA_WITH_AES_256_CBC_SHA"
)

// CassandraClusterReconciler reconciles a CassandraCluster object
type CassandraClusterReconciler struct {
	client.Client
	Log          *zap.SugaredLogger
	Scheme       *runtime.Scheme
	Cfg          config.Config
	Events       *events.EventRecorder
	ProberClient func(url *url.URL) prober.ProberClient
	CqlClient    func(cluster *gocql.ClusterConfig) (cql.CqlClient, error)
	ReaperClient func(url *url.URL, clusterName string, defaultRepairThreadCount int32) reaper.ReaperClient
}

// +kubebuilder:rbac:groups=db.ibm.com,resources=cassandraclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=db.ibm.com,resources=cassandraclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups="",resources=configmaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;patch;create;update;
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get;patch;update
// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=list;get;watch;create;update;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;serviceaccounts,verbs=list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch
// +kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=list;watch;get;create;update;delete
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

	err = r.reconcileAdminAuth(ctx, cc)
	if err != nil {
		if errors.Cause(err) == errAdminSecretNotFound || errors.Cause(err) == errAdminSecretInvalid {
			r.Log.Warnf("Failed to reconcile admin auth with secret %q: %s", cc.Spec.AdminRoleSecretName, err.Error())
			return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
		}
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

	err = proberClient.UpdateDCs(ctx, cc.Spec.DCs)
	if err != nil {
		return ctrl.Result{}, err
	}

	restartChecksum := checksumContainer{}
	if err = r.reconcileCassandraConfigMap(ctx, cc, restartChecksum); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling cassandra configmaps")
	}

	// although the pods don't exist yet on the first run, we still need to create the configmap (even empty)
	// so that the pods won't fail trying to mount an empty configmap
	if err = r.reconcileCassandraPodsConfigMap(ctx, cc, proberClient); err != nil {
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

	if err = r.reconcileCassandraPodLabels(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile cassandra pods labels")
	}

	if err = r.reconcileMaintenance(ctx, cc); err != nil {
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

	allDCs, err := r.getAllDCs(ctx, cc, proberClient)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "can't get all dcs across regions")
	}

	cqlClient, err := r.reconcileAdminRole(ctx, cc, allDCs)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile Admin Role")
	}
	defer cqlClient.CloseSession()

	err = r.reconcileRoles(ctx, cc, cqlClient)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err = r.reconcileReaperPrerequisites(ctx, cc, cqlClient, allDCs); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling reaper")
	}

	if res, err := r.reconcileReaper(ctx, cc); needsRequeue(res, err) {
		return res, err
	}

	reaperServiceUrl, err := url.Parse(fmt.Sprintf("http://%s.%s.svc.cluster.local:8080", names.ReaperService(cc.Name), cc.Namespace))
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error parsing reaper service url")
	}
	reaperClient := r.ReaperClient(reaperServiceUrl, cc.Name, cc.Spec.Reaper.RepairThreadCount)
	isRunning, err := reaperClient.IsRunning(ctx)
	if err != nil {
		r.Log.Warnf("Reaper ping request failed: %s. Trying again in %s...", err.Error(), r.Cfg.RetryDelay)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}
	if !isRunning {
		r.Log.Infof("Reaper is not ready. Trying again in %s...", r.Cfg.RetryDelay)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}

	if err = r.reaperInitialization(ctx, cc, reaperClient); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to initialize reaper")
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

	if err = r.removeDefaultUserIfExists(ctx, cc, cqlClient); err != nil {
		return ctrl.Result{}, err
	}

	err = cleanupClientTLSDir(cc)
	if err != nil {
		r.Log.Errorf("%+v", err)
	}

	return ctrl.Result{}, nil
}

func needsRequeue(result ctrl.Result, err error) bool {
	return result.Requeue || result.RequeueAfter.Nanoseconds() > 0 || err != nil
}

func (r *CassandraClusterReconciler) clusterReady(ctx context.Context, cc *v1alpha1.CassandraCluster, proberClient prober.ProberClient) (bool, error) {
	unreadyDCs, err := r.unreadyDCs(ctx, cc)
	if err != nil {
		return false, errors.Wrap(err, "Failed to check DCs readiness")
	}

	if len(unreadyDCs) > 0 {
		if err = proberClient.UpdateDCStatus(ctx, false); err != nil {
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
		r.Log.Warnf("Not all regions are ready: %q.", unreadyRegions)
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

		if *dc.Replicas != sts.Status.ReadyReplicas || (sts.Status.ReadyReplicas == 0 && *dc.Replicas != 0) {
			unreadyDCs = append(unreadyDCs, dc.Name)
		}
	}
	return unreadyDCs, nil
}

func (r *CassandraClusterReconciler) unreadyRegions(ctx context.Context, cc *v1alpha1.CassandraCluster, proberClient prober.ProberClient) ([]string, error) {
	unreadyRegions := make([]string, 0)
	for _, externalRegion := range cc.Spec.ExternalRegions {
		if len(externalRegion.Domain) == 0 { //region not managed by an operator
			continue
		}
		proberHost := names.ProberIngressDomain(cc.Name, externalRegion.Domain, cc.Namespace)
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

	if cc.Spec.Encryption.Client.Enabled {
		cassCfg.SslOpts = &gocql.SslOptions{
			CertPath: fmt.Sprintf("%s/%s", names.OperatorClientTLSDir(cc),
				cc.Spec.Encryption.Client.TLSSecret.TLSCrtFileKey),
			KeyPath: fmt.Sprintf("%s/%s", names.OperatorClientTLSDir(cc),
				cc.Spec.Encryption.Client.TLSSecret.TLSFileKey),
			CaPath: fmt.Sprintf("%s/%s", names.OperatorClientTLSDir(cc),
				cc.Spec.Encryption.Client.TLSSecret.CAFileKey),
			EnableHostVerification: false,
		}
	}

	return cassCfg
}

func (r *CassandraClusterReconciler) doWithRetry(retryFunc func() error) error {
	var err error
	for currentAttempt := 1; currentAttempt <= retryAttempts; currentAttempt++ {
		err = retryFunc()
		if err != nil {
			delay := initialRetryDelaySeconds * time.Second * time.Duration(currentAttempt)
			r.Log.Warnf("Attempt %d of %d failed. Error: %s. Trying again in %s.", currentAttempt, retryAttempts, err.Error(), delay.String())
			time.Sleep(delay)
			continue
		}
		break
	}

	return err
}

func (r *CassandraClusterReconciler) removeDefaultUserIfExists(ctx context.Context, cc *v1alpha1.CassandraCluster, cqlClient cql.CqlClient) error {
	existingRoles, err := cqlClient.GetRoles()
	if err != nil {
		return err
	}

	defaultRoleExists := false
	for _, existingRole := range existingRoles {
		if existingRole.Role == v1alpha1.CassandraDefaultRole {
			defaultRoleExists = true
		}
	}

	if !defaultRoleExists {
		return nil
	}

	defaultUserSession, err := r.CqlClient(newCassandraConfig(cc, v1alpha1.CassandraDefaultRole, v1alpha1.CassandraDefaultPassword))
	if err != nil { // can't connect so the default user exists but with changed password which is fine
		return nil
	}
	defaultUserSession.CloseSession()

	adminSecret := &v1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: cc.Spec.AdminRoleSecretName, Namespace: cc.Namespace}, adminSecret)
	if err != nil {
		return errors.Wrap(err, "can't get admin secret")
	}

	if string(adminSecret.Data[v1alpha1.CassandraOperatorAdminRole]) == v1alpha1.CassandraDefaultRole &&
		string(adminSecret.Data[v1alpha1.CassandraOperatorAdminPassword]) == v1alpha1.CassandraDefaultPassword {
		return nil // user wants to use the default role, do not drop it
	}

	r.Log.Info("Dropping role " + v1alpha1.CassandraDefaultRole)
	err = cqlClient.DropRole(cql.Role{Role: v1alpha1.CassandraDefaultRole})
	if err != nil {
		return errors.Wrap(err, "Can't drop role "+v1alpha1.CassandraDefaultRole)
	}

	r.Events.Normal(cc, events.EventDefaultAdminRoleDropped, "default admin role is removed")
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
		Watches(&source.Kind{Type: &v1.Secret{}}, eventhandler.NewAnnotationEventHandler()).
		//WithEventFilter(predicate.NewPredicate(logr)). //uncomment to see kubernetes events in the logs, e.g. ConfigMap updates
		Complete(r)
}

func (r *CassandraClusterReconciler) reconcileAnnotations(ctx context.Context, object client.Object, annotations map[string]string) error {
	currentAnnotations := object.GetAnnotations()

	if currentAnnotations == nil {
		object.SetAnnotations(annotations)
	} else {
		util.MergeMap(currentAnnotations, annotations)
		object.SetAnnotations(currentAnnotations)
	}

	err := r.Update(ctx, object)
	if err != nil {
		return err
	}

	return nil
}

func (r *CassandraClusterReconciler) getAllDCs(ctx context.Context, cc *v1alpha1.CassandraCluster, proberClient prober.ProberClient) ([]v1alpha1.DC, error) {
	allDCs := make([]v1alpha1.DC, 0)
	allDCs = append(allDCs, cc.Spec.DCs...)

	if len(cc.Spec.ExternalRegions) == 0 {
		return allDCs, nil
	}

	for _, externalRegion := range cc.Spec.ExternalRegions {
		if len(externalRegion.Domain) != 0 {
			externalDCs, err := proberClient.GetDCs(ctx, names.ProberIngressDomain(cc.Name, externalRegion.Domain, cc.Namespace))
			if err != nil {
				return nil, errors.Wrapf(err, "can't get dcs list from dc %q", externalRegion.Domain)
			}

			allDCs = append(allDCs, externalDCs...)
		} else {
			if len(externalRegion.Seeds) > 0 && len(externalRegion.DCs) > 0 {
				for _, dc := range externalRegion.DCs {
					rf := int32(defaultRF)
					if dc.RF < 3 {
						rf = dc.RF
					}
					allDCs = append(allDCs, v1alpha1.DC{Name: dc.Name, Replicas: &rf})
				}
			}
		}
	}

	return allDCs, nil
}
