package v1alpha1

import (
	"fmt"
	"github.com/ibm/cassandra-operator/controllers/util"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StorageProvider string

const (
	StorageProviderS3     StorageProvider = "s3"
	StorageProviderGCP    StorageProvider = "gcp"
	StorageProviderAzure  StorageProvider = "azure"
	StorageProviderMinio  StorageProvider = "minio"
	StorageProviderCeph   StorageProvider = "ceph"
	StorageProviderOracle StorageProvider = "oracle"
)

type CassandraBackupSpec struct {
	// CassandraCluster that is being backed up
	CassandraCluster string `json:"cassandraCluster"`
	// example: gcp://myBucket
	// location where SSTables will be uploaded.
	// A value of the storageLocation property has to have exact format which is 'protocol://bucket-name
	// protocol is either 'gcp', 's3', 'azure', 'minio', 'ceph' or 'oracle'.
	StorageLocation string `json:"storageLocation"`
	// Name of the secret from which credentials used for the communication to cloud storage providers are read.
	SecretName string `json:"secretName"`
	// Tag name that identifies the backup. Defaulted to the name of the CassandraBackup.
	SnapshotTag string `json:"snapshotTag,omitempty"`
	// Based on this field, there will be throughput per second computed based on what size data we want to upload we have.
	// The formula is "size / duration". The lower the duration is, the higher throughput per second we will need and vice versa.
	// This will influence e.g. responsiveness of a node to its business requests so one can control how much bandwidth is used for backup purposes in case a cluster is fully operational.
	// The format of this field is "amount unit". 'unit' is just a (case-insensitive) java.util.concurrent.TimeUnit enum value.
	// If not used, there will not be any restrictions as how fast an upload can be.
	Duration string `json:"duration,omitempty"`
	// bandwidth used during uploads
	Bandwidth *DataRate `json:"bandwidth,omitempty"`
	// number of threads used for upload, there might be at most so many uploading threads at any given time, when not set, it defaults to 10
	// +kubebuilder:validation:Minimum=1
	ConcurrentConnections int64 `json:"concurrentConnections,omitempty"`
	// name of datacenter to backup, nodes in the other datacenter(s) will not be involved
	DC string `json:"dc,omitempty"`
	// database entities to backup, it might be either only keyspaces or only tables (from different keyspaces if needed),
	// e.g. 'k1,k2' if one wants to backup whole keyspaces and 'ks1.t1,ks2,t2' if one wants to backup tables.
	// These formats can not be used together so 'k1,k2.t2' is invalid. If this field is empty, all keyspaces are backed up.
	Entities string `json:"entities,omitempty"`
	// number of hours to wait until backup is considered failed if not finished already
	// +kubebuilder:validation:Minimum=1
	Timeout int64 `json:"timeout,omitempty"`
	// Relevant during upload to S3-like bucket only. Specifies whether the metadata is copied from the source object or replaced with metadata provided in the request.
	// Defaults to COPY. Consult com.amazonaws.services.s3.model.MetadatDirective for more information.
	// +kubebuilder:validation:Enum=COPY;REPLACE
	MetadataDirective string `json:"metadataDirective,omitempty"`
	// Relevant during upload to S3-like bucket only. If true, communication is done via HTTP instead of HTTPS. Defaults to false.
	Insecure bool `json:"insecure,omitempty"`
	// Automatically creates a bucket if it does not exist. If a bucket does not exist, backup operation will fail. Defaults to false.
	CreateMissingBucket bool `json:"createMissingBucket,omitempty"`
	// Do not check the existence of a bucket.
	// Some storage providers (e.g. S3) requires a special permissions to be able to list buckets or query their existence which might not be allowed.
	// This flag will skip that check. Keep in mind that if that bucket does not exist, the whole backup operation will fail.
	SkipBucketVerification bool `json:"skipBucketVerification,omitempty"`
	// If set to true, refreshment of an object in a remote bucket (e.g. for s3) will be skipped.
	// This might help upon backuping to specific s3 storage providers like Dell ECS storage.
	// You will also skip versioning creating new versions when turned off as refreshment creates new version of files as a side effect.
	SkipRefreshing bool  `json:"skipRefreshing,omitempty"`
	Retry          Retry `json:"retry,omitempty"`
}

type Retry struct {
	// Defaults to false if not specified. If false, retry mechanism on upload / download operations in case they fail will not be used.
	Enabled bool `json:"enabled,omitempty"`
	// Time gap between retries, linear strategy will have always this gap constant, exponential strategy will make the gap bigger exponentially (power of 2) on each attempt
	// +kubebuilder:validation:Minimum=1
	Interval int64 `json:"interval,omitempty"`
	// Strategy how retry should be driven, might be either 'LINEAR' or 'EXPONENTIAL'
	// +kubebuilder:validation:Enum=LINEAR;EXPONENTIAL
	Strategy string `json:"strategy,omitempty"`
	// Number of repetitions of an upload / download operation in case it fails before giving up completely.
	// +kubebuilder:validation:Minimum=1
	MaxAttempts int64 `json:"maxAttempts,omitempty"`
}

type DataRate struct {
	// +kubebuilder:validation:Minimum=1
	Value int64 `json:"value"`
	// +kubebuilder:validation:Enum=BPS;KBPS;MBPS;GBPS
	Unit string `json:"unit"`
}

type CassandraBackupStatus struct {
	// The current state of the backup
	State string `json:"state,omitempty"`
	// Errors that occurred during backup process. Errors from all nodes are aggregated here
	Errors []BackupError `json:"errors,omitempty"`
	// A value from 0 to 100 indicating the progress of the backup as a percentage
	Progress int `json:"progress,omitempty"`
}

type BackupError struct {
	// Name of the node where the error occurred
	Source string `json:"source,omitempty"`
	// The error message
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// CassandraBackup is the Schema for the CassandraBackups API
type CassandraBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CassandraBackupSpec   `json:"spec"`
	Status CassandraBackupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CassandraBackupList contains a list of CassandraBackup
type CassandraBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CassandraBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CassandraBackup{}, &CassandraBackupList{})
}

func (in *CassandraBackup) StorageProvider() StorageProvider {
	return storageProvider(in.Spec.StorageLocation)
}

func storageProvider(storageLocation string) StorageProvider {
	if strings.HasPrefix(storageLocation, "gcp://") {
		return StorageProviderGCP
	}
	if strings.HasPrefix(storageLocation, "s3://") {
		return StorageProviderS3
	}
	if strings.HasPrefix(storageLocation, "azure://") {
		return StorageProviderAzure
	}
	if strings.HasPrefix(storageLocation, "oracle://") {
		return StorageProviderOracle
	}
	if strings.HasPrefix(storageLocation, "minio://") {
		return StorageProviderMinio
	}
	if strings.HasPrefix(storageLocation, "ceph://") {
		return StorageProviderCeph
	}

	return ""
}

func ValidateStorageSecret(logger *zap.SugaredLogger, secret *v1.Secret, storageProvider StorageProvider) error {
	if util.Contains([]string{
		string(StorageProviderS3),
		string(StorageProviderMinio),
		string(StorageProviderOracle),
		string(StorageProviderCeph),
	}, string(storageProvider)) {
		if len(secret.Data["awssecretaccesskey"]) == 0 {
			logger.Info(fmt.Sprintf("'awssecretaccesskey' key for secret %s is not set, "+
				"will try to use AWS compatible env vars to obtain credentials", secret.Name))
		}

		if len(secret.Data["awsaccesskeyid"]) == 0 {
			logger.Info(fmt.Sprintf("'awssecretaccesskey' key for secret %s is not set, "+
				"will try to use AWS compatible env vars to obtain credentials", secret.Name))
		}

		if len(secret.Data["awssecretaccesskey"]) != 0 && len(secret.Data["awsaccesskeyid"]) != 0 {
			if len(secret.Data["awsregion"]) == 0 {
				return fmt.Errorf("there is no 'awsregion' property "+
					"while you have set both 'awssecretaccesskey' and 'awsaccesskeyid in %s secret", secret.Name)
			}
		}

		if len(secret.Data["awsendpoint"]) != 0 && len(secret.Data["awsregion"]) == 0 {
			return fmt.Errorf("'awsendpoint' is specified but 'awsregion' is not set in %s secret", secret.Name)
		}
	}

	if storageProvider == StorageProviderGCP && len(secret.Data["gcp"]) == 0 {
		return fmt.Errorf("storage provider is GCP but key 'gpc' for secret %s is not set", secret.Name)
	}

	if storageProvider == StorageProviderAzure {
		if len(secret.Data["azurestorageaccount"]) == 0 {
			return fmt.Errorf("'azurestorageaccount' key for secret %s is not set", secret.Name)
		}

		if len(secret.Data["azurestoragekey"]) == 0 {
			return fmt.Errorf("'azurestoragekey' key for secret %s is not set", secret.Name)
		}
	}

	return nil
}
