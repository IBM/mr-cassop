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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CassandraClusterReconciler) reconcileKwatcherRole(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	desiredRole := &rbac.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.KwatcherRole(cc),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentKwatcher),
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{"apps"},
				Resources: []string{"statefulsets"},
				Verbs:     []string{"list", "watch", "get"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps", "secrets"},
				Verbs:     []string{"list", "watch", "get", "update"},
			},
			{
				APIGroups: []string{"batch"},
				Resources: []string{"jobs"},
				Verbs:     []string{"get", "list", "watch", "update", "patch", "delete", "create"},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cc, desiredRole, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualRole := &rbac.Role{}

	err := r.Get(ctx, types.NamespacedName{Name: desiredRole.Name, Namespace: desiredRole.Namespace}, actualRole)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Info("Creating kwatcher Role")
		if err = r.Create(ctx, desiredRole); err != nil {
			return errors.Wrap(err, "Unable to create kwatcher role")
		}
	} else if err != nil {
		return errors.Wrap(err, "Could not Get kwatcher role")
	} else if !compare.EqualRole(actualRole, desiredRole) {
		r.Log.Info("Updating kwatcher Role")
		r.Log.Debug(compare.DiffRole(actualRole, desiredRole))
		actualRole.Rules = desiredRole.Rules
		actualRole.Labels = desiredRole.Labels
		if err = r.Update(ctx, actualRole); err != nil {
			return errors.Wrap(err, "Could not Update kwatcher role")
		}
	} else {
		r.Log.Debug("No updates for kwatcher Role")

	}
	return nil
}
