package controllers

import (
	"context"

	"github.com/ibm/cassandra-operator/controllers/compare"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (r *CassandraClusterReconciler) getConfigMap(ctx context.Context, name, namespace string) (*v1.ConfigMap, error) {
	cm := &v1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, cm); err != nil {
		if kerrors.IsNotFound(err) {
			return nil, errors.Wrapf(err, "%s configmap doesn't exist", name)
		}
		return nil, errors.Wrapf(err, "can't get %s", name)
	}
	return cm, nil
}

func (r *CassandraClusterReconciler) getSecret(ctx context.Context, name, namespace string) (*v1.Secret, error) {
	st := &v1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, st); err != nil {
		if kerrors.IsNotFound(err) {
			return nil, errors.Wrapf(err, "%s secret doesn't exist", name)
		}
		return nil, errors.Wrapf(err, "can't get %s", name)
	}
	return st, nil
}

func (r *CassandraClusterReconciler) reconcileConfigMap(ctx context.Context, desiredCM *v1.ConfigMap) error {
	actualCM := &v1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: desiredCM.Name, Namespace: desiredCM.Namespace}, actualCM)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Infof("Creating %s", desiredCM.Name)
		if err = r.Create(ctx, desiredCM); err != nil {
			return errors.Wrapf(err, "Unable to create %s", desiredCM.Name)
		}
	} else if err != nil {
		return errors.Wrapf(err, "Could not get %s", desiredCM.Name)
	} else {
		desiredCM.Annotations = util.MergeMap(actualCM.Annotations, desiredCM.Annotations)
		if !compare.EqualConfigMap(actualCM, desiredCM) {
			r.Log.Infof("Updating %s", desiredCM.Name)
			r.Log.Debug(compare.DiffConfigMap(actualCM, desiredCM))
			actualCM.Labels = desiredCM.Labels
			actualCM.Data = desiredCM.Data
			actualCM.Annotations = desiredCM.Annotations
			actualCM.OwnerReferences = desiredCM.OwnerReferences
			if err = r.Update(ctx, actualCM); err != nil {
				return errors.Wrapf(err, "Could not update %s", desiredCM.Name)
			}
		} else {
			r.Log.Debugf("No updates for %s", desiredCM.Name)
		}
	}
	return nil
}

func (r *CassandraClusterReconciler) reconcileSecret(ctx context.Context, desiredSecret *v1.Secret) error {
	actualSecret := &v1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: desiredSecret.Name, Namespace: desiredSecret.Namespace}, actualSecret)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Infof("Creating secret %s", desiredSecret.Name)
		if err = r.Create(ctx, desiredSecret); err != nil {
			return errors.Wrapf(err, "Unable to create secret %s", desiredSecret.Name)
		}
	} else if err != nil {
		return errors.Wrapf(err, "Could not get secret %s", desiredSecret.Name)
	} else {
		desiredSecret.Annotations = util.MergeMap(actualSecret.Annotations, desiredSecret.Annotations)
		if !compare.EqualSecret(actualSecret, desiredSecret) {
			r.Log.Infof("Updating %s", desiredSecret.Name)
			r.Log.Debug(compare.DiffSecret(actualSecret, desiredSecret))
			actualSecret.Labels = desiredSecret.Labels
			actualSecret.Data = desiredSecret.Data
			actualSecret.Annotations = util.MergeMap(actualSecret.Annotations, desiredSecret.Annotations)
			actualSecret.OwnerReferences = desiredSecret.OwnerReferences
			if err = r.Update(ctx, actualSecret); err != nil {
				return errors.Wrapf(err, "Could not update secret %s", desiredSecret.Name)
			}
		} else {
			r.Log.Debugf("No updates for secret %s", desiredSecret.Name)
		}
	}
	return nil
}
