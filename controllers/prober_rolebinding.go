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

func (r *CassandraClusterReconciler) reconcileProberRoleBinding(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	desiredRoleBinding := &rbac.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ProberRoleBinding(cc),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentProber),
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      names.ProberServiceAccount(cc),
				Namespace: cc.Namespace,
			},
		},
		RoleRef: rbac.RoleRef{
			Kind:     "Role",
			Name:     names.ProberRole(cc),
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	if err := controllerutil.SetControllerReference(cc, desiredRoleBinding, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualRoleBinding := &rbac.RoleBinding{}

	err := r.Get(ctx, types.NamespacedName{Name: desiredRoleBinding.Name, Namespace: desiredRoleBinding.Namespace}, actualRoleBinding)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Info("Creating prober RoleBinding")
		if err = r.Create(ctx, desiredRoleBinding); err != nil {
			return errors.Wrap(err, "Unable to create prober roleBinding")
		}
	} else if err != nil {
		return errors.Wrap(err, "Could not Get prober roleBinding")
	} else if !compare.EqualRoleBinding(actualRoleBinding, desiredRoleBinding) {
		r.Log.Info("Updating prober RoleBinding")
		r.Log.Debug(compare.DiffRoleBinding(actualRoleBinding, desiredRoleBinding))
		actualRoleBinding.Subjects = desiredRoleBinding.Subjects
		actualRoleBinding.RoleRef = desiredRoleBinding.RoleRef
		actualRoleBinding.Labels = desiredRoleBinding.Labels
		if err = r.Update(ctx, actualRoleBinding); err != nil {
			return errors.Wrap(err, "Could not Update prober roleBinding")
		}
	} else {
		r.Log.Debugw("No updates for prober Rolebinding")
	}
	return nil
}
