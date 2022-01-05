package controllers

import (
	"fmt"

	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func initContainer(cc *dbv1alpha1.CassandraCluster) v1.Container {
	memory := resource.MustParse("200Mi")
	cpu := resource.MustParse("0.5")

	args := []string{
		`config_path=/etc/pods-config/${POD_NAME}_${POD_UID}.sh
COUNT=1
until stat $config_path; do
  echo Could not access mount $config_path. Attempt $(( COUNT++ ))...
  sleep 10
done

source $config_path
echo PAUSE_INIT=$PAUSE_INIT

until [[ "$PAUSE_INIT" == "false" ]]; do
  echo PAUSE_INIT=$PAUSE_INIT
  echo -n "."
  sleep 10
  source $config_path
done
`,
	}

	return v1.Container{
		Name:            "init",
		Image:           cc.Spec.Cassandra.Image,
		ImagePullPolicy: cc.Spec.Cassandra.ImagePullPolicy,
		Resources: v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceMemory: memory,
				v1.ResourceCPU:    cpu,
			},
			Limits: map[v1.ResourceName]resource.Quantity{
				v1.ResourceMemory: memory,
				v1.ResourceCPU:    cpu,
			},
		},
		VolumeMounts: []v1.VolumeMount{
			podsConfigVolumeMount(),
		},
		Env: []v1.EnvVar{
			{
				Name: "POD_NAME",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.name"},
				},
			},
			{
				Name: "POD_UID",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.uid"},
				},
			},
		},
		Command: []string{
			"bash",
			"-c",
		},
		Args:                     args,
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: v1.TerminationMessageReadFile,
	}
}

func privilegedInitContainer(cc *dbv1alpha1.CassandraCluster) v1.Container {
	memory := resource.MustParse("200Mi")
	cpu := resource.MustParse("0.5")

	args := []string{
		"chown cassandra:cassandra /var/lib/cassandra",
	}

	return v1.Container{
		Name:            "privileged-init",
		Image:           cc.Spec.Cassandra.Image,
		ImagePullPolicy: cc.Spec.Cassandra.ImagePullPolicy,
		SecurityContext: &v1.SecurityContext{
			Privileged: proto.Bool(true),
			RunAsUser:  proto.Int64(0),
			RunAsGroup: proto.Int64(0),
		},
		Resources: v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceMemory: memory,
				v1.ResourceCPU:    cpu,
			},
			Limits: map[v1.ResourceName]resource.Quantity{
				v1.ResourceMemory: memory,
				v1.ResourceCPU:    cpu,
			},
		},
		VolumeMounts: []v1.VolumeMount{
			cassandraDataVolumeMount(),
		},
		Command: []string{
			"bash",
			"-c",
		},
		Args:                     args,
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: v1.TerminationMessageReadFile,
	}
}

func maintenanceContainer(cc *dbv1alpha1.CassandraCluster) v1.Container {
	memory := resource.MustParse("200Mi")
	cpu := resource.MustParse("0.5")
	return v1.Container{
		Name:            "maintenance-mode",
		Image:           cc.Spec.Cassandra.Image,
		ImagePullPolicy: cc.Spec.Cassandra.ImagePullPolicy,
		Resources: v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceMemory: memory,
				v1.ResourceCPU:    cpu,
			},
			Limits: map[v1.ResourceName]resource.Quantity{
				v1.ResourceMemory: memory,
				v1.ResourceCPU:    cpu,
			},
		},
		VolumeMounts: []v1.VolumeMount{
			cassandraDataVolumeMount(),
			maintenanceVolumeMount(),
		},
		Command: []string{
			"bash",
			"-c",
		},
		Args: []string{
			fmt.Sprintf("while [[ -f %s/${HOSTNAME} ]]; do sleep 10; done", maintenanceDir),
		},
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: v1.TerminationMessageReadFile,
	}
}

func maintenanceVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      "maintenance-config",
		MountPath: maintenanceDir,
	}
}
