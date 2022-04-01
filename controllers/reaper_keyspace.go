package controllers

import (
	"fmt"

	"github.com/google/go-cmp/cmp"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/pkg/errors"
)

func (r *CassandraClusterReconciler) reconcileReaperKeyspace(cc *v1alpha1.CassandraCluster, cqlClient cql.CqlClient, allDCs []v1alpha1.DC) error {
	keyspaces, err := cqlClient.GetKeyspacesInfo()
	if err != nil {
		return errors.Wrap(err, "failed to get keyspaces info")
	}

	replicationOptions := desiredReplicationOptions(cc, cc.Spec.Reaper.Keyspace, allDCs)
	reaperKeyspaces, found := getKeyspaceByName(keyspaces, cc.Spec.Reaper.Keyspace)
	if !found {
		query := fmt.Sprintf("CREATE KEYSPACE %s %s", cc.Spec.Reaper.Keyspace, cql.ReplicationQuery(replicationOptions))
		err := cqlClient.Query(query)
		if err != nil {
			return errors.Wrap(err, "failed to create reaper keyspace")
		}

		return nil
	}

	// even though the keyspaces management logic would update the rf settings to correct one,
	// on DC scaledown a removed DC can cause reaper to fail requests so need to update the keyspaces before that
	if !cmp.Equal(replicationOptions, reaperKeyspaces.Replication) {
		r.Log.Infof("updating reaper keyspace rf settings. Diff: %s", cmp.Diff(replicationOptions, reaperKeyspaces.Replication))
		query := fmt.Sprintf("ALTER KEYSPACE %s %s", cc.Spec.Reaper.Keyspace, cql.ReplicationQuery(replicationOptions))
		err := cqlClient.Query(query)
		if err != nil {
			return errors.Wrap(err, "failed to create reaper keyspace")
		}
	}
	return nil
}
