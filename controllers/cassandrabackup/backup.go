package cassandrabackup

import (
	"context"
	"strings"
	"time"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/icarus"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *CassandraBackupReconciler) reconcileBackup(ctx context.Context, ic icarus.Icarus, cb *v1alpha1.CassandraBackup, cc *v1alpha1.CassandraCluster) (ctrl.Result, error) {
	existingBackups, err := ic.Backups(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	icarusBackup, relatedIcarusBackupFound := r.findRelatedBackup(cb, existingBackups)

	if cb.Status.State == icarus.StateFailed {
		if !relatedIcarusBackupFound {
			r.Log.Infof("Backup %s/%s has failed and no respecting backup record found in icarus. "+
				"Recreate the CassandraBackup resource to start a new backup attempt", cb.Namespace, cb.Name)
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, r.reconcileFailedBackup(ctx, ic, icarusBackup, cc, cb)
	}

	// create if not found or found, but it's a new backup with the same tag (e.g. incremental backup)
	if !relatedIcarusBackupFound || (relatedIcarusBackupFound && len(cb.Status.State) == 0) {
		icarusBackup, err = ic.Backup(ctx, createBackupRequest(cc, cb))
		if err != nil {
			return ctrl.Result{}, err
		}

		r.Log.Debugf("Backup request sent")
	}

	err = r.reconcileStatus(ctx, cb, icarusBackup)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
}

func (r *CassandraBackupReconciler) findRelatedBackup(cb *v1alpha1.CassandraBackup, icarusBackups []icarus.Backup) (icarus.Backup, bool) {
	if len(icarusBackups) == 0 {
		return icarus.Backup{}, false
	}

	tagName := cb.Spec.SnapshotTag
	if len(tagName) == 0 {
		tagName = cb.Name
	}
	var relatedBackups []icarus.Backup //there may be several related backups if we retried a failed one
	for _, item := range icarusBackups {
		if !strings.Contains(item.SnapshotTag, tagName+"-"+item.SchemaVersion) && tagName != item.SnapshotTag {
			continue
		}

		if !item.GlobalRequest { //only look at coordinator requests
			continue
		}
		relatedBackups = append(relatedBackups, item)
	}

	if len(relatedBackups) == 0 {
		return icarus.Backup{}, false
	}

	var existingBackup icarus.Backup
	if len(relatedBackups) == 1 {
		existingBackup = relatedBackups[0]
	} else { // more than one related backups
		for i, relatedBackup := range relatedBackups {
			candidateCreatedTime, err := time.Parse(time.RFC3339, relatedBackup.CreationTime)
			if err != nil {
				r.Log.Warnf("Couldn't parse creation time for backup with id %s. Skipping it", relatedBackup.ID)
				continue
			}
			if len(existingBackup.CreationTime) == 0 {
				existingBackup = relatedBackups[i]
				continue
			}

			existingBackupCreatedTime, err := time.Parse(time.RFC3339, existingBackup.CreationTime)
			if err != nil {
				r.Log.Warnf("skipping backup with ID %s and tag %s. Couldn't parse create time to determine if it's the most recent backup: %s",
					relatedBackup.ID, relatedBackup.SnapshotTag, err.Error())
				continue
			}
			if existingBackupCreatedTime.Before(candidateCreatedTime) {
				//this one is created later, use it
				existingBackup = relatedBackups[i]
			}
		}
	}

	return existingBackup, true
}

func (r *CassandraBackupReconciler) reconcileFailedBackup(ctx context.Context, ic icarus.Icarus, existingBackup icarus.Backup,
	cc *v1alpha1.CassandraCluster, cb *v1alpha1.CassandraBackup) error {
	newBackupRequest := createBackupRequest(cc, cb)
	if r.backupConfigChanged(existingBackup, newBackupRequest) {
		r.Log.Info("Detected a configuration change for backup %s/%s, sending a new backup request", cb.Namespace, cb.Name)
		icarusBackup, err := ic.Backup(ctx, newBackupRequest)
		if err != nil {
			return err
		}

		cb.Status = v1alpha1.CassandraBackupStatus{} // reset status since we're restarting backup in Icarus
		err = r.reconcileStatus(ctx, cb, icarusBackup)
		if err != nil {
			return err
		}
	} else {
		r.Log.Infof("Backup %s/%s has failed. Assuming configuration error. "+
			"Apply the fixed CassandraBackup configuration to trigger a new retry", cb.Namespace, cb.Name)
	}

	return nil
}
