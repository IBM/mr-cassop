package cassandrarestore

import (
	"context"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/icarus"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *CassandraRestoreReconciler) reconcileRestore(ctx context.Context, ic icarus.Icarus,
	cr *v1alpha1.CassandraRestore, cb *v1alpha1.CassandraBackup, cc *v1alpha1.CassandraCluster) (ctrl.Result, error) {
	snapshotTag := cr.Spec.SnapshotTag
	if len(snapshotTag) == 0 {
		if cb == nil {
			return ctrl.Result{}, errors.New("No snapshotTag specified. " +
				"It should be in the CassandraRestore spec (.spec.snapshotTag) or a CassandraBackup should be specified (.spec.cassandraBackup)")
		}
		snapshotTag = cb.Name
	}

	icarusRestores, err := ic.Restores(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	relatedIcarusRestore, relatedIcarusRestoreFound := findRelatedIcarusRestore(icarusRestores, snapshotTag)

	if cr.Status.State == icarus.StateFailed {
		if !relatedIcarusRestoreFound {
			r.Log.Infof("Restore %s/%s has failed and no matching restore request found in icarus. "+
				"Recreate the CassandraRestore resource to start a new restore attempt", cr.Namespace, cr.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, r.reconcileFailedRestore(ctx, ic, cr, cb, cc, relatedIcarusRestore)
	}

	if !relatedIcarusRestoreFound { // doesn't exist yet, create it
		err = ic.Restore(ctx, createRestoreReq(cc, cb, cr))
		if err != nil {
			return ctrl.Result{}, err
		}

		r.Log.Info("Restore request sent")
		return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
	}

	err = r.reconcileStatus(ctx, cr, relatedIcarusRestore)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
}

func findRelatedIcarusRestore(icarusRestores []icarus.Restore, snapshotTag string) (icarus.Restore, bool) {
	for i, existingRestore := range icarusRestores {
		if !existingRestore.GlobalRequest { //filter out non coordinator requests
			continue
		}

		if existingRestore.SnapshotTag != snapshotTag {
			continue
		}

		return icarusRestores[i], true
	}

	return icarus.Restore{}, false
}

func (r *CassandraRestoreReconciler) reconcileFailedRestore(ctx context.Context, ic icarus.Icarus, cr *v1alpha1.CassandraRestore,
	cb *v1alpha1.CassandraBackup, cc *v1alpha1.CassandraCluster, relatedIcarusRestore icarus.Restore) error {
	newRestoreRequest := createRestoreReq(cc, cb, cr)
	if r.restoreConfigChanged(relatedIcarusRestore, newRestoreRequest) {
		r.Log.Info("Detected a configuration change for restore %s/%s, sending a new restore request", cb.Namespace, cb.Name)
		err := ic.Restore(ctx, newRestoreRequest)
		if err != nil {
			return err
		}

		cr.Status = v1alpha1.CassandraRestoreStatus{} // reset status since we're restarting restore in Icarus
		err = r.Status().Update(ctx, cr)
		if err != nil {
			return err
		}

		return nil
	} else {
		r.Log.Debugf("Backup %s/%s has failed. Assuming configration error. "+
			"Apply the fixed CassandraRestore configuration to trigger a new retry", cb.Namespace, cb.Name)
	}
	return nil
}
