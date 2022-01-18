package controllers

import (
	"context"
	"fmt"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/google/go-cmp/cmp"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/reaper"
	"github.com/pkg/errors"
)

const (
	defaultRF                 = 3
	keyspaceSystemAuth        = "system_auth"
	keyspaceSystemDistributed = "system_distributed"
	keyspaceSystemTraces      = "system_traces"
)

func (r *CassandraClusterReconciler) reconcileKeyspaces(ctx context.Context, cc *dbv1alpha1.CassandraCluster, cqlClient cql.CqlClient, reaperClient reaper.ReaperClient, allDCs []dbv1alpha1.DC) error {
	keyspacesToReconcile := desiredKeyspacesToReconcile(cc)

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

		desiredOptions := desiredReplicationOptions(cc, string(systemKeyspace), allDCs)
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

func desiredReplicationOptions(cc *dbv1alpha1.CassandraCluster, keyspace string, allDCs []dbv1alpha1.DC) map[string]string {
	options := make(map[string]string, len(cc.Spec.SystemKeyspaces.DCs))

	if len(cc.Spec.SystemKeyspaces.DCs) > 0 && keyspaceExists(cc.Spec.SystemKeyspaces.Names, keyspace) {
		for _, dc := range cc.Spec.SystemKeyspaces.DCs {
			options[dc.Name] = fmt.Sprint(dc.RF)
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

func desiredKeyspacesToReconcile(cc *dbv1alpha1.CassandraCluster) []dbv1alpha1.KeyspaceName {
	keyspacesToReconcile := []dbv1alpha1.KeyspaceName{
		keyspaceSystemAuth,
		keyspaceSystemDistributed,
		keyspaceSystemTraces,
		dbv1alpha1.KeyspaceName(cc.Spec.Reaper.Keyspace),
	}

	if len(cc.Spec.SystemKeyspaces.Names) == 0 {
		return keyspacesToReconcile
	}

	for _, desiredKeyspace := range cc.Spec.SystemKeyspaces.Names {
		if !keyspaceExists(keyspacesToReconcile, string(desiredKeyspace)) {
			keyspacesToReconcile = append(keyspacesToReconcile, desiredKeyspace)
		}
	}

	return keyspacesToReconcile
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
func (r *CassandraClusterReconciler) reconcileSystemAuthKeyspace(ctx context.Context, cc *dbv1alpha1.CassandraCluster, cqlClient cql.CqlClient, allDCs []dbv1alpha1.DC) error {
	currentKeyspaces, err := cqlClient.GetKeyspacesInfo()
	if err != nil {
		return errors.Wrap(err, "can't get keyspace info")
	}

	keyspaceInfo, found := getKeyspaceByName(currentKeyspaces, keyspaceSystemAuth)
	if !found {
		return errors.Errorf("keyspace %s doesn't exist", keyspaceSystemAuth)
	}

	desiredOptions := desiredReplicationOptions(cc, keyspaceSystemAuth, allDCs)
	if !cmp.Equal(keyspaceInfo.Replication, desiredOptions) {
		r.Log.Infof("Updating keyspace %s with replication options %v", keyspaceSystemAuth, desiredOptions)
		err = cqlClient.UpdateRF(keyspaceSystemAuth, desiredOptions)
		if err != nil {
			return errors.Wrapf(err, "failed to alter %s keyspace", keyspaceSystemAuth)
		}

		// reaper may be already running (in case of adding a new DC) so try to run a repair for the updated keyspace
		reaperClient := r.ReaperClient(reaperServiceURL(cc), cc.Name, cc.Spec.Reaper.RepairThreadCount)
		if isRunning, err := reaperClient.IsRunning(ctx); err == nil && isRunning {
			r.Log.Infof("Running repair for keyspace system_auth")
			err := reaperClient.RunRepair(ctx, keyspaceSystemAuth, repairCauseKeyspacesInit)
			if err != nil {
				return errors.Wrap(err, "Can't run repair for system_auth keyspace")
			}
		}
	}

	return nil
}

func (r CassandraClusterReconciler) reconcileSystemAuthIfReady(ctx context.Context, cc *dbv1alpha1.CassandraCluster, allDCs []dbv1alpha1.DC) error {
	adminRoleSecret := &v1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: cc.Spec.AdminRoleSecretName}, adminRoleSecret)
	if err != nil {
		return err
	}
	cassandraOperatorAdminRole := string(adminRoleSecret.Data[dbv1alpha1.CassandraOperatorAdminRole])
	cassandraOperatorAdminPassword := string(adminRoleSecret.Data[dbv1alpha1.CassandraOperatorAdminPassword])
	cqlClient, err := r.CqlClient(newCassandraConfig(cc, cassandraOperatorAdminRole, cassandraOperatorAdminPassword))
	if err == nil { // if the current region is the ready one it will succeed
		defer cqlClient.CloseSession()
		err = r.reconcileSystemAuthKeyspace(ctx, cc, cqlClient, allDCs)
		if err != nil {
			return err
		}
	}

	return nil
}
