package controllers

import (
	"context"
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

func (r *CassandraClusterReconciler) reconcileKwatcherServiceAccount(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	saName := names.KwatcherServiceAccount(cc)
	desiredServiceAccound := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentKwatcher),
		},
	}

	if err := controllerutil.SetControllerReference(cc, desiredServiceAccound, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualServiceAccount := &v1.ServiceAccount{}

	err := r.Get(ctx, types.NamespacedName{Name: desiredServiceAccound.Name, Namespace: desiredServiceAccound.Namespace}, actualServiceAccount)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Info("Creating kwatcher serviceaccount")
		if err = r.Create(ctx, desiredServiceAccound); err != nil {
			return errors.Wrap(err, "Unable to create kwatcher serviceaccount")
		}
	} else if err != nil {
		return errors.Wrap(err, "Could not get kwatcher serviceaccount")
	} else if !compare.EqualServiceAccount(actualServiceAccount, desiredServiceAccound) {
		r.Log.Info("Updating kwatcher Service account")
		r.Log.Debugf(compare.DiffServiceAccount(actualServiceAccount, desiredServiceAccound))
		actualServiceAccount.Labels = desiredServiceAccound.Labels
		if err = r.Update(ctx, actualServiceAccount); err != nil {
			return errors.Wrap(err, "Unable to update kwatcher serviceaccount")
		}
	} else {
		r.Log.Debug("No updates for kwatcher serviceaccount")
	}
	return nil
}
