---
title: Cassandra backup and restore
slug: /cassandra-backup-restore
---

The Cassandra operator uses [Icarus](https://github.com/instaclustr/icarus) to perform backups and restores. 
Refer to its documentation for more details on how the backup and restore procedures work internally.

Backup and restore creation and configuration is done by creating CassandraBackup and CassandraRestore custom resources.

S3, Azure and GCP storage providers are supported. The type is determined by the `storageLocation` field, which should be in the following format:
`protocol://backup/location`. So an S3 provider would look like to following: `s3://location/to/the/backup`

To provide credentials, the `secretName` field should be used to refer to a secret in the following format:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cassandra-backup-restore-secret-cluster-my-cluster
type: Opaque
stringData:
  awssecretaccesskey: _AWS secret key_
  awsaccesskeyid: _AWS access id_
  awsregion: e.g. eu-central-1
  awsendpoint: endpoint
  azurestorageaccount: _Azure storage account_
  azurestoragekey: _Azure storage key_
  gcp: 'whole json with service account'
```

Only fields for a particular provider used should be set.

### CassandraBackup

To create a backup simply create a CassandraBackup resource:

```yaml
apiVersion: db.ibm.com/v1alpha1
kind: CassandraBackup
metadata:
  name: example-backup
spec:
  cassandraCluster: test-cluster
  storageLocation: s3://bucket-name/backup/location
  secretName: backup-restore-credentials
```

To track progress of the backup process you can see the status of the object, where you can see the state, progress and other information about the backup. If a backup failed you'll see the errors in the status object as well.

See [all fields description](cassandrabackup-configuration.md) for more information

#### Restarting a failed backup

If a misconfigured backup has failed, the operator will retry only when a configuration is changed. If a retry is needed without a configuration change, simply recreate the resource.

### CassandraRestore

To restore a backup a CassandraRestore should be created which will start the restore process.

The backup can be referenced either by setting the corresponding CassandraBackup resource name or by manually setting the `storageLocation` and `snapshotTag` fields.

```yaml
apiVersion: db.ibm.com/v1alpha1
kind: CassandraRestore
metadata:
  name: rest3
spec:
    cassandraCluster: test-cluster
    cassandraBackup: example-backup
    # or the following if no corresponding cassandraBackup available
    # storageLocation: s3://bucket-name/backup/location
    # snapshotTag: example-backup //the name of the CassandraBackup if the backup was created using the Cassandra Operator
```

The Cassandra Operator will update the progress of the restore in the status field of CassandraRestores CR object.

See [all fields description](cassandrarestore-configuration.md) for more information.