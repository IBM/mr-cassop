package controllers

import (
	"context"
	"encoding/json"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/compare"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type CassandraKeyspace struct {
	Keyspace          string              `json:"keyspace"`
	Strategy          string              `json:"strategy"`
	DurableWrites     bool                `json:"durable_writes"`
	ReplicationFactor []ReplicationFactor `json:"replication_factor"`
}

type ReplicationFactor struct {
	DC string `json:"dc"`
	RF int32  `json:"rf"`
}

func (r *CassandraClusterReconciler) reconcileKwatcherKeyspaceConfigMap(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	desiredCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.KeyspaceConfigMap(cc),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentKwatcher),
		},
	}
	data := make(map[string]string)
	for _, keyspaceName := range cc.Spec.SystemKeyspaces.Names {
		replicationFactor := make([]ReplicationFactor, 0, len(cc.Spec.SystemKeyspaces.DCs))
		for _, dc := range cc.Spec.SystemKeyspaces.DCs {
			replicationFactor = append(replicationFactor, ReplicationFactor{DC: dc.Name, RF: dc.RF})
		}

		keyspace := CassandraKeyspace{
			Keyspace:          string(keyspaceName),
			Strategy:          "NetworkTopologyStrategy",
			DurableWrites:     true,
			ReplicationFactor: replicationFactor,
		}

		jsonKeyspace, _ := json.Marshal(keyspace)
		data[string(keyspaceName)] = string(jsonKeyspace)
	}
	desiredCM.Data = data

	if err := controllerutil.SetControllerReference(cc, desiredCM, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}
	if err := r.reconcileConfigMap(ctx, desiredCM); err != nil {
		return err
	}
	return nil
}

// creates a stub configmap for now
func (r *CassandraClusterReconciler) reconcileKwatcherJobPacementConfigMap(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	desiredCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cc.Name + "-job-placement-configmap",
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentKwatcher),
			Namespace: cc.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(cc, desiredCM, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualCM := &v1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: desiredCM.Name, Namespace: desiredCM.Namespace}, actualCM)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Info("Creating kwatcher job-placement ConfigMap")
		if err = r.Create(ctx, desiredCM); err != nil {
			return errors.Wrap(err, "Unable to create kwatcher job-placement ConfigMap")
		}
	} else if err != nil {
		return errors.Wrap(err, "Could not Get kwatcher job-placement ConfigMap")
	} else if !compare.EqualConfigMap(actualCM, desiredCM) {
		r.Log.Info("Updating kwatcher job-placement ConfigMap")
		r.Log.Debug(compare.DiffConfigMap(actualCM, desiredCM))
		actualCM.Labels = desiredCM.Labels
		actualCM.Data = desiredCM.Data
		if err = r.Update(ctx, actualCM); err != nil {
			return errors.Wrap(err, "Could not Update kwatcher job-placement ConfigMap")
		}
	} else {
		r.Log.Debug("No updates for prober desiredCM")
	}

	return nil
}
