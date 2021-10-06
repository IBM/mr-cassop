package controllers

import (
	"context"
	"fmt"
	"github.com/ibm/cassandra-operator/controllers/util"

	"github.com/ibm/cassandra-operator/controllers/reaper"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *CassandraClusterReconciler) reconcileCQLConfigMaps(ctx context.Context, cc *dbv1alpha1.CassandraCluster, cqlClient cql.CqlClient, reaperClient reaper.ReaperClient) error {
	cmList := &v1.ConfigMapList{}
	err := r.List(ctx, cmList, client.HasLabels{cc.Spec.CQLConfigMapLabelKey}, client.InNamespace(cc.Namespace))
	if err != nil {
		return errors.Wrap(err, "Can't get list of config maps")
	}

	if len(cmList.Items) == 0 {
		r.Log.Debug(fmt.Sprintf("No configmaps found with label %q", cc.Spec.CQLConfigMapLabelKey))
		return nil
	}

	for _, cm := range cmList.Items {
		lastChecksum := cm.Annotations["cql-checksum"]
		checksum := util.Sha1(fmt.Sprintf("%v", cm.Data))
		if lastChecksum == checksum {
			continue
		}

		for queryKey, cqlQuery := range cm.Data {
			r.Log.Debugf("Executing query with queryKey %q from ConfigMap %q", queryKey, cm.Name)
			if err = cqlClient.Query(cqlQuery); err != nil {
				return errors.Wrapf(err, "Query with queryKey %q failed", queryKey)
			}
		}

		keyspaceToRepair := cm.Annotations["cql-repairKeyspace"]
		if len(keyspaceToRepair) > 0 {
			r.Log.Infof("Starting a repair for %q keyspace", keyspaceToRepair)
			err := reaperClient.RunRepair(ctx, keyspaceToRepair, repairCauseCQLConfigMap)
			if err != nil {
				return errors.Wrapf(err, "failed to run repair on %q keyspace", keyspaceToRepair)
			}
		} else {
			r.Log.Warnf("Keyspace for ConfigMap %q is not set. Skipping repair.", cm.Name)
		}

		r.Log.Debugf("Updating checksum for ConfigMap %q", cm.Name)
		cm.Annotations["cql-checksum"] = checksum
		if err := r.Update(ctx, &cm); err != nil {
			return errors.Wrapf(err, "Failed to update CM %q", cm.Name)
		}
	}

	return nil
}
