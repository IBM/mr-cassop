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
	"github.com/ibm/cassandra-operator/controllers/config"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/nodetool"
	"github.com/ibm/cassandra-operator/controllers/prober"
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
	"time"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
)

// CassandraClusterReconciler reconciles a CassandraCluster object
type CassandraClusterReconciler struct {
	client.Client
	Log        *zap.SugaredLogger
	Scheme     *runtime.Scheme
	Cfg        config.Config
	Clientset  *kubernetes.Clientset
	RESTConfig *rest.Config
}

// +kubebuilder:rbac:groups=db.ibm.com,resources=cassandraclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=db.ibm.com,resources=cassandraclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups="",resources=configmaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;
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

	if err := r.reconcileScriptsConfigMap(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling scripts configmap")
	}

	if err := r.reconcileUsersSecret(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling users secret")
	}

	if err := r.reconcileJMXSecret(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling jxm secret")
	}

	if err := r.reconcileProber(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling prober")
	}

	proberClient := prober.NewProberClient(fmt.Sprintf("%s.%s.svc.cluster.local", names.ProberService(cc), cc.Namespace))
	proberReady, err := r.proberReady(ctx, cc)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "can't get prober's status")
	}

	if !proberReady {
		r.Log.Warnf("Prober is not ready. Trying again in 5 seconds...")
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	if err := r.reconcileCassandraConfigConfigMap(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling cassandra config configmaps")
	}

	if err := r.reconcileCassandra(ctx, cc); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error reconciling statefulsets")
	}

	readyAllDCs, err := proberClient.ReadyAllDCs(ctx)
	if err != nil {
		r.Log.Warnf("Prober request failed: %s. Trying again in 5 seconds...", err.Error())
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if !readyAllDCs {
		r.Log.Info("Not all DCs are ready. Trying again in 10 seconds...")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	r.Log.Debug("All DCs are ready")

	if cc.Spec.Kwatcher.Enabled {
		if err := r.reconcileKwatcher(ctx, cc); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "Error reconciling kwatcher")
		}
	}

	ntClient := nodetool.NewNodetoolClient(r.Clientset, r.RESTConfig)
	cqlClient, err := cql.NewCQLClient(cc)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Can't create cassandra session")
	}

	if err = r.reconcileRFSettings(cc, cqlClient, ntClient); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to reconcile RF settings")
	}

	if cc.Spec.InternalAuth && cc.Spec.Kwatcher.Enabled {
		r.Log.Debug("Internal auth and kwatcher are enabled. Checking if all users are created.")
		created, err := r.usersCreated(ctx, cc, cqlClient)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "Can't get created users info")
		}

		if !created {
			r.Log.Info("Users hasn't been created yet. Checking again in 10 seconds...")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		r.Log.Info("Users has been created")
	}

	if err = r.reconcileCQLConfigMaps(ctx, cc, cqlClient, ntClient); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile CQL config maps")
	}

	return ctrl.Result{}, nil
}

func (r *CassandraClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1alpha1.CassandraCluster{}).
		Owns(&appsv1.Deployment{}).
		Owns(&v1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&v1.Secret{}).
		Owns(&v1.ConfigMap{}).
		Owns(&rbac.ClusterRole{}).
		Owns(&rbac.ClusterRoleBinding{}).
		Complete(r)
}
