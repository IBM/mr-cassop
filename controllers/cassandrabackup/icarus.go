package cassandrabackup

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/icarus"
)

func createBackupRequest(cc *v1alpha1.CassandraCluster, backup *v1alpha1.CassandraBackup) icarus.BackupRequest {
	storageLocation := backup.Spec.StorageLocation
	if storageLocation[len(storageLocation):] != "/" {
		storageLocation += "/"
	}

	storageLocation = fmt.Sprintf("%s%s/%s/1", storageLocation, cc.Name, cc.Spec.DCs[0].Name)
	tagName := backup.Spec.SnapshotTag
	if len(tagName) == 0 {
		tagName = backup.Name
	}
	backupRequest := icarus.BackupRequest{
		Type:                   "backup",
		StorageLocation:        storageLocation,
		DataDirs:               []string{"/var/lib/cassandra/data"},
		GlobalRequest:          true,
		SnapshotTag:            tagName,
		K8sNamespace:           backup.Namespace,
		K8sSecretName:          backup.Spec.SecretName,
		ConcurrentConnections:  backup.Spec.ConcurrentConnections,
		DC:                     backup.Spec.DC,
		Entities:               backup.Spec.Entities,
		Timeout:                backup.Spec.Timeout,
		Duration:               backup.Spec.Duration,
		MetadataDirective:      backup.Spec.MetadataDirective,
		Insecure:               backup.Spec.Insecure,
		CreateMissingBucket:    backup.Spec.CreateMissingBucket,
		SkipBucketVerification: backup.Spec.SkipBucketVerification,
		SkipRefreshing:         backup.Spec.SkipRefreshing,
		Retry: icarus.Retry{
			Interval:    backup.Spec.Retry.Interval,
			Strategy:    backup.Spec.Retry.Strategy,
			MaxAttempts: backup.Spec.Retry.MaxAttempts,
			Enabled:     backup.Spec.Retry.Enabled,
		},
	}

	if backup.Spec.Bandwidth != nil && backup.Spec.Bandwidth.Value > 0 {
		backupRequest.Bandwidth = &icarus.DataRate{
			Value: backup.Spec.Bandwidth.Value,
			Unit:  backup.Spec.Bandwidth.Unit,
		}
	}

	if backupRequest.ConcurrentConnections == 0 {
		backupRequest.ConcurrentConnections = 10
	}

	if backupRequest.Timeout == 0 {
		backupRequest.Timeout = 5
	}

	if backupRequest.MetadataDirective == "" {
		backupRequest.MetadataDirective = "COPY"
	}

	if backupRequest.Retry.Interval == 0 {
		backupRequest.Retry.Interval = 10
	}

	if backupRequest.Retry.Strategy == "" {
		backupRequest.Retry.Strategy = "LINEAR"
	}

	if backupRequest.Retry.MaxAttempts == 0 {
		backupRequest.Retry.MaxAttempts = 3
	}

	return backupRequest
}

func (r *CassandraBackupReconciler) backupConfigChanged(existingBackup icarus.Backup, backupReq icarus.BackupRequest) bool {
	oldReq := icarus.BackupRequest{
		Type:                   "backup",
		StorageLocation:        existingBackup.StorageLocation,
		DataDirs:               existingBackup.DataDirs,
		GlobalRequest:          true,
		SnapshotTag:            existingBackup.SnapshotTag,
		K8sNamespace:           existingBackup.K8sNamespace,
		K8sSecretName:          existingBackup.K8sSecretName,
		Duration:               existingBackup.Duration,
		Bandwidth:              existingBackup.Bandwidth,
		ConcurrentConnections:  existingBackup.ConcurrentConnections,
		DC:                     existingBackup.DC,
		Entities:               existingBackup.Entities,
		Timeout:                existingBackup.Timeout,
		MetadataDirective:      existingBackup.MetadataDirective,
		Insecure:               existingBackup.Insecure,
		CreateMissingBucket:    existingBackup.CreateMissingBucket,
		SkipRefreshing:         existingBackup.SkipRefreshing,
		SkipBucketVerification: existingBackup.SkipBucketVerification,
		Retry: icarus.Retry{
			Interval:    existingBackup.Retry.Interval,
			MaxAttempts: existingBackup.Retry.MaxAttempts,
			Enabled:     existingBackup.Retry.Enabled,
			Strategy:    existingBackup.Retry.Strategy,
		},
	}

	cmpIgnoreFields := cmpopts.IgnoreFields(icarus.BackupRequest{}, "SnapshotTag")
	if !cmp.Equal(oldReq, backupReq, cmpIgnoreFields) {
		r.Log.Debugf(cmp.Diff(oldReq, backupReq, cmpIgnoreFields))
		return true
	}

	return false
}
