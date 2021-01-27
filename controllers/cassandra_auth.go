package controllers

import (
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
)

func createAuthVolumes(cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC) []v1.Volume {
	return []v1.Volume{
		rolesVolume(cc),
		scriptsVolume(cc),
		cassandraDCConfigVolume(cc, dc),
		jmxSecretVolume(cc),
	}
}

func sharedVolumeMounts() []v1.VolumeMount {
	return []v1.VolumeMount{
		rolesVolumeMount(),
		scriptsVolumeMount(),
		jmxSecretVolumeMount(),
		cassandraDCConfigVolumeMount(),
	}
}

func createCassandraAuth(cc *dbv1alpha1.CassandraCluster) v1.Container {
	cassandraAuth := v1.Container{
		Name:            "cassandra-bash",
		Image:           cc.Spec.Cassandra.Image,
		ImagePullPolicy: cc.Spec.Cassandra.ImagePullPolicy,
		Env: []v1.EnvVar{
			// Used in probe.sh and readinessrc.sh scripts
			{Name: "CASSANDRA_JMX_AUTH", Value: "true" /*strconv.FormatBool(cc.Spec.JMX.Authentication == "local_files" || cc.Spec.JMX.Authentication == "internal")*/},
			{Name: "CASSANDRA_JMX_SSL", Value: "false" /*strconv.FormatBool(cc.Spec.JMX.SSL)*/},
			{Name: "CASSANDRA_INTERNAL_AUTH", Value: "true" /*strconv.FormatBool(cc.Spec.Cassandra.InternalAuth || cc.Spec.JMX.Authentication == "internal")*/},
			{Name: "USERS_DIR", Value: cassandraRolesDir},
		},
		VolumeMounts:             sharedVolumeMounts(),
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: v1.TerminationMessageReadFile,
		Command: []string{
			"bash",
			"-c",
		},
	}
	return cassandraAuth
}
