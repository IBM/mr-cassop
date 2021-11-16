package controllers

import (
	"fmt"

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
	_, found := getKeyspaceByName(keyspaces, cc.Spec.Reaper.Keyspace)
	if !found {
		query := fmt.Sprintf("CREATE KEYSPACE %s %s", cc.Spec.Reaper.Keyspace, cql.ReplicationQuery(replicationOptions))
		err := cqlClient.Query(query)
		if err != nil {
			return errors.Wrap(err, "failed to create reaper keyspace")
		}

		return nil
	}

	// no need to update keyspaces now, they will be updated by keyspace management if needed
	return nil
}
