package controllers

import (
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
)

func icarusContainer(cc *dbv1alpha1.CassandraCluster) v1.Container {
	container := v1.Container{
		Name:            "icarus",
		Image:           cc.Spec.Icarus.Image,
		ImagePullPolicy: cc.Spec.Icarus.ImagePullPolicy,
		Args:            []string{"--jmx-credentials=/etc/cassandra-auth-config/icarus-jmx", "--jmx-client-auth=true"},
		VolumeMounts: []v1.VolumeMount{
			cassandraDataVolumeMount(),
			authVolumeMount(),
		},
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: v1.TerminationMessageReadFile,
	}

	icarusPort := v1.ContainerPort{
		Name:          "icarus",
		ContainerPort: dbv1alpha1.IcarusPort,
		Protocol:      v1.ProtocolTCP,
		HostPort:      0,
	}

	if cc.Spec.HostPort.Enabled {
		icarusPort.HostPort = dbv1alpha1.IcarusPort
	}

	if cc.Spec.Encryption.Client.Enabled {
		tlsArgs := []string{
			"--jmx-truststore=/etc/cassandra-client-tls/" + cc.Spec.Encryption.Client.NodeTLSSecret.TruststoreFileKey,
			"--jmx-truststore-password=$(ICARUS_TRUSTSTORE_PASSWORD)",
			"--jmx-keystore=/etc/cassandra-client-tls/" + cc.Spec.Encryption.Client.NodeTLSSecret.KeystoreFileKey,
			"--jmx-keystore-password=$(ICARUS_KEYSTORE_PASSWORD)",
		}

		container.Env = append(container.Env,
			v1.EnvVar{
				Name: "ICARUS_TRUSTSTORE_PASSWORD",
				ValueFrom: &v1.EnvVarSource{
					SecretKeyRef: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{
							Name: cc.Spec.Encryption.Client.NodeTLSSecret.Name,
						},
						Key: cc.Spec.Encryption.Client.NodeTLSSecret.TruststorePasswordKey,
					},
				},
			},
			v1.EnvVar{
				Name: "ICARUS_KEYSTORE_PASSWORD",
				ValueFrom: &v1.EnvVarSource{
					SecretKeyRef: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{
							Name: cc.Spec.Encryption.Client.NodeTLSSecret.Name,
						},
						Key: cc.Spec.Encryption.Client.NodeTLSSecret.KeystorePasswordKey,
					},
				},
			},
		)

		container.Args = append(container.Args, tlsArgs...)
		container.VolumeMounts = append(container.VolumeMounts, cassandraClientTLSVolumeMount())
	}

	container.Ports = append(container.Ports, icarusPort)

	return container
}
