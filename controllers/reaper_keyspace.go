package controllers

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/pkg/errors"
)

func (r *CassandraClusterReconciler) reconcileReaperKeyspace(cc *v1alpha1.CassandraCluster, cqlClient cql.CqlClient) error {
	keyspaces, err := cqlClient.GetKeyspacesInfo()
	if err != nil {
		return errors.Wrap(err, "failed to get keyspaces info")
	}

	replicationOptions := desiredReaperReplicationOptions(cc)
	reaperKeyspace, found := getKeyspaceByName(keyspaces, cc.Spec.Reaper.Keyspace)
	if !found {
		query := fmt.Sprintf("CREATE KEYSPACE %s %s", cc.Spec.Reaper.Keyspace, cql.ReplicationQuery(replicationOptions))
		err := cqlClient.Query(query)
		if err != nil {
			return errors.Wrap(err, "failed to create reaper keyspace")
		}
	}

	if cmp.Equal(reaperKeyspace.Replication, replicationOptions) {
		return nil
	}
	r.Log.Infof("Updating %s keyspace", cc.Spec.Reaper.Keyspace)

	err = cqlClient.Query(fmt.Sprintf("ALTER KEYSPACE %s %s ;", cc.Spec.Reaper.Keyspace, cql.ReplicationQuery(replicationOptions)))
	if err != nil {
		return errors.Wrap(err, "failed to update reaper keyspace")
	}

	return nil
}

func desiredReaperReplicationOptions(cc *v1alpha1.CassandraCluster) map[string]string {
	options := make(map[string]string, len(cc.Spec.Reaper.DCs))
	for _, dc := range cc.Spec.Reaper.DCs {
		options[dc.Name] = fmt.Sprint(*dc.Replicas)
	}
	options["class"] = cql.ReplicationClassNetworkTopologyStrategy

	return options
}
