package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type CassandraRestoreSpec struct {
	CassandraCluster string `json:"cassandraCluster"`
	CassandraBackup  string `json:"cassandraBackup,omitempty"`
	// example: gcp://myBucket
	// location of SSTables
	// A value of the storageLocation property has to have exact format which is 'protocol://bucket-name
	// protocol is either 'gcp', 's3', 'azure', 'minio', 'ceph' or 'oracle'.
	// If empty, the value is retrieved from the CassandraBackup spec
	StorageLocation string `json:"storageLocation,omitempty"`
	// Name of the snapshot tag to restore. Can be used to manually set the snapshot tag. Retrieved from CassandraBackup if not specified
	SnapshotTag string `json:"snapshotTag,omitempty"`
	// Name of the secret from which credentials used for the communication to cloud storage providers are read.
	// The secret from the backup spec is used when empty
	SecretName string `json:"secretName,omitempty"`
	// number of threads used for download, there might be at most so many downloading threads at any given time,
	// when not set, it defaults to 10
	// +kubebuilder:validation:Minimum=1
	ConcurrentConnections int64 `json:"concurrentConnections,omitempty"`
	// Name of datacenter(s) against which restore will be done. It means that nodes in a different DC will not receive restore requests.
	// Multiple dcs are separated by comma
	DC string `json:"dc,omitempty"`
	// database entities to backup, it might be either only keyspaces or only tables (from different keyspaces if needed),
	// e.g. 'k1,k2' if one wants to backup whole keyspaces and 'ks1.t1,ks2,t2' if one wants to backup tables.
	// These formats can not be used together so 'k1,k2.t2' is invalid. If this field is empty, all keyspaces are backed up.
	Entities string `json:"entities,omitempty"`
	// flag saying if we should not delete truncated SSTables after they are imported, as part of CLEANUP phase, defaults to false
	NoDeleteTruncates bool `json:"noDeleteTruncates,omitempty"`
	// flag saying if we should not delete downloaded SSTables from remote location, as part of CLEANUP phase, defaults to false
	NoDeleteDownloads bool `json:"noDeleteDownloads,omitempty"`
	// flag saying if we should not download data from remote location as we expect them to be there already, defaults to false,
	// setting this to true has sense only in case noDeleteDownloads was set to true in previous restoration requests
	NoDownloadData bool `json:"noDownloadData,omitempty"`
	// object used upon restoration,
	// keyspace and table fields do not need to be set when restoration strategy type is IMPORT or HARDLINKS as this object will be initialised for each entities entry with right keyspace and table.
	// 'sourceDir' property is used for pointing to a directory where we expect to find downloaded SSTables.
	// This in turn means that all SSTables and other meta files will be downloaded into this directory (from which they will be fed to CFSMB).
	// All other fields are taken from ColumnFamilyStoreMBean#importNewSSTables
	Import RestoreImport `json:"import,omitempty"`
	// number of hours to wait until restore is considered failed if not finished already
	// +kubebuilder:validation:Minimum=1
	Timeout int64 `json:"timeout,omitempty"`
	// if set to true, host id of node to restore will be resolved from remote topology file located in a bucket by translating it from provided nodeId of storageLocation field
	ResolveHostIdFromTopology bool `json:"resolveHostIdFromTopology,omitempty"`
	// Relevant during upload to S3-like bucket only. If true, communication is done via HTTP instead of HTTPS. Defaults to false.
	Insecure bool `json:"insecure,omitempty"`
	// Do not check the existence of a bucket.
	// Some storage providers (e.g. S3) requires a special permissions to be able to list buckets or query their existence which might not be allowed.
	// This flag will skip that check. Keep in mind that if that bucket does not exist, the whole backup operation will fail.
	SkipBucketVerification bool  `json:"skipBucketVerification,omitempty"`
	Retry                  Retry `json:"retry,omitempty"`
	// Map of key and values where keys and values are in format "keyspace.table", if key is "ks1.tb1" and value is "ks1.tb2",
	// it means that upon restore, table ks1.tb1 will be restored into table ks1.tb2.
	// This in practice means that table ks1.tb2 will be truncated and populated with data from ks1.tb1.
	// The source table, ks1.tb1, will not be touched. It is expected that user knows that schema of both tables is compatible.
	// There is not any check done in this regard.
	Rename map[string]string `json:"rename,omitempty"`
	// version of schema we want to restore from.
	// Upon backup, a schema version is automatically appended to snapshot name and its manifest is uploaded under that name (plus timestamp at the end).
	// In case we have two snapshots having same name, we might distinguish between them by this schema version.
	// If schema version is not specified, we expect that there will be one and only one backup taken with respective snapshot name.
	// This schema version has to match the version of a Cassandra nodes.
	SchemaVersion string `json:"schemaVersion,omitempty"`
	// flag saying if we indeed want a schema version of a running node match with schema version a snapshot is taken on.
	// There might be cases when we want to restore a table for which its CQL schema has not changed,
	// but it has changed for other table / keyspace but a schema for that node has changed by doing that.
	ExactSchemaVersion bool `json:"exactSchemaVersion,omitempty"`
}

type RestoreImport struct {
	KeepLevel          bool `json:"keepLevel,omitempty"`
	NoVerify           bool `json:"noVerify,omitempty"`
	NoVerifyTokens     bool `json:"noVerifyTokens,omitempty"`
	NoInvalidateCaches bool `json:"noInvalidateCaches,omitempty"`
	Quick              bool `json:"quick,omitempty"`
	ExtendedVerify     bool `json:"extendedVerify,omitempty"`
	KeepRepaired       bool `json:"keepRepaired,omitempty"`
}

type CassandraRestoreStatus struct {
	State    string         `json:"state,omitempty"`
	Progress int            `json:"progress,omitempty"`
	Errors   []RestoreError `json:"errors,omitempty"`
}

type RestoreError struct {
	Source  string `json:"source,omitempty"`
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// CassandraRestore is the Schema for the CassandraRestores API
type CassandraRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CassandraRestoreSpec   `json:"spec"`
	Status CassandraRestoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CassandraRestoreList contains a list of CassandraRestore
type CassandraRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CassandraRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CassandraRestore{}, &CassandraRestoreList{})
}

func (in *CassandraRestore) StorageProvider() StorageProvider {
	return storageProvider(in.Spec.StorageLocation)
}
