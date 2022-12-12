package controllers

import (
	"context"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CassandraClusterReconciler) reconcileShiroConfigMap(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	operatorCM, err := r.getConfigMap(ctx, names.OperatorShiroCM(), r.Cfg.Namespace)
	if err != nil {
		return err
	}
	desiredCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ShiroConfigMap(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentReaper),
		},
		Data: operatorCM.Data,
	}
	if err := controllerutil.SetControllerReference(cc, desiredCM, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}
	if err = r.reconcileConfigMap(ctx, desiredCM); err != nil {
		return err
	}
	return nil
}
