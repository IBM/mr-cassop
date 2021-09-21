package controllers

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/compare"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	desiredSts := cassandraStatefulSet(cc, dc)

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

func cassandraStatefulSet(cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC) *appsv1.StatefulSet {
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
						maintenanceContainer(cc),
						waitContainer(cc),
					},
					ImagePullSecrets: imagePullSecrets(cc),
					Volumes: []v1.Volume{
						scriptsVolume(cc),
						maintenanceVolume(cc),
						cassandraDCConfigVolume(cc, dc),
						podsConfigVolume(cc),
						authVolume(cc),
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
	return desiredSts
}

func cassandraContainer(cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC) v1.Container {
	container := v1.Container{
		Name:            "cassandra",
		Image:           cc.Spec.Cassandra.Image,
		ImagePullPolicy: cc.Spec.Cassandra.ImagePullPolicy,
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
			{
				Name: "POD_IP",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"},
				},
			},
			{
				Name:  "LOG_LEVEL",
				Value: cc.Spec.Cassandra.LogLevel,
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
				Name:  "CASSANDRA_LISTEN_ADDRESS",
				Value: "$(POD_IP)",
			},
			// If not defined the MAX_HEAP_SIZE value will be generated in Cassandra image by cassandra-env.sh script on startup
			{
				Name:  "MAX_HEAP_SIZE",
				Value: cc.Spec.JVM.MaxHeapSize,
			},
			// If not defined the HEAP_NEWSIZE value will be generated in Cassandra image by cassandra-env.sh script on startup
			{
				Name:  "HEAP_NEWSIZE",
				Value: cc.Spec.JVM.HeapNewSize,
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
					Port: intstr.FromString("cql"),
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
						"source /etc/pods-config/${POD_NAME}_${POD_UID}.sh && " +
							fmt.Sprintf("http --check-status --timeout 2 --body GET %s/healthz/$CASSANDRA_BROADCAST_ADDRESS", names.ProberService(cc.Name)),
					},
				},
			},
			TimeoutSeconds:      20,
			PeriodSeconds:       10,
			SuccessThreshold:    2,
			FailureThreshold:    3,
			InitialDelaySeconds: 40,
		},
		Lifecycle: &v1.Lifecycle{
			PreStop: &v1.Handler{
				Exec: &v1.ExecAction{
					Command: []string{
						"bash",
						"-c",
						"source /home/cassandra/.bashrc && nodetool drain",
					},
				},
			},
		},
		VolumeMounts: []v1.VolumeMount{
			scriptsVolumeMount(),
			cassandraDCConfigVolumeMount(),
			cassandraDataVolumeMount(),
			podsConfigVolumeMount(),
			authVolumeMount(),
		},
		Ports:                    cassandraContainerPorts(cc),
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
					Port:       dbv1alpha1.JmxPort,
					TargetPort: intstr.FromInt(dbv1alpha1.JmxPort),
					NodePort:   0,
				},
				{
					Name:       "cql",
					Protocol:   v1.ProtocolTCP,
					Port:       dbv1alpha1.CqlPort,
					TargetPort: intstr.FromInt(dbv1alpha1.CqlPort),
					NodePort:   0,
				},
				{
					Name:       "thrift",
					Protocol:   v1.ProtocolTCP,
					Port:       dbv1alpha1.ThriftPort,
					TargetPort: intstr.FromInt(dbv1alpha1.ThriftPort),
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
		desiredService.Spec.ClusterIPs = actualService.Spec.ClusterIPs
		desiredService.Spec.IPFamilies = actualService.Spec.IPFamilies
		desiredService.Spec.IPFamilyPolicy = actualService.Spec.IPFamilyPolicy
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

func getLocalSeedsHostnames(cc *dbv1alpha1.CassandraCluster, broadcastAddresses map[string]string) []string {
	seedsList := make([]string, 0)
	for _, dc := range cc.Spec.DCs {
		numSeeds := dcNumberOfSeeds(cc, dc)
		for i := int32(0); i < numSeeds; i++ {
			seed := getSeedHostname(cc, dc.Name, i, !cc.Spec.HostPort.Enabled)
			if cc.Spec.HostPort.Enabled {
				seed = broadcastAddresses[seed]
			}
			seedsList = append(seedsList, seed)
		}
	}

	return seedsList
}

func isSeedPod(pod v1.Pod) bool {
	if pod.Labels == nil {
		return false
	}

	_, labelExists := pod.Labels[dbv1alpha1.CassandraClusterSeed]
	return labelExists
}

func dcNumberOfSeeds(cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC) int32 {
	numSeeds := cc.Spec.Cassandra.NumSeeds
	if numSeeds >= *dc.Replicas { // don't configure all dc's nodes as seeds
		numSeeds = *dc.Replicas - 1 // minimum of 1 non-seed node
		if *dc.Replicas <= 1 {      // unless dc.Replicas only has 1 or 0 nodes
			numSeeds = *dc.Replicas
		}
	}

	return numSeeds
}

func getSeedHostname(cc *dbv1alpha1.CassandraCluster, dcName string, podSuffix int32, isFQDN bool) string {
	svc := names.DC(cc.Name, dcName)
	if isFQDN {
		return fmt.Sprintf("%s-%d.%s.%s.svc.cluster.local", svc, podSuffix, svc, cc.Namespace)
	}
	return fmt.Sprintf("%s-%d", svc, podSuffix)
}

func getCassandraRunCommand(cc *dbv1alpha1.CassandraCluster) string {
	var args []string
	if cc.Spec.Cassandra.PurgeGossip {
		args = append(args, "rm -rf /var/lib/cassandra/data/system/peers*")
	}

	args = append(args,
		"cp /etc/cassandra-configmaps/* $CASSANDRA_CONF/",
		"cp /etc/cassandra-configmaps/jvm.options $CASSANDRA_HOME/",
		"source /etc/pods-config/${POD_NAME}_${POD_UID}.sh",
	)

	cassandraRunCommand := []string{
		"/docker-entrypoint.sh -f -R",
		fmt.Sprintf("-Dcassandra.jmx.remote.port=%d", dbv1alpha1.JmxPort),
		fmt.Sprintf("-Dcom.sun.management.jmxremote.rmi.port=%d", dbv1alpha1.JmxPort),
		"-Djava.rmi.server.hostname=$CASSANDRA_BROADCAST_ADDRESS",
		"-Dcom.sun.management.jmxremote.authenticate=true",
	}

	if cc.Spec.JMX.Authentication == "local_files" {
		cassandraRunCommand = append(cassandraRunCommand,
			"-Dcom.sun.management.jmxremote.password.file=/etc/cassandra-auth-config/jmxremote.password",
			"-Dcom.sun.management.jmxremote.access.file=/etc/cassandra-auth-config/jmxremote.access",
		)
	} else { // Internal auth is default
		cassandraRunCommand = append(cassandraRunCommand,
			"-Dcassandra.jmx.remote.login.config=CassandraLogin",
			"-Djava.security.auth.login.config=$CASSANDRA_HOME/conf/cassandra-jaas.config",
			"-Dcassandra.jmx.authorizer=org.apache.cassandra.auth.jmx.AuthorizationProxy",
		)
	}

	args = append(args, strings.Join(cassandraRunCommand, " "))

	return strings.Join(args, "\n")
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

func waitContainer(cc *dbv1alpha1.CassandraCluster) v1.Container {
	memory := resource.MustParse("200Mi")
	cpu := resource.MustParse("0.5")

	args := []string{
		`config_path=/etc/pods-config/${POD_NAME}_${POD_UID}.sh
COUNT=1
until stat $config_path; do
  echo Could not access mount $config_path. Attempt $(( COUNT++ ))...
  sleep 10
done
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
		Name:            "pre-flight-checks",
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
				DefaultMode: proto.Int32(v1.SecretVolumeSourceDefaultMode),
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

func cassandraContainerPorts(cc *dbv1alpha1.CassandraCluster) []v1.ContainerPort {
	containerPorts := []v1.ContainerPort{
		{
			Name:          "intra",
			ContainerPort: 7000,
			Protocol:      v1.ProtocolTCP,
			HostPort:      0,
		},
		{
			Name:          "tls",
			ContainerPort: 7001,
			Protocol:      v1.ProtocolTCP,
			HostPort:      0,
		},
		{
			Name:          "jmx",
			ContainerPort: dbv1alpha1.JmxPort,
			Protocol:      v1.ProtocolTCP,
			HostPort:      0,
		},
		{
			Name:          "cql",
			ContainerPort: dbv1alpha1.CqlPort,
			Protocol:      v1.ProtocolTCP,
			HostPort:      0,
		},
		{
			Name:          "thrift",
			ContainerPort: dbv1alpha1.ThriftPort,
			Protocol:      v1.ProtocolTCP,
			HostPort:      0,
		},
	}

	if cc.Spec.HostPort.Enabled {
		portsToExpose := cc.Spec.HostPort.Ports
		// jmx port is required to make jolokia work
		// intra needed to allow cassandra nodes to talk to each other
		portsToExpose = append(portsToExpose, "jmx", "intra")
		for i, port := range containerPorts {
			if util.Contains(portsToExpose, containerPorts[i].Name) {
				containerPorts[i].HostPort = port.ContainerPort
			}
		}
	}

	return containerPorts
}

func authVolume(cc *dbv1alpha1.CassandraCluster) v1.Volume {
	volume := v1.Volume{
		Name: "auth-config",
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: names.AdminAuthConfigSecret(cc.Name),
				Items: []v1.KeyToPath{
					{
						Key:  "cqlshrc",
						Path: "cqlshrc",
					},
					{
						Key:  "admin_username",
						Path: "admin_username",
					},
					{
						Key:  "admin_password",
						Path: "admin_password",
					},
				},

				DefaultMode: proto.Int32(v1.SecretVolumeSourceDefaultMode),
			},
		},
	}

	if cc.Spec.JMX.Authentication == "local_files" {
		volume.VolumeSource.Secret.Items = append(volume.VolumeSource.Secret.Items, v1.KeyToPath{
			Key:  "jmxremote.password",
			Path: "jmxremote.password",
		})
		volume.VolumeSource.Secret.Items = append(volume.VolumeSource.Secret.Items, v1.KeyToPath{
			Key:  "jmxremote.access",
			Path: "jmxremote.access",
		})
	}

	return volume
}

func authVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      "auth-config",
		MountPath: "/etc/cassandra-auth-config/",
	}
}
