package controllers

import (
	"context"
	"fmt"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CassandraClusterReconciler) reconcileReaperCqlConfigMap(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	desiredCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ReaperCqlConfigMap(cc),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentReaper),
			Annotations: map[string]string{
				"cql-repairKeyspace": cc.Spec.Reaper.Keyspace,
			},
		},
		Data: map[string]string{
			"init-reaper.cql": fmt.Sprintf("CREATE KEYSPACE IF NOT EXISTS %s %s ;", cc.Spec.Reaper.Keyspace, WithReplication(cc)),
		},
	}
	desiredCM.ObjectMeta.Labels[cc.Spec.CQLConfigMapLabelKey] = ""
	if err := controllerutil.SetControllerReference(cc, desiredCM, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	if err := r.reconcileConfigMap(ctx, desiredCM); err != nil {
		return err
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
