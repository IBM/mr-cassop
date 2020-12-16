package controllers

import (
	"context"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/compare"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"

	"github.com/pkg/errors"

	rbac "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *CassandraClusterReconciler) reconcileKwatcherClusterRole(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	desiredCR := &rbac.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   names.KwatcherClusterRole(cc),
			Labels: labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentKwatcher),
			Annotations: map[string]string{
				annotationCassandraClusterName:      cc.Name,
				annotationCassandraClusterNamespace: cc.Namespace,
			},
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"list", "watch", "get"},
			},
		},
	}

	actualCR := &rbac.ClusterRole{}
	err := r.Get(ctx, types.NamespacedName{Name: desiredCR.Name}, actualCR)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Info("Creating kwatcher ClusterRole")
		if err = r.Create(ctx, desiredCR); err != nil {
			return errors.Wrap(err, "Unable to create kwatcher clusterrole")
		}
	} else if err != nil {
		return errors.Wrap(err, "Could not Get kwatcher clusterrole")
	} else if !compare.EqualClusterRole(actualCR, actualCR) {
		r.Log.Info("Updating kwatcher ClusterRole")
		r.Log.Debug(compare.DiffClusterRole(actualCR, desiredCR))
		actualCR.Rules = desiredCR.Rules
		actualCR.Labels = desiredCR.Labels
		actualCR.Annotations = desiredCR.Annotations
		if err = r.Update(ctx, actualCR); err != nil {
			return errors.Wrap(err, "Could not Update kwatcher clusterrole")
		}
	} else {
		r.Log.Debug("No updates for kwatcher ClusterRole")
	}

	return nil
}
