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

func (r *CassandraClusterReconciler) reconcileKwatcherClusterRoleBinding(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	desiredCRB := &rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   names.KwatcherClusterRoleBinding(cc),
			Labels: labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentKwatcher),
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      names.KwatcherServiceAccount(cc),
				Namespace: cc.Namespace,
			},
		},
		RoleRef: rbac.RoleRef{
			Kind:     "ClusterRole",
			Name:     names.KwatcherClusterRole(cc),
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	actualCRB := &rbac.ClusterRoleBinding{}
	err := r.Get(ctx, types.NamespacedName{Name: desiredCRB.Name}, actualCRB)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Info("Creating kwatcher ClusterRoleBinding")
		if err = r.Create(ctx, desiredCRB); err != nil {
			return errors.Wrap(err, "Unable to create kwatcher ClusterRoleBinding")
		}
	} else if err != nil {
		return errors.Wrap(err, "Could not Get kwatcher ClusterRoleBinding")
	} else if !compare.EqualClusterRoleBinding(actualCRB, desiredCRB) {
		r.Log.Info("Updating kwatcher ClusterRoleBinding")
		r.Log.Debug(compare.DiffClusterRoleBinding(actualCRB, desiredCRB))
		actualCRB.Subjects = desiredCRB.Subjects
		actualCRB.RoleRef = desiredCRB.RoleRef
		actualCRB.Labels = desiredCRB.Labels
		if err = r.Update(ctx, actualCRB); err != nil {
			return errors.Wrap(err, "Could not Update kwatcher ClusterRoleBinding")
		}
	} else {
		r.Log.Debugw("No updates for kwatcher ClusterRolebinding")
	}
	return nil
}
