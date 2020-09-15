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
	"encoding/base64"
	"fmt"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/nodetool"
	"github.com/ibm/cassandra-operator/controllers/prober"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
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

	proberClient := prober.NewProberClient(cc.Spec.ProberHost)
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

	ntClient := nodetool.NewNodetoolClient(r.Clientset, r.RESTConfig)
	cqlClient, err := cql.NewCQLClient(cc)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Can't create cassandra session")
	}

	if err = r.reconcileRFSettings(cc, cqlClient, ntClient); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to reconcile RF settings")
	}

	if cc.Spec.InternalAuth && cc.Spec.KwatcherEnabled {
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

	r.Log.Debug("Executing queries from CQL ConfigMaps")
	if err = r.reconcileCQLConfigMaps(ctx, cc, cqlClient, ntClient); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile CQL config maps")
	}

	return ctrl.Result{}, nil
}

func (r *CassandraClusterReconciler) reconcileRFSettings(cc *dbv1alpha1.CassandraCluster, cqlClient *cql.CQLClient, ntClient *nodetool.NodetoolCLient) error {
	//TODO implement a check if the RF settings need to be updated
	r.Log.Debug("Executing query to update RF info")
	if err := cqlClient.UpdateRF(cc); err != nil {
		return errors.Wrap(err, "Can't update RF info")
	}

	r.Log.Debug("Repairing system_auth keyspace")
	if err := ntClient.RepairKeyspace(cc, "system_auth"); err != nil {
		return errors.Wrapf(err, "Failed to repair %q keyspace", "system_auth")
	}

	return nil
}

func (r *CassandraClusterReconciler) reconcileCQLConfigMaps(ctx context.Context, cc *dbv1alpha1.CassandraCluster, cqlClient *cql.CQLClient, ntClient *nodetool.NodetoolCLient) error {
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
			if err = cqlClient.Query(cqlQuery).Exec(); err != nil {
				return errors.Wrapf(err, "Query with queryKey %q failed", queryKey)
			}
		}

		keyspaceToRepair := cm.Annotations["cql-repairKeyspace"]
		if len(keyspaceToRepair) > 0 {
			r.Log.Debugf("Repairing %q keyspace", keyspaceToRepair)

			if err = ntClient.RepairKeyspace(cc, keyspaceToRepair); err != nil {
				return errors.Wrapf(err, "Failed to repair %q keyspace", keyspaceToRepair)
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

func (r *CassandraClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1alpha1.CassandraCluster{}).
		// how to watch and map resources
		Complete(r)
}
