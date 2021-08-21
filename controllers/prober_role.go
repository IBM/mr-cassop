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

func (r *CassandraClusterReconciler) reconcileProberRole(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	desiredRole := &rbac.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ProberRole(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentProber),
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{"apps"},
				Resources: []string{"statefulsets"},
				Verbs:     []string{"list", "watch", "get"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"services"},
				Verbs:     []string{"list", "watch", "update"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"list", "watch", "get", "patch", "update", "create", "delete"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"list", "watch", "get", "patch", "update", "create"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods/status", "configmaps/status"},
				Verbs:     []string{"get", "patch", "update"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}

	// Set controller reference for role
	if err := controllerutil.SetControllerReference(cc, desiredRole, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualRole := &rbac.Role{}

	err := r.Get(ctx, types.NamespacedName{Name: desiredRole.Name, Namespace: desiredRole.Namespace}, actualRole)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Info("Creating prober Role")
		if err = r.Create(ctx, desiredRole); err != nil {
			return errors.Wrap(err, "Unable to create prober role")
		}
	} else if err != nil {
		return errors.Wrap(err, "Could not Get prober role")
	} else if !compare.EqualRole(actualRole, desiredRole) {
		r.Log.Info("Updating prober Role")
		r.Log.Debug(compare.DiffRole(actualRole, desiredRole))
		actualRole.Rules = desiredRole.Rules
		actualRole.Labels = desiredRole.Labels
		if err = r.Update(ctx, actualRole); err != nil {
			return errors.Wrap(err, "Could not Update prober role")
		}
	} else {
		r.Log.Debug("No updates for prober Role")

	}
	return nil
}
