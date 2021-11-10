package controllers

import (
	"fmt"
	"strconv"

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

	replicationOptions := desiredReaperReplicationOptions(cc, allDCs)
	reaperKeyspace, found := getKeyspaceByName(keyspaces, cc.Spec.Reaper.Keyspace)
	if !found {
		query := fmt.Sprintf("CREATE KEYSPACE %s %s", cc.Spec.Reaper.Keyspace, cql.ReplicationQuery(replicationOptions))
		err := cqlClient.Query(query)
		if err != nil {
			return errors.Wrap(err, "failed to create reaper keyspace")
		}

		return nil
	}

	if cmp.Equal(reaperKeyspace.Replication, replicationOptions) {
		return nil
	}
	r.Log.Infof("Updating %s keyspace", cc.Spec.Reaper.Keyspace)
	r.Log.Debug(cmp.Diff(reaperKeyspace.Replication, replicationOptions))

	err = cqlClient.Query(fmt.Sprintf("ALTER KEYSPACE %s %s ;", cc.Spec.Reaper.Keyspace, cql.ReplicationQuery(replicationOptions)))
	if err != nil {
		return errors.Wrap(err, "failed to update reaper keyspace")
	}

	return nil
}

func desiredReaperReplicationOptions(cc *v1alpha1.CassandraCluster, allDCs []v1alpha1.DC) map[string]string {
	options := make(map[string]string, len(cc.Spec.Reaper.DCs))

	if cc.Spec.Reaper != nil && len(cc.Spec.Reaper.DCs) > 0 {
		for _, dc := range cc.Spec.Reaper.DCs {
			options[dc.Name] = fmt.Sprint(*dc.Replicas)
		}
	} else {
		for _, dc := range allDCs {
			rf := strconv.Itoa(defaultRF)
			if dc.Replicas != nil && *dc.Replicas < defaultRF {
				rf = strconv.Itoa(int(*dc.Replicas))
			}
			options[dc.Name] = rf
		}
	}
	options["class"] = cql.ReplicationClassNetworkTopologyStrategy

	return options
}
