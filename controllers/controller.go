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
	"github.com/gocql/gocql"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
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
	"net/url"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

const (
	proberContainerPort  = 8888
	jolokiaContainerPort = 8080
	maintenancePort      = 8889
	cqlPort              = 9042
	jmxPort              = 7199
	thriftPort           = 9160

	maintenanceDir              = "/etc/maintenance"
	cassandraRolesDir           = "/etc/cassandra-roles"
	defaultCQLConfigMapLabelKey = "cql-scripts"

	annotationCassandraClusterName      = "cassandra-cluster-name"
	annotationCassandraClusterNamespace = "cassandra-cluster-namespace"
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
	NodetoolClient func(clientset *kubernetes.Clientset, config *rest.Config) nodetool.NodetoolClient
	CqlClient      func(cluster *gocql.ClusterConfig) (cql.CqlClient, error)
	ReaperClient   func(url *url.URL) reaper.ReaperClient
}

// +kubebuilder:rbac:groups=db.ibm.com,resources=cassandraclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=db.ibm.com,resources=cassandraclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups="",resources=configmaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;patch;create;update;
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get;patch;update
// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=list;get;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=services;serviceaccounts,verbs=list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=list;watch;get;create;update;delete

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

	r.defaultCassandraCluster(cc)

	if err := r.reconcileScriptsConfigMap(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling scripts configmap")
	}

	if err := r.reconcileRolesSecret(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling roles secret")
	}

	if err := r.reconcileJMXSecret(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling jxm secret")
	}

	if err := r.reconcileProber(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling prober")
	}
	proberUrl, err := url.Parse(fmt.Sprintf("http://%s.%s.svc.cluster.local", names.ProberService(cc), cc.Namespace))
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

	readyAllDCs, err := proberClient.ReadyAllDCs(ctx)
	if err != nil {
		r.Log.Warnf("Prober request failed: %s. Trying again in %s...", err.Error(), r.Cfg.RetryDelay)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}

	if !readyAllDCs {
		r.Log.Infof("Not all DCs are ready. Trying again in %s...", r.Cfg.RetryDelay)
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}

	r.Log.Debug("All DCs are ready")

	ntClient := r.NodetoolClient(r.Clientset, r.RESTConfig)
	cqlClient, err := r.CqlClient(newCassandraConfig(cc))
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Can't create cassandra session")
	}

	err = r.reconcileKeyspaces(cc, cqlClient, ntClient)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile keyspaces")
	}

	err = r.reconcileRoles(cqlClient)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err = r.reconcileCQLConfigMaps(ctx, cc, cqlClient, ntClient); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile CQL config maps")
	}

	if err = r.reconcileReaper(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling reaper")
	}
	reaperServiceUrl, err := url.Parse(fmt.Sprintf("http://%s.%s.svc.cluster.local:8080", names.ReaperService(cc), cc.Namespace))
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

func (r *CassandraClusterReconciler) defaultCassandraCluster(cc *dbv1alpha1.CassandraCluster) {
	if cc.Spec.CQLConfigMapLabelKey == "" {
		cc.Spec.CQLConfigMapLabelKey = defaultCQLConfigMapLabelKey
	}

	if cc.Spec.PodManagementPolicy == "" {
		cc.Spec.PodManagementPolicy = appsv1.ParallelPodManagement
	}

	if cc.Spec.Cassandra == nil {
		cc.Spec.Cassandra = &dbv1alpha1.Cassandra{}
	}

	if cc.Spec.Cassandra.Image == "" {
		cc.Spec.Cassandra.Image = r.Cfg.DefaultCassandraImage
	}

	if cc.Spec.Cassandra.ImagePullPolicy == "" {
		cc.Spec.Cassandra.ImagePullPolicy = v1.PullIfNotPresent
	}

	if cc.Spec.Cassandra.NumSeeds == 0 {
		cc.Spec.Cassandra.NumSeeds = 2
	}

	if cc.Spec.Prober.Image == "" {
		cc.Spec.Prober.Image = r.Cfg.DefaultProberImage
	}

	if cc.Spec.Prober.ImagePullPolicy == "" {
		cc.Spec.Prober.ImagePullPolicy = v1.PullIfNotPresent
	}

	if cc.Spec.Prober.Jolokia.Image == "" {
		cc.Spec.Prober.Jolokia.Image = r.Cfg.DefaultJolokiaImage
	}

	if cc.Spec.Prober.Jolokia.ImagePullPolicy == "" {
		cc.Spec.Prober.Jolokia.ImagePullPolicy = v1.PullIfNotPresent
	}

	if len(cc.Spec.SystemKeyspaces.DCs) == 0 {
		for _, dc := range cc.Spec.DCs {
			cc.Spec.SystemKeyspaces.DCs = append(cc.Spec.SystemKeyspaces.DCs, dbv1alpha1.SystemKeyspaceDC{Name: dc.Name, RF: 3})
		}
	}

	if cc.Spec.Reaper == nil {
		cc.Spec.Reaper = &dbv1alpha1.Reaper{}
	}

	if cc.Spec.Reaper.Keyspace == "" {
		cc.Spec.Reaper.Keyspace = "reaper_db"
	}

	if cc.Spec.Reaper.Image == "" {
		cc.Spec.Reaper.Image = r.Cfg.DefaultReaperImage
	}

	if cc.Spec.Reaper.ImagePullPolicy == "" {
		cc.Spec.Reaper.ImagePullPolicy = v1.PullIfNotPresent
	}

	if len(cc.Spec.Reaper.DCs) == 0 {
		cc.Spec.Reaper.DCs = cc.Spec.DCs
	}

	if cc.Spec.Reaper.RepairIntensity == "" {
		cc.Spec.Reaper.RepairIntensity = "1.0"
	}

	if cc.Spec.Reaper.DatacenterAvailability == "" {
		cc.Spec.Reaper.DatacenterAvailability = "each"
	}

	if len(cc.Spec.Reaper.Tolerations) == 0 {
		cc.Spec.Reaper.Tolerations = nil
	}

	if len(cc.Spec.Reaper.NodeSelector) == 0 {
		cc.Spec.Reaper.NodeSelector = nil
	}

	if len(cc.Spec.Reaper.ScheduleRepairs.Repairs) != 0 {
		for i, repair := range cc.Spec.Reaper.ScheduleRepairs.Repairs {
			if repair.RepairParallelism == "" {
				cc.Spec.Reaper.ScheduleRepairs.Repairs[i].RepairParallelism = "datacenter_aware"
			}

			// RepairParallelism must be 'parallel' if IncrementalRepair is 'true'
			if repair.IncrementalRepair {
				cc.Spec.Reaper.ScheduleRepairs.Repairs[i].RepairParallelism = "parallel"
			}

			if repair.ScheduleDaysBetween == 0 {
				cc.Spec.Reaper.ScheduleRepairs.Repairs[i].ScheduleDaysBetween = 7
			}

			if len(repair.Datacenters) == 0 {
				dcNames := make([]string, 0, len(cc.Spec.DCs))
				for _, dc := range cc.Spec.DCs {
					dcNames = append(dcNames, dc.Name)
				}
				cc.Spec.Reaper.ScheduleRepairs.Repairs[i].Datacenters = dcNames
			}
			if repair.RepairThreadCount == 0 {
				cc.Spec.Reaper.ScheduleRepairs.Repairs[i].RepairThreadCount = 2
			}
		}
	}
}

func newCassandraConfig(cc *dbv1alpha1.CassandraCluster) *gocql.ClusterConfig {
	cassCfg := gocql.NewCluster(fmt.Sprintf("%s.%s.svc.cluster.local", names.DCService(cc, cc.Spec.DCs[0].Name), cc.Namespace))
	cassCfg.Authenticator = &gocql.PasswordAuthenticator{
		Username: dbv1alpha1.CassandraRole,
		Password: dbv1alpha1.CassandraPassword,
	}

	cassCfg.Timeout = 6 * time.Second
	cassCfg.ConnectTimeout = 6 * time.Second
	cassCfg.ProtoVersion = 4
	cassCfg.Consistency = gocql.LocalQuorum

	return cassCfg
}

func SetupCassandraReconciler(r reconcile.Reconciler, mgr manager.Manager, logr *zap.SugaredLogger) error {
	annotationEventHandler := &handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(
		func(a handler.MapObject) []ctrl.Request {
			annotations := a.Meta.GetAnnotations()
			if annotations[annotationCassandraClusterNamespace] == "" {
				return nil
			}

			if annotations[annotationCassandraClusterName] == "" {
				return nil
			}

			return []ctrl.Request{
				{NamespacedName: types.NamespacedName{
					Name:      annotations[annotationCassandraClusterName],
					Namespace: annotations[annotationCassandraClusterNamespace],
				}},
			}
		}),
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("cassandracluster").
		For(&dbv1alpha1.CassandraCluster{}).
		Owns(&appsv1.Deployment{}).
		Owns(&v1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&v1.Secret{}).
		Owns(&v1.ConfigMap{}).
		Owns(&rbac.Role{}).
		Owns(&rbac.RoleBinding{}).
		Owns(&v1.ServiceAccount{}).
		Watches(&source.Kind{Type: &rbac.ClusterRole{}}, annotationEventHandler).
		Watches(&source.Kind{Type: &rbac.ClusterRoleBinding{}}, annotationEventHandler).
		//WithEventFilter(predicate.NewPredicate(logr)). //uncomment to see kubernetes events in the logs, e.g. ConfigMap updates
		Complete(r)
}
