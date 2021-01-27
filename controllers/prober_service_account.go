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

func (r *CassandraClusterReconciler) reconcileProberServiceAccount(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	saName := names.ProberServiceAccount(cc.Name)
	desiredServiceAccount := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentProber),
		},
	}

	if err := controllerutil.SetControllerReference(cc, desiredServiceAccount, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualServiceAccount := &v1.ServiceAccount{}
	err := r.Get(ctx, types.NamespacedName{Name: desiredServiceAccount.Name, Namespace: desiredServiceAccount.Namespace}, actualServiceAccount)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Info("Creating Service prober account")
		if err = r.Create(ctx, desiredServiceAccount); err != nil {
			return errors.Wrap(err, "Unable to create prober Service account")
		}
	} else if err != nil {
		return errors.Wrap(err, "Could not get prober Service account")
	} else if !compare.EqualServiceAccount(actualServiceAccount, desiredServiceAccount) {
		r.Log.Info("Updating prober Service account")
		r.Log.Debugf(compare.DiffServiceAccount(actualServiceAccount, desiredServiceAccount))
		actualServiceAccount.Labels = desiredServiceAccount.Labels
		if err = r.Update(ctx, actualServiceAccount); err != nil {
			return errors.Wrap(err, "Unable to update prober service account")
		}
	} else {
		r.Log.Debug("No updates for prober Service account")
	}
	return nil
}
