package controllers

import (
	"fmt"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/nodetool"
	"github.com/pkg/errors"
)

func (r *CassandraClusterReconciler) reconcileRFSettings(cc *dbv1alpha1.CassandraCluster, cqlClient cql.CqlClient, ntClient nodetool.NodetoolClient) error {
	rfSettingsChanged, err := r.rfSettingsChanged(cc, cqlClient)
	if err != nil {
		return errors.Wrapf(err, "failed to get info if rf settings changed")
	}

	if !rfSettingsChanged {
		r.Log.Debug("RF settings haven't changed")
		return nil
	}

	r.Log.Debug("Executing query to update RF info")
	if err := cqlClient.UpdateRF(cc); err != nil {
		return errors.Wrap(err, "Can't update RF info")
	}

	r.Log.Debug("Repairing system_auth keyspace")
	if err := ntClient.RepairKeyspace(cc, "system_auth"); err != nil {
		return errors.Wrapf(err, "Failed to repair %q keyspace", "system_auth")
	}

	return nil
}

func (r CassandraClusterReconciler) rfSettingsChanged(cc *dbv1alpha1.CassandraCluster, cqlClient cql.CqlClient) (bool, error) {
	keyspaces, err := cqlClient.GetKeyspacesInfo()
	if err != nil {
		return false, errors.Wrapf(err, "can't get keyspaces info")
	}

	var systemAuthKeyspace cql.Keyspace
	for _, keyspace := range keyspaces {
		if keyspace.Name == "system_auth" {
			systemAuthKeyspace = keyspace
			break
		}
	}

	if systemAuthKeyspace.Name == "" {
		return false, errors.New("system_auth keyspace not found")
	}

	for _, dc := range cc.Spec.SystemKeyspaces.DCs {
		if rf, found := systemAuthKeyspace.Replication[dc.Name]; found && fmt.Sprintf("%d", dc.RF) == rf {
			continue
		}
		return true, nil
	}

	return false, nil
}
