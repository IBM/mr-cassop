package controllers

import (
	"context"
	"fmt"
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

func (r *CassandraClusterReconciler) reconcileReaperCqlConfigMap(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	desiredCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:   names.ReaperCqlConfigMap(cc),
			Labels: labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentReaper),
			Annotations: map[string]string{
				"cql-repairKeyspace": cc.Spec.Reaper.Keyspace,
			},
			Namespace: cc.Namespace,
		},
		Data: map[string]string{
			"init-reaper.cql": fmt.Sprintf("CREATE KEYSPACE IF NOT EXISTS %s %s ;", cc.Spec.Reaper.Keyspace, WithReplication(cc)),
		},
	}
	desiredCM.ObjectMeta.Labels[cc.Spec.CQLConfigMapLabelKey] = ""

	if err := controllerutil.SetControllerReference(cc, desiredCM, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualCM := &v1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: desiredCM.Name, Namespace: desiredCM.Namespace}, actualCM)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Info("Creating reaper cql ConfigMap")
		if err = r.Create(ctx, desiredCM); err != nil {
			return errors.Wrap(err, "Unable to create reaper cql ConfigMap")
		}
	} else if err != nil {
		return errors.Wrap(err, "Could not Get reaper cql ConfigMap")
	} else {
		desiredCM.Annotations = actualCM.Annotations
		if !compare.EqualConfigMap(actualCM, desiredCM) {
			r.Log.Info("Updating reaper cql ConfigMap")
			r.Log.Debug(compare.DiffConfigMap(actualCM, desiredCM))
			actualCM.Labels = desiredCM.Labels
			actualCM.Data = desiredCM.Data
			if err = r.Update(ctx, actualCM); err != nil {
				return errors.Wrap(err, "Could not Update reaper cql ConfigMap")
			}
		} else {
			r.Log.Debug("No updates for reaper cql ConfigMap")
		}
	}

	return nil
}

func WithReplication(cc *v1alpha1.CassandraCluster) string {
	str := "WITH REPLICATION = { 'class' : 'NetworkTopologyStrategy'"
	for _, dc := range cc.Spec.Reaper.DCs {
		str += fmt.Sprintf(", '%s' : %d", dc.Name, *dc.Replicas)
	}
	str += "}"
	return str
}
