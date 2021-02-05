package controllers

import (
	"context"
	"fmt"
	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/compare"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
)

const (
	CassandraEndpointLabels = "cassandra-cluster-component=cassandra"
)

func (r *CassandraClusterReconciler) reconcileCassandra(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	for _, dc := range cc.Spec.DCs {
		err := r.reconcileDCService(ctx, cc, dc)
		if err != nil {
			return errors.Wrapf(err, "failed to reconcile dc %q", dc.Name)
		}

		err = r.reconcileDCStatefulSet(ctx, cc, dc)
		if err != nil {
			return errors.Wrapf(err, "failed to reconcile dc %q", dc.Name)
		}
	}

	return nil
}

func (r *CassandraClusterReconciler) reconcileDCStatefulSet(ctx context.Context, cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC) error {
	stsLabels := labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra)
	stsLabels = labels.WithDCLabel(stsLabels, dc.Name)
	desiredSts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.DC(cc.Name, dc.Name),
			Namespace: cc.Namespace,
			Labels:    stsLabels,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName:         names.DCService(cc.Name, dc.Name),
			Replicas:            dc.Replicas,
			Selector:            &metav1.LabelSelector{MatchLabels: stsLabels},
			PodManagementPolicy: cc.Spec.PodManagementPolicy,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type:          appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{Partition: proto.Int32(0)},
			},
			RevisionHistoryLimit: proto.Int32(10),
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: stsLabels,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						cassandraContainer(cc, dc),
					},
					InitContainers: []v1.Container{
						maintenanceContainer(cc, dc),
					},
					ImagePullSecrets: imagePullSecrets(cc),
					Volumes: []v1.Volume{
						rolesVolume(cc),
						scriptsVolume(cc),
						jmxSecretVolume(cc),
						maintenanceVolume(cc),
						cassandraDCConfigVolume(cc, dc),
						podsConfigVolume(cc),
					},
					RestartPolicy:                 v1.RestartPolicyAlways,
					TerminationGracePeriodSeconds: cc.Spec.Cassandra.TerminationGracePeriodSeconds,
					DNSPolicy:                     v1.DNSClusterFirst,
					SecurityContext:               &v1.PodSecurityContext{},
				},
			},
		},
	}

	if cc.Spec.Cassandra.Persistence.Enabled {
		desiredSts.Spec.VolumeClaimTemplates = cassandraVolumeClaimTemplates(cc)
	} else {
		desiredSts.Spec.Template.Spec.Volumes = append(desiredSts.Spec.Template.Spec.Volumes, emptyDirDataVolume())
	}

	if err := controllerutil.SetControllerReference(cc, desiredSts, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualSts := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: names.DC(cc.Name, dc.Name), Namespace: cc.Namespace}, actualSts)
	if err != nil && apierrors.IsNotFound(err) {
		r.Log.Infof("Creating cassandra statefulset for DC %q", dc.Name)
		err = r.Create(ctx, desiredSts)
		if err != nil {
			return errors.Wrap(err, "Failed to create statefulset")
		}
	} else if err != nil {
		return errors.Wrap(err, "Failed to get statefulset")
	} else {
		desiredSts.Annotations = actualSts.Annotations
		if !compare.EqualStatefulSet(desiredSts, actualSts) {
			r.Log.Info("Updating cassandra statefulset")
			r.Log.Debug(compare.DiffStatefulSet(actualSts, desiredSts))
			actualSts.Spec = desiredSts.Spec
			actualSts.Labels = desiredSts.Labels
			if err = r.Update(ctx, actualSts); err != nil {
				return errors.Wrap(err, "failed to update statefulset")
			}
		} else {
			r.Log.Debugf("No updates to cassandra statefulset")
		}
	}

	return nil
}

func cassandraContainer(cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC) v1.Container {
	container := v1.Container{
		Name:            "cassandra",
		Image:           cc.Spec.Cassandra.Image,
		ImagePullPolicy: cc.Spec.Cassandra.ImagePullPolicy,
		Env: []v1.EnvVar{
			{
				Name: "POD_IP",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"},
				},
			},
			{
				Name: "POD_NAME",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.name"},
				},
			},
			{
				Name:  "CASSANDRA_CLUSTER_NAME",
				Value: cc.Name,
			},
			{
				Name:  "CASSANDRA_DC",
				Value: dc.Name,
			},
			{
				// In order for Cassandra to apply cassandra-rackdc.properties, otherwise this variable will have no effect.
				Name:  "CASSANDRA_ENDPOINT_SNITCH",
				Value: "GossipingPropertyFileSnitch",
			},
			{
				Name:  "CASSANDRA_SEEDS",
				Value: strings.Join(getSeedsList(cc), ","),
			},
			{
				Name:  "CASSANDRA_LISTEN_ADDRESS",
				Value: "auto",
			},
			{
				Name: "CASSANDRA_BROADCAST_ADDRESS",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"},
				},
			},
			{
				Name: "CASSANDRA_BROADCAST_RPC_ADDRESS",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"},
				},
			},
		},
		Args: []string{
			"bash",
			"-c",
			getCassandraRunCommand(cc),
		},
		Resources: cc.Spec.Cassandra.Resources,
		LivenessProbe: &v1.Probe{
			Handler: v1.Handler{
				TCPSocket: &v1.TCPSocketAction{
					Port: intstr.FromString("jmx"),
				},
			},
			InitialDelaySeconds: 60,
			TimeoutSeconds:      20,
			PeriodSeconds:       20,
			SuccessThreshold:    1,
			FailureThreshold:    3,
		},
		ReadinessProbe: &v1.Probe{
			Handler: v1.Handler{
				Exec: &v1.ExecAction{
					Command: []string{
						"bash",
						"-c",
						fmt.Sprintf("http --check-status --timeout 2 --body GET %s/healthz/$POD_IP", names.ProberService(cc.Name)),
					},
				},
			},
			TimeoutSeconds:      20,
			PeriodSeconds:       10,
			SuccessThreshold:    2,
			FailureThreshold:    3,
			InitialDelaySeconds: 20,
		},
		Lifecycle: &v1.Lifecycle{
			PreStop: &v1.Handler{
				Exec: &v1.ExecAction{
					Command: []string{
						"bash",
						"-c",
						"/scripts/probe.sh drain",
					},
				},
			},
		},
		VolumeMounts: []v1.VolumeMount{
			rolesVolumeMount(),
			scriptsVolumeMount(),
			jmxSecretVolumeMount(),
			cassandraDCConfigVolumeMount(),
			cassandraDataVolumeMount(),
			podsConfigVolumeMount(),
		},
		Ports: []v1.ContainerPort{
			{
				Name:          "intra",
				ContainerPort: 7000,
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "tls",
				ContainerPort: 7001,
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "jmx",
				ContainerPort: jmxPort,
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "cql",
				ContainerPort: cqlPort,
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "thrift",
				ContainerPort: thriftPort,
				Protocol:      v1.ProtocolTCP,
			},
		},
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: v1.TerminationMessageReadFile,
	}

	if cc.Spec.Cassandra.Persistence.Enabled && cc.Spec.Cassandra.Persistence.CommitLogVolume {
		container.VolumeMounts = append(container.VolumeMounts, commitLogVolumeMount())
	}

	return container
}

func (r *CassandraClusterReconciler) reconcileDCService(ctx context.Context, cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC) error {
	svcLabels := labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra)
	svcLabels = labels.WithDCLabel(svcLabels, dc.Name)
	desiredService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.DCService(cc.Name, dc.Name),
			Labels:    svcLabels,
			Namespace: cc.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       "intra",
					Protocol:   v1.ProtocolTCP,
					Port:       7000,
					TargetPort: intstr.FromInt(7000),
					NodePort:   0,
				},
				{
					Name:       "tls",
					Protocol:   v1.ProtocolTCP,
					Port:       7001,
					TargetPort: intstr.FromInt(7001),
					NodePort:   0,
				},
				{
					Name:       "jmx",
					Protocol:   v1.ProtocolTCP,
					Port:       jmxPort,
					TargetPort: intstr.FromInt(jmxPort),
					NodePort:   0,
				},
				{
					Name:       "cql",
					Protocol:   v1.ProtocolTCP,
					Port:       cqlPort,
					TargetPort: intstr.FromInt(cqlPort),
					NodePort:   0,
				},
				{
					Name:       "thrift",
					Protocol:   v1.ProtocolTCP,
					Port:       thriftPort,
					TargetPort: intstr.FromInt(thriftPort),
					NodePort:   0,
				},
			},
			ClusterIP:                v1.ClusterIPNone,
			Type:                     v1.ServiceTypeClusterIP,
			SessionAffinity:          v1.ServiceAffinityNone,
			PublishNotReadyAddresses: true,
			Selector:                 svcLabels,
		},
	}

	if err := controllerutil.SetControllerReference(cc, desiredService, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualService := &v1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: names.DCService(cc.Name, dc.Name), Namespace: cc.Namespace}, actualService)
	if err != nil && apierrors.IsNotFound(err) {
		r.Log.Infof("Creating service for DC %q", dc.Name)
		err = r.Create(ctx, desiredService)
		if err != nil {
			return errors.Wrapf(err, "Failed to create service for DC %q", dc.Name)
		}
	} else if err != nil {
		return errors.Wrapf(err, "Failed to get service for DC %q", dc.Name)
	} else {
		// ClusterIP is immutable once created, so always enforce the same as existing
		desiredService.Spec.ClusterIP = actualService.Spec.ClusterIP
		if !compare.EqualService(desiredService, actualService) {
			r.Log.Infof("Updating service for DC %q", dc.Name)
			r.Log.Debugf(compare.DiffService(actualService, desiredService))
			actualService.Spec = desiredService.Spec
			actualService.Labels = desiredService.Labels
			actualService.Annotations = desiredService.Annotations
			if err = r.Update(ctx, actualService); err != nil {
				return errors.Wrapf(err, "failed to update service for DC %q", dc.Name)
			}
		} else {
			r.Log.Debugf("No updates to service for DC %q", dc.Name)
		}
	}

	return nil
}

func getSeedsList(cc *dbv1alpha1.CassandraCluster) []string {
	seedsList := make([]string, 0)
	for _, dc := range cc.Spec.DCs {
		seedsCount := *dc.Replicas
		if seedsCount > cc.Spec.Cassandra.NumSeeds {
			seedsCount = cc.Spec.Cassandra.NumSeeds
		}

		for i := int32(0); i < seedsCount; i++ {
			seedsList = append(seedsList, getSeed(cc, dc.Name, i))
		}
	}

	return seedsList
}

func getSeed(cc *dbv1alpha1.CassandraCluster, dcName string, replicaNum int32) string {
	return fmt.Sprintf(
		"%s-%d.%s.%s.svc.cluster.local",
		names.DC(cc.Name, dcName), replicaNum, names.DCService(cc.Name, dcName), cc.Namespace)
}

func getCassandraRunCommand(cc *dbv1alpha1.CassandraCluster) string {
	commands := make([]string, 0)
	if cc.Spec.Cassandra.PurgeGossip {
		commands = append(commands, "[[ -d /var/lib/cassandra/data/system ]] && find /var/lib/cassandra/data/system -mindepth 1 -maxdepth 1 -name 'peers*' -type d -exec rm -rf {} \\;")
	}
	commands = append(commands, "cp /etc/cassandra-configmaps/* $CASSANDRA_CONF")
	commands = append(commands, "cp /etc/cassandra-configmaps/jvm.options $CASSANDRA_HOME")
	commands = append(commands, "until stat /etc/pods-config/${POD_NAME}.env; do \"Waiting for pod configuration...\"; sleep 5; done")
	commands = append(commands, "source /etc/pods-config/${POD_NAME}.env")
	commands = append(commands, fmt.Sprintf("./docker-entrypoint.sh -R -f -Dcassandra.jmx.remote.port=%d -Dcom.sun.management.jmxremote.rmi.port=%d -Dcom.sun.management.jmxremote.authenticate=false -Djava.rmi.server.hostname=$POD_IP", jmxPort, jmxPort))
	return strings.Join(commands, "\n") + "\n"
}

func imagePullSecrets(cc *dbv1alpha1.CassandraCluster) []v1.LocalObjectReference {
	return []v1.LocalObjectReference{
		{
			Name: cc.Spec.ImagePullSecretName,
		},
	}
}

func emptyDirDataVolume() v1.Volume {
	return v1.Volume{
		Name: "data",
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	}
}

func cassandraDataVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      "data",
		MountPath: "/var/lib/cassandra",
	}
}

func commitLogVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      "commitlog",
		MountPath: cassandraCommitLogDir,
	}
}

func cassandraVolumeClaimTemplates(cc *dbv1alpha1.CassandraCluster) []v1.PersistentVolumeClaim {
	volumeClaims := []v1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "data",
				Labels:      cc.Spec.Cassandra.Persistence.Labels,
				Annotations: cc.Spec.Cassandra.Persistence.Annotations,
			},
			Spec: cc.Spec.Cassandra.Persistence.DataVolumeClaimSpec,
		},
	}

	if cc.Spec.Cassandra.Persistence.CommitLogVolume {
		volumeClaims = append(volumeClaims, v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "commitlog",
				Labels:      cc.Spec.Cassandra.Persistence.Labels,
				Annotations: cc.Spec.Cassandra.Persistence.Annotations,
			},
			Spec: cc.Spec.Cassandra.Persistence.CommitLogVolumeClaimSpec,
		})
	}

	return volumeClaims
}

func maintenanceContainer(cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC) v1.Container {
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

func maintenanceVolume(cc *dbv1alpha1.CassandraCluster) v1.Volume {
	return v1.Volume{
		Name: "maintenance-config",
		VolumeSource: v1.VolumeSource{
			ConfigMap: &v1.ConfigMapVolumeSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: names.MaintenanceConfigMap(cc.Name),
				},
				DefaultMode: proto.Int32(0700),
			},
		},
	}
}

func maintenanceVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      "maintenance-config",
		MountPath: maintenanceDir,
	}
}

func podsConfigVolume(cc *dbv1alpha1.CassandraCluster) v1.Volume {
	return v1.Volume{
		Name: "pods-config",
		VolumeSource: v1.VolumeSource{
			ConfigMap: &v1.ConfigMapVolumeSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: names.PodsConfigConfigmap(cc.Name),
				},
				DefaultMode: proto.Int32(0644),
			},
		},
	}
}

func podsConfigVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      "pods-config",
		MountPath: "/etc/pods-config",
	}
}
