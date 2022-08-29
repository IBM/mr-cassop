package cassandrarestore

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/icarus"
)

func createRestoreReq(cc *v1alpha1.CassandraCluster, backup *v1alpha1.CassandraBackup, restore *v1alpha1.CassandraRestore) icarus.RestoreRequest {
	storageLocation := restore.Spec.StorageLocation
	if len(storageLocation) == 0 {
		storageLocation = backup.Spec.StorageLocation
	}
	if storageLocation[len(storageLocation):] != "/" {
		storageLocation += "/"
	}

	storageLocation = fmt.Sprintf("%s%s/%s/1", storageLocation, cc.Name, cc.Spec.DCs[0].Name)
	snapshotTag := restore.Spec.SnapshotTag
	if len(snapshotTag) == 0 {
		snapshotTag = backup.Name
	}

	secretName := restore.Spec.SecretName
	if len(secretName) == 0 {
		secretName = backup.Spec.SecretName
	}

	restoreReq := icarus.RestoreRequest{
		Type:                    "restore",
		DC:                      restore.Spec.DC,
		StorageLocation:         storageLocation,
		SnapshotTag:             snapshotTag,
		DataDirs:                []string{"/var/lib/cassandra/data"},
		GlobalRequest:           true,
		RestorationStrategyType: "HARDLINKS",
		RestorationPhase:        "INIT",
		Import: icarus.RestoreImport{
			Type:               "import",
			SourceDir:          "file:///var/lib/cassandra/downloadedsstables",
			KeepLevel:          restore.Spec.Import.KeepLevel,
			NoVerify:           restore.Spec.Import.NoVerify,
			NoVerifyTokens:     restore.Spec.Import.NoVerifyTokens,
			NoInvalidateCaches: restore.Spec.Import.NoInvalidateCaches,
			Quick:              restore.Spec.Import.Quick,
			ExtendedVerify:     restore.Spec.Import.ExtendedVerify,
			KeepRepaired:       restore.Spec.Import.KeepRepaired,
		},
		K8sNamespace:  restore.Namespace,
		K8sSecretName: secretName,
		Entities:      restore.Spec.Entities,
		SinglePhase:   false,
		Retry: icarus.Retry{
			Interval:    restore.Spec.Retry.Interval,
			Strategy:    restore.Spec.Retry.Strategy,
			MaxAttempts: restore.Spec.Retry.MaxAttempts,
			Enabled:     restore.Spec.Retry.Enabled,
		},
		ConcurrentConnections:     restore.Spec.ConcurrentConnections,
		SkipBucketVerification:    restore.Spec.SkipBucketVerification,
		Timeout:                   restore.Spec.Timeout,
		NoDeleteDownloads:         restore.Spec.NoDeleteDownloads,
		NoDeleteTruncates:         restore.Spec.NoDeleteTruncates,
		NoDownloadData:            restore.Spec.NoDownloadData,
		Insecure:                  restore.Spec.Insecure,
		Rename:                    restore.Spec.Rename,
		ResolveHostIdFromTopology: restore.Spec.ResolveHostIdFromTopology,
		ExactSchemaVersion:        restore.Spec.ExactSchemaVersion,
		SchemaVersion:             restore.Spec.SchemaVersion,
	}

	if restoreReq.ConcurrentConnections == 0 {
		restoreReq.ConcurrentConnections = 10
	}

	if restoreReq.Timeout == 0 {
		restoreReq.Timeout = 5
	}

	if restoreReq.Retry.Interval == 0 {
		restoreReq.Retry.Interval = 10
	}

	if restoreReq.Retry.MaxAttempts == 0 {
		restoreReq.Retry.MaxAttempts = 3
	}

	if len(restoreReq.Retry.Strategy) == 0 {
		restoreReq.Retry.Strategy = "LINEAR"
	}

	if restoreReq.Rename == nil {
		restoreReq.Rename = make(map[string]string)
	}

	return restoreReq
}

func (r *CassandraRestoreReconciler) restoreConfigChanged(existingRestore icarus.Restore, restoreReq icarus.RestoreRequest) bool {
	oldReq := icarus.RestoreRequest{
		Type:                   "restore",
		StorageLocation:        existingRestore.StorageLocation,
		DataDirs:               existingRestore.DataDirs,
		GlobalRequest:          true,
		SnapshotTag:            existingRestore.SnapshotTag,
		K8sNamespace:           existingRestore.K8sNamespace,
		K8sSecretName:          existingRestore.K8sSecretName,
		ConcurrentConnections:  existingRestore.ConcurrentConnections,
		DC:                     existingRestore.DC,
		Entities:               existingRestore.Entities,
		Timeout:                existingRestore.Timeout,
		Insecure:               existingRestore.Insecure,
		SkipBucketVerification: existingRestore.SkipBucketVerification,
		Retry: icarus.Retry{
			Interval:    existingRestore.Retry.Interval,
			MaxAttempts: existingRestore.Retry.MaxAttempts,
			Enabled:     existingRestore.Retry.Enabled,
			Strategy:    existingRestore.Retry.Strategy,
		},
		ResolveHostIdFromTopology: existingRestore.ResolveHostIdFromTopology,
		Rename:                    existingRestore.Rename,
		NoDownloadData:            existingRestore.NoDownloadData,
		NoDeleteTruncates:         existingRestore.NoDeleteTruncates,
		NoDeleteDownloads:         existingRestore.NoDeleteDownloads,
		SinglePhase:               existingRestore.SinglePhase,
		Import:                    existingRestore.Import,
		RestorationPhase:          existingRestore.RestorationPhase,
		RestorationStrategyType:   existingRestore.RestorationStrategyType,
	}

	cmpIgnoreFields := cmpopts.IgnoreFields(icarus.RestoreRequest{}, "SnapshotTag")
	if !cmp.Equal(oldReq, restoreReq, cmpIgnoreFields) {
		r.Log.Debugf(cmp.Diff(oldReq, restoreReq, cmpIgnoreFields))
		return true
	}

	return false
}
