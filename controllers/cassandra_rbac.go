package controllers

import (
	"context"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/compare"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"

	"github.com/pkg/errors"

	v1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CassandraClusterReconciler) reconcileCassandraRBAC(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	if err := r.reconcileCassandraRole(ctx, cc); err != nil {
		return err
	}
	if err := r.reconcileCassandraRoleBinding(ctx, cc); err != nil {
		return err
	}
	if err := r.reconcileCassandraServiceAccount(ctx, cc); err != nil {
		return err
	}

	return nil
}

func (r *CassandraClusterReconciler) reconcileCassandraRole(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	desiredRole := &rbac.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.CassandraRole(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra),
		},
		Rules: []rbac.PolicyRule{
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
		r.Log.Info("Creating cassandra Role")
		if err = r.Create(ctx, desiredRole); err != nil {
			return errors.Wrap(err, "Unable to create cassandra role")
		}
	} else if err != nil {
		return errors.Wrap(err, "Could not Get cassandra role")
	} else if !compare.EqualRole(actualRole, desiredRole) {
		r.Log.Info("Updating cassandra Role")
		r.Log.Debug(compare.DiffRole(actualRole, desiredRole))
		actualRole.Rules = desiredRole.Rules
		actualRole.Labels = desiredRole.Labels
		if err = r.Update(ctx, actualRole); err != nil {
			return errors.Wrap(err, "Could not Update cassandra role")
		}
	} else {
		r.Log.Debug("No updates for cassandra Role")

	}
	return nil
}

func (r *CassandraClusterReconciler) reconcileCassandraRoleBinding(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	desiredRoleBinding := &rbac.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.CassandraRoleBinding(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra),
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      names.CassandraServiceAccount(cc.Name),
				Namespace: cc.Namespace,
			},
		},
		RoleRef: rbac.RoleRef{
			Kind:     "Role",
			Name:     names.CassandraRole(cc.Name),
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	if err := controllerutil.SetControllerReference(cc, desiredRoleBinding, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualRoleBinding := &rbac.RoleBinding{}

	err := r.Get(ctx, types.NamespacedName{Name: desiredRoleBinding.Name, Namespace: desiredRoleBinding.Namespace}, actualRoleBinding)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Info("Creating cassandra RoleBinding")
		if err = r.Create(ctx, desiredRoleBinding); err != nil {
			return errors.Wrap(err, "Unable to create cassandra roleBinding")
		}
	} else if err != nil {
		return errors.Wrap(err, "Could not Get cassandra roleBinding")
	} else if !compare.EqualRoleBinding(actualRoleBinding, desiredRoleBinding) {
		r.Log.Info("Updating cassandra RoleBinding")
		r.Log.Debug(compare.DiffRoleBinding(actualRoleBinding, desiredRoleBinding))
		actualRoleBinding.Subjects = desiredRoleBinding.Subjects
		actualRoleBinding.RoleRef = desiredRoleBinding.RoleRef
		actualRoleBinding.Labels = desiredRoleBinding.Labels
		if err = r.Update(ctx, actualRoleBinding); err != nil {
			return errors.Wrap(err, "Could not Update cassandra roleBinding")
		}
	} else {
		r.Log.Debugw("No updates for cassandra Rolebinding")
	}
	return nil
}

func (r *CassandraClusterReconciler) reconcileCassandraServiceAccount(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	saName := names.CassandraServiceAccount(cc.Name)
	desiredServiceAccount := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra),
		},
	}

	if err := controllerutil.SetControllerReference(cc, desiredServiceAccount, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualServiceAccount := &v1.ServiceAccount{}
	err := r.Get(ctx, types.NamespacedName{Name: desiredServiceAccount.Name, Namespace: desiredServiceAccount.Namespace}, actualServiceAccount)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Info("Creating Service cassandra account")
		if err = r.Create(ctx, desiredServiceAccount); err != nil {
			return errors.Wrap(err, "Unable to create cassandra Service account")
		}
	} else if err != nil {
		return errors.Wrap(err, "Could not get cassandra Service account")
	} else if !compare.EqualServiceAccount(actualServiceAccount, desiredServiceAccount) {
		r.Log.Info("Updating cassandra Service account")
		r.Log.Debugf(compare.DiffServiceAccount(actualServiceAccount, desiredServiceAccount))
		actualServiceAccount.Labels = desiredServiceAccount.Labels
		if err = r.Update(ctx, actualServiceAccount); err != nil {
			return errors.Wrap(err, "Unable to update cassandra service account")
		}
	} else {
		r.Log.Debug("No updates for cassandra Service account")
	}
	return nil
}
