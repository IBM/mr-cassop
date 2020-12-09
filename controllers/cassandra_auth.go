package controllers

import (
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"strconv"
)

func createAuthVolumes(cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC) []v1.Volume {
	return []v1.Volume{
		usersVolume(cc),
		scriptsVolume(cc),
		cassandraDCConfigVolume(cc, dc),
		jmxSecretVolume(cc),
	}
}

func sharedVolumeMounts() []v1.VolumeMount {
	return []v1.VolumeMount{
		usersVolumeMount(),
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
			{Name: "CASSANDRA_JMX_AUTH", Value: strconv.FormatBool(cc.Spec.Jmx.Authentication == "local_files" || cc.Spec.Jmx.Authentication == "internal")},
			{Name: "CASSANDRA_JMX_SSL", Value: strconv.FormatBool(cc.Spec.Jmx.SSL)},
			{Name: "CASSANDRA_INTERNAL_AUTH", Value: strconv.FormatBool(cc.Spec.Config.InternalAuth || cc.Spec.Jmx.Authentication == "internal")},
			{Name: "USERS_DIR", Value: cc.Spec.Cassandra.UsersDir},
		},
		VolumeMounts:             sharedVolumeMounts(),
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: v1.TerminationMessageReadFile,
		Command: []string{
			"/sbin/tini",
			"--",
			"bash",
			"-c",
		},
	}
	return cassandraAuth
}
