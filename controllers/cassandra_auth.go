package controllers

import (
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
)

func createAuthVolumes(cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC) []v1.Volume {
	return []v1.Volume{
		scriptsVolume(cc),
		cassandraDCConfigVolume(cc, dc),
	}
}
