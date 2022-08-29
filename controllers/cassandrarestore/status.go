package cassandrarestore

import (
	"context"

	"github.com/google/go-cmp/cmp"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/icarus"
)

func (r *CassandraRestoreReconciler) reconcileStatus(ctx context.Context, cr *v1alpha1.CassandraRestore, relatedIcarusRestore icarus.Restore) error {
	restoreStatus := cr.DeepCopy()
	restoreStatus.Status.Progress = int(relatedIcarusRestore.Progress * 100)
	if cr.Status.State != relatedIcarusRestore.State {
		restoreStatus.Status.State = relatedIcarusRestore.State
		if relatedIcarusRestore.State == icarus.StateFailed {
			var restoreErrors []v1alpha1.RestoreError
			for _, restoreError := range relatedIcarusRestore.Errors {
				restoreErrors = append(restoreErrors, v1alpha1.RestoreError{
					Source:  restoreError.Source,
					Message: restoreError.Message,
				})
			}

			restoreStatus.Status.Errors = restoreErrors
		}
	}

	if !cmp.Equal(cr.Status, restoreStatus.Status) {
		r.Log.Info("Updating restore status")
		r.Log.Debugf(cmp.Diff(cr.Status, restoreStatus.Status))
		err := r.Status().Update(ctx, restoreStatus)
		if err != nil {
			return err
		}
	}

	return nil
}
