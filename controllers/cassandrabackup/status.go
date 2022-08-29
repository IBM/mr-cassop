package cassandrabackup

import (
	"context"

	"github.com/google/go-cmp/cmp"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/icarus"
)

func (r *CassandraBackupReconciler) reconcileStatus(ctx context.Context, cb *v1alpha1.CassandraBackup, relatedIcarusBackup icarus.Backup) error {
	backupStatus := cb.DeepCopy()
	//found, update state
	backupStatus.Status.Progress = int(relatedIcarusBackup.Progress * 100)
	if cb.Status.State != relatedIcarusBackup.State {
		backupStatus.Status.State = relatedIcarusBackup.State
		if relatedIcarusBackup.State == icarus.StateFailed {
			var backupErrors []v1alpha1.BackupError
			for _, backupError := range relatedIcarusBackup.Errors {
				backupErrors = append(backupErrors, v1alpha1.BackupError{
					Source:  backupError.Source,
					Message: backupError.Message,
				})
			}

			backupStatus.Status.Errors = backupErrors
		}
	}

	if !cmp.Equal(cb.Status, backupStatus.Status) {
		r.Log.Info("Updating backup status")
		r.Log.Debugf(cmp.Diff(cb.Status, backupStatus.Status))
		err := r.Status().Update(ctx, backupStatus)
		if err != nil {
			return err
		}
	}

	return nil
}
