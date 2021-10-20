package controllers

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/reaper"
	"github.com/pkg/errors"
)

func (r *CassandraClusterReconciler) reconcileKeyspaces(ctx context.Context, cc *dbv1alpha1.CassandraCluster, cqlClient cql.CqlClient, reaperClient reaper.ReaperClient) error {
	keyspacesToReconcile := cc.Spec.SystemKeyspaces.Names
	if !keyspaceExists(keyspacesToReconcile, "system_auth") {
		keyspacesToReconcile = append(keyspacesToReconcile, "system_auth")
	}

	currentKeyspaces, err := cqlClient.GetKeyspacesInfo()
	if err != nil {
		return errors.Wrap(err, "can't get keyspace info")
	}

	for _, systemKeyspace := range keyspacesToReconcile {
		keyspaceInfo, found := getKeyspaceByName(currentKeyspaces, string(systemKeyspace))
		if !found {
			r.Log.Warnf("Keyspace %q doesn't exists", systemKeyspace)
			continue
		}

		desiredOptions := desiredReplicationOptions(cc)
		if !cmp.Equal(keyspaceInfo.Replication, desiredOptions) {
			r.Log.Infof("Updating keyspace %q with replication options %v", systemKeyspace, desiredOptions)
			err = cqlClient.UpdateRF(string(systemKeyspace), desiredOptions)
			if err != nil {
				return errors.Wrapf(err, "failed to alter %q keyspace", systemKeyspace)
			}
			r.Log.Infof("Done updating keyspace %q", systemKeyspace)

			r.Log.Infof("Repairing keyspace %s", string(systemKeyspace))
			err := reaperClient.RunRepair(ctx, string(systemKeyspace), repairCauseKeyspacesInit)
			if err != nil {
				return errors.Wrapf(err, "failed to run repair on %q keyspace", systemKeyspace)
			}
		} else {
			r.Log.Debugf("No updates to %q keyspace", systemKeyspace)
		}
	}

	return nil
}

func getKeyspaceByName(existingKeyspaces []cql.Keyspace, keyspaceName string) (cql.Keyspace, bool) {
	for _, keyspace := range existingKeyspaces {
		if keyspace.Name == keyspaceName {
			return keyspace, true
		}
	}

	return cql.Keyspace{}, false
}

func desiredReplicationOptions(cc *dbv1alpha1.CassandraCluster) map[string]string {
	options := make(map[string]string, len(cc.Spec.SystemKeyspaces.DCs))
	for _, dc := range cc.Spec.SystemKeyspaces.DCs {
		options[dc.Name] = fmt.Sprint(dc.RF)
	}
	options["class"] = cql.ReplicationClassNetworkTopologyStrategy

	return options
}

func keyspaceExists(keyspaces []dbv1alpha1.KeyspaceName, keyspaceName string) bool {
	for _, keyspace := range keyspaces {
		if string(keyspace) == keyspaceName {
			return true
		}
	}

	return false
}

// reconcileSystemAuthKeyspace reconciles the keyspace without starting the repair.
// Needed for initialization when the RF options need to be updated before reaper is available
func (r *CassandraClusterReconciler) reconcileSystemAuthKeyspace(cc *dbv1alpha1.CassandraCluster, cqlClient cql.CqlClient) error {
	currentKeyspaces, err := cqlClient.GetKeyspacesInfo()
	if err != nil {
		return errors.Wrap(err, "can't get keyspace info")
	}

	keyspaceInfo, found := getKeyspaceByName(currentKeyspaces, "system_auth")
	if !found {
		return errors.New("Keyspace system_auth doesn't exists")
	}

	desiredOptions := desiredReplicationOptions(cc)
	if !cmp.Equal(keyspaceInfo.Replication, desiredOptions) {
		r.Log.Infof("Updating keyspace system_auth with replication options %v", desiredOptions)
		err = cqlClient.UpdateRF("system_auth", desiredOptions)
		if err != nil {
			return errors.Wrap(err, "failed to alter system_auth keyspace")
		}
	}

	return nil
}
