package controllers

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/compare"
	"github.com/ibm/cassandra-operator/controllers/events"
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

type tlsSecretChecksum struct {
	server string
	client string
}

var (
	errTLSSecretNotFound = errors.New("TLS secret not found")
	errTLSSecretInvalid  = errors.New("TLS secret is not valid")
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
	var err error
	tlsSecretChecksums := tlsSecretChecksum{}
	clientTLSSecret := &v1.Secret{}
	serverTLSSecret := &v1.Secret{}

	if cc.Spec.Encryption.Server.InternodeEncryption != internodeEncryptionNone {
		err = r.Get(ctx, types.NamespacedName{Name: cc.Spec.Encryption.Server.TLSSecret.Name, Namespace: cc.Namespace}, serverTLSSecret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				errMsg := fmt.Sprintf("server TLS secret %s is not found", cc.Spec.Encryption.Server.TLSSecret.Name)
				r.Events.Warning(cc, events.EventTLSSecretNotFound, errMsg)
				r.Log.Warn(errMsg)
				return errTLSSecretNotFound
			}
			return errors.Wrapf(err, "failed to get TLS Secret %s", cc.Spec.Encryption.Server.TLSSecret.Name)
		}

		err = r.checkRequiredFields(cc, serverTLSSecret)
		if err != nil {
			return errors.Wrap(err, "failed to validate Server TLS Secret fields")
		}

		annotations := make(map[string]string)
		annotations[dbv1alpha1.CassandraClusterInstance] = cc.Name
		err := r.reconcileAnnotations(ctx, serverTLSSecret, annotations)
		if err != nil {
			return errors.Wrapf(err, "Failed to reconcile Annotations for Secret %s", serverTLSSecret.Name)
		}

		tlsSecretChecksums.server = util.Sha1(fmt.Sprintf("%v", serverTLSSecret.Data))
	}

	if cc.Spec.Encryption.Client.Enabled {
		clientTLSSecret, err = r.getSecret(ctx, cc.Spec.Encryption.Client.TLSSecret.Name, cc.Namespace)
		if err != nil {
			if apierrors.IsNotFound(err) {
				errMsg := fmt.Sprintf("client TLS secret %s is not found", cc.Spec.Encryption.Client.TLSSecret.Name)
				r.Events.Warning(cc, events.EventTLSSecretNotFound, errMsg)
				r.Log.Warn(errMsg)
				return errTLSSecretNotFound
			}
			return errors.Wrapf(err, "failed to get TLS Secret %s", cc.Spec.Encryption.Client.TLSSecret.Name)
		}

		err = r.checkRequiredFields(cc, clientTLSSecret)
		if err != nil {
			return errors.Wrap(err, "failed to validate Client TLS Secret fields")
		}

		annotations := make(map[string]string)
		annotations[dbv1alpha1.CassandraClusterInstance] = cc.Name
		err := r.reconcileAnnotations(ctx, clientTLSSecret, annotations)
		if err != nil {
			return errors.Wrapf(err, "Failed to reconcile Annotations for Secret %s", clientTLSSecret.Name)
		}

		tlsSecretChecksums.client = util.Sha1(fmt.Sprintf("%v", clientTLSSecret.Data))
	}

	desiredSts := cassandraStatefulSet(cc, dc, tlsSecretChecksums, clientTLSSecret)

	if err := controllerutil.SetControllerReference(cc, desiredSts, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualSts := &appsv1.StatefulSet{}
	err = r.Get(ctx, types.NamespacedName{Name: names.DC(cc.Name, dc.Name), Namespace: cc.Namespace}, actualSts)
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

func cassandraStatefulSet(cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC, tlsSecretChecksums tlsSecretChecksum, clientTLSSecret *v1.Secret) *appsv1.StatefulSet {
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
						cassandraContainer(cc, dc, tlsSecretChecksums, clientTLSSecret),
					},
					InitContainers: []v1.Container{
						privilegedInitContainer(cc),
						maintenanceContainer(cc),
						initContainer(cc),
					},
					ImagePullSecrets: imagePullSecrets(cc),
					Volumes: []v1.Volume{
						scriptsVolume(cc),
						prometheusVolume(cc),
						maintenanceVolume(cc),
						cassandraConfigVolume(cc),
						podsConfigVolume(cc),
						authVolume(cc),
					},
					Affinity:                      dc.Affinity,
					Tolerations:                   dc.Tolerations,
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

	if cc.Spec.Encryption.Server.InternodeEncryption != internodeEncryptionNone {
		desiredSts.Spec.Template.Spec.Volumes = append(desiredSts.Spec.Template.Spec.Volumes, cassandraServerTLSVolume(cc))
	}

	if cc.Spec.Encryption.Client.Enabled {
		desiredSts.Spec.Template.Spec.Volumes = append(desiredSts.Spec.Template.Spec.Volumes, cassandraClientTLSVolume(cc))
	}

	return desiredSts
}

func cassandraContainer(cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC, tlsSecretChecksums tlsSecretChecksum, clientTLSSecret *v1.Secret) v1.Container {
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
			getCassandraRunCommand(cc, clientTLSSecret),
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
			prometheusVolumeMount(),
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

	if cc.Spec.Encryption.Server.InternodeEncryption != internodeEncryptionNone {
		container.VolumeMounts = append(container.VolumeMounts, cassandraServerTLSVolumeMount())
		container.Env = append(container.Env, v1.EnvVar{
			Name:  "SERVER_TLS_SHA1",
			Value: tlsSecretChecksums.server,
		})
	}

	if cc.Spec.Encryption.Client.Enabled {
		container.VolumeMounts = append(container.VolumeMounts, cassandraClientTLSVolumeMount())
		container.Env = append(container.Env, v1.EnvVar{
			Name:  "CLIENT_TLS_SHA1",
			Value: tlsSecretChecksums.client,
		})
		container.Env = append(container.Env, v1.EnvVar{
			Name:  "TLS_ARG",
			Value: "--ssl",
		})
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
					Port:       dbv1alpha1.IntraPort,
					TargetPort: intstr.FromInt(dbv1alpha1.IntraPort),
					NodePort:   0,
				},
				{
					Name:       "tls",
					Protocol:   v1.ProtocolTCP,
					Port:       dbv1alpha1.TlsPort,
					TargetPort: intstr.FromInt(dbv1alpha1.TlsPort),
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

func getCassandraRunCommand(cc *dbv1alpha1.CassandraCluster, clientTLSSecret *v1.Secret) string {
	var args []string
	if cc.Spec.Cassandra.PurgeGossip {
		args = append(args, "rm -rf /var/lib/cassandra/data/system/peers*")
	}

	args = append(args,
		"cp /etc/cassandra-configmaps/* $CASSANDRA_CONF/",
		"cp /etc/cassandra-configmaps/jvm.options $CASSANDRA_HOME/",
		"source /etc/pods-config/${POD_NAME}_${POD_UID}.sh",
		`replace_address=""
old_ip=$CASSANDRA_NODE_PREVIOUS_IP
if ([[ "${old_ip}" != "" ]]) && ([ ! -d "/var/lib/cassandra/data" ] || [ -z "$(ls -A /var/lib/cassandra/data)" ]); then
  replace_address="-Dcassandra.replace_address_first_boot=${old_ip}"
fi`,
	)

	if cc.Spec.Encryption.Client.Enabled {
		args = append(args,
			"mkdir -p /home/cassandra/.cassandra/",
			"cp /etc/cassandra-auth-config/nodetool-ssl.properties /home/cassandra/.cassandra/")
	}

	cassandraRunCommand := []string{
		"/docker-entrypoint.sh -f -R",
		fmt.Sprintf("-Dcassandra.jmx.remote.port=%d", dbv1alpha1.JmxPort),
		fmt.Sprintf("-Dcom.sun.management.jmxremote.rmi.port=%d", dbv1alpha1.JmxPort),
		"-Djava.rmi.server.hostname=$CASSANDRA_BROADCAST_ADDRESS",
		"-Dcom.sun.management.jmxremote.authenticate=true",
		"-Dcassandra.storagedir=/var/lib/cassandra",
		"${replace_address}",
	}

	if cc.Spec.JMX.Authentication == jmxAuthenticationLocalFiles {
		cassandraRunCommand = append(cassandraRunCommand,
			"-Dcom.sun.management.jmxremote.password.file=/etc/cassandra-auth-config/jmxremote.password",
			"-Dcom.sun.management.jmxremote.access.file=/etc/cassandra-auth-config/jmxremote.access",
		)
	}

	if cc.Spec.JMX.Authentication == jmxAuthenticationInternal {
		cassandraRunCommand = append(cassandraRunCommand,
			"-Dcassandra.jmx.remote.login.config=CassandraLogin",
			"-Djava.security.auth.login.config=$CASSANDRA_HOME/conf/cassandra-jaas.config",
			"-Dcassandra.jmx.authorizer=org.apache.cassandra.auth.jmx.AuthorizationProxy",
		)
	}

	if cc.Spec.Monitoring.Enabled {
		cassandraRunCommand = append(cassandraRunCommand, getJavaAgent(cc.Spec.Monitoring.Agent))
	}

	if cc.Spec.Encryption.Client.Enabled {
		cassandraRunCommand = append(cassandraRunCommand,
			"-Dcom.sun.management.jmxremote.ssl=true",
			fmt.Sprintf("-Dcom.sun.management.jmxremote.ssl.need.client.auth=%v", *cc.Spec.Encryption.Client.RequireClientAuth),
			"-Dcom.sun.management.jmxremote.registry.ssl=true",
			tlsJVMArgs(cc, clientTLSSecret),
		)
	}

	args = append(args, strings.Join(cassandraRunCommand, " "))

	return strings.Join(args, "\n")
}

func getJavaAgent(agent string) string {
	javaAgent := ""
	switch agent {
	case "instaclustr":
		javaAgent = "-javaagent:/prometheus/cassandra-exporter-agent.jar"
	case "datastax":
		javaAgent = "-javaagent:/prometheus/datastax-mcac-agent/lib/datastax-mcac-agent.jar"
	case "tlp":
		javaAgent = "-javaagent:/prometheus/jmx_prometheus_javaagent.jar=8090:/prometheus/prometheus.yaml"
	}
	return javaAgent
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
	pvcLabels := labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra)
	if cc.Spec.Cassandra.Persistence.Labels != nil {
		pvcLabels = util.MergeMap(cc.Spec.Cassandra.Persistence.Labels, pvcLabels)
	}
	volumeClaims := []v1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "data",
				Labels:      pvcLabels,
				Annotations: cc.Spec.Cassandra.Persistence.Annotations,
			},
			Spec: cc.Spec.Cassandra.Persistence.DataVolumeClaimSpec,
		},
	}

	if cc.Spec.Cassandra.Persistence.CommitLogVolume {
		volumeClaims = append(volumeClaims, v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "commitlog",
				Labels:      pvcLabels,
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
				DefaultMode: proto.Int32(v1.ConfigMapVolumeSourceDefaultMode),
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
			ContainerPort: dbv1alpha1.IntraPort,
			Protocol:      v1.ProtocolTCP,
			HostPort:      0,
		},
		{
			Name:          "tls",
			ContainerPort: dbv1alpha1.TlsPort,
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

		if cc.Spec.Encryption.Server.InternodeEncryption != internodeEncryptionNone {
			portsToExpose = append(portsToExpose, "tls")
		}

		for i, port := range containerPorts {
			if util.Contains(portsToExpose, containerPorts[i].Name) {
				containerPorts[i].HostPort = port.ContainerPort
			}
		}
	}

	return containerPorts
}

func authVolume(cc *dbv1alpha1.CassandraCluster) v1.Volume {
	items := []v1.KeyToPath{
		{
			Key:  "cqlshrc",
			Path: "cqlshrc",
		},
		{
			Key:  dbv1alpha1.CassandraOperatorAdminRole,
			Path: dbv1alpha1.CassandraOperatorAdminRole,
		},
		{
			Key:  dbv1alpha1.CassandraOperatorAdminPassword,
			Path: dbv1alpha1.CassandraOperatorAdminPassword,
		},
	}

	if cc.Spec.Encryption.Client.Enabled {
		items = append(items, v1.KeyToPath{
			Key:  "nodetool-ssl.properties",
			Path: "nodetool-ssl.properties",
		})
	}

	volume := v1.Volume{
		Name: "auth-config",
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: names.AdminAuthConfigSecret(cc.Name),
				Items:      items,

				DefaultMode: proto.Int32(v1.SecretVolumeSourceDefaultMode),
			},
		},
	}

	if cc.Spec.JMX.Authentication == jmxAuthenticationLocalFiles {
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

func cassandraServerTLSVolume(cc *dbv1alpha1.CassandraCluster) v1.Volume {
	return v1.Volume{
		Name: cassandraServerTLSVolumeName,
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: cc.Spec.Encryption.Server.TLSSecret.Name,
				Items: []v1.KeyToPath{
					{
						Key:  cc.Spec.Encryption.Server.TLSSecret.KeystoreFileKey,
						Path: cc.Spec.Encryption.Server.TLSSecret.KeystoreFileKey,
					},
					{
						Key:  cc.Spec.Encryption.Server.TLSSecret.TruststoreFileKey,
						Path: cc.Spec.Encryption.Server.TLSSecret.TruststoreFileKey,
					},
				},
				DefaultMode: proto.Int32(v1.SecretVolumeSourceDefaultMode),
			},
		},
	}
}

func cassandraServerTLSVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      cassandraServerTLSVolumeName,
		MountPath: cassandraServerTLSDir,
	}
}

func cassandraClientTLSVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      cassandraClientTLSVolumeName,
		MountPath: cassandraClientTLSDir,
	}
}

func cassandraClientTLSVolume(cc *dbv1alpha1.CassandraCluster) v1.Volume {
	return v1.Volume{
		Name: cassandraClientTLSVolumeName,
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: cc.Spec.Encryption.Client.TLSSecret.Name,
				Items: []v1.KeyToPath{
					{
						Key:  cc.Spec.Encryption.Client.TLSSecret.CAFileKey,
						Path: cc.Spec.Encryption.Client.TLSSecret.CAFileKey,
					},
					{
						Key:  cc.Spec.Encryption.Client.TLSSecret.TLSCrtFileKey,
						Path: cc.Spec.Encryption.Client.TLSSecret.TLSCrtFileKey,
					},
					{
						Key:  cc.Spec.Encryption.Client.TLSSecret.TLSFileKey,
						Path: cc.Spec.Encryption.Client.TLSSecret.TLSFileKey,
					},
					{
						Key:  cc.Spec.Encryption.Client.TLSSecret.KeystoreFileKey,
						Path: cc.Spec.Encryption.Client.TLSSecret.KeystoreFileKey,
					},
					{
						Key:  cc.Spec.Encryption.Client.TLSSecret.TruststoreFileKey,
						Path: cc.Spec.Encryption.Client.TLSSecret.TruststoreFileKey,
					},
				},
				DefaultMode: proto.Int32(v1.SecretVolumeSourceDefaultMode),
			},
		},
	}
}

func (r *CassandraClusterReconciler) checkRequiredFields(cc *dbv1alpha1.CassandraCluster, tlsSecret *v1.Secret) error {
	if cc.Spec.Encryption.Server.InternodeEncryption != internodeEncryptionNone {
		requiredFields := []string{
			cc.Spec.Encryption.Server.TLSSecret.KeystoreFileKey,
			cc.Spec.Encryption.Server.TLSSecret.KeystorePasswordKey,
			cc.Spec.Encryption.Server.TLSSecret.TruststoreFileKey,
			cc.Spec.Encryption.Server.TLSSecret.TruststorePasswordKey,
		}
		emptyFields := util.EmptySecretFields(tlsSecret, requiredFields)
		if tlsSecret.Data == nil || len(emptyFields) != 0 {
			errMsg := fmt.Sprintf("TLS Server Secret %s has some empty or missing fields: %v", tlsSecret.Name, emptyFields)
			r.Log.Warnf(errMsg)
			r.Events.Warning(cc, events.EventTLSSecretInvalid, errMsg)
			return errTLSSecretInvalid
		}
	}

	if cc.Spec.Encryption.Client.Enabled {
		requiredFields := []string{
			cc.Spec.Encryption.Client.TLSSecret.KeystoreFileKey,
			cc.Spec.Encryption.Client.TLSSecret.KeystorePasswordKey,
			cc.Spec.Encryption.Client.TLSSecret.TruststoreFileKey,
			cc.Spec.Encryption.Client.TLSSecret.TruststorePasswordKey,
			cc.Spec.Encryption.Client.TLSSecret.CAFileKey,
			cc.Spec.Encryption.Client.TLSSecret.TLSCrtFileKey,
			cc.Spec.Encryption.Client.TLSSecret.TLSFileKey,
		}

		emptyFields := util.EmptySecretFields(tlsSecret, requiredFields)
		if tlsSecret.Data == nil || len(emptyFields) != 0 {
			errMsg := fmt.Sprintf("TLS Secret %s has some empty or missing fields: %v", tlsSecret.Name, emptyFields)
			r.Log.Warnf(errMsg)
			r.Events.Warning(cc, events.EventTLSSecretInvalid, errMsg)
			return errTLSSecretInvalid
		}
	}

	return nil
}

func tlsJVMArgs(cc *dbv1alpha1.CassandraCluster, clientTLSSecret *v1.Secret) string {
	return fmt.Sprintf("-Djavax.net.ssl.keyStore=%s/%s -Djavax.net.ssl.keyStorePassword=%s -Djavax.net.ssl.trustStore=%s/%s -Djavax.net.ssl.trustStorePassword=%s",
		cassandraClientTLSDir, cc.Spec.Encryption.Client.TLSSecret.KeystoreFileKey, strings.TrimRight(string(clientTLSSecret.Data[cc.Spec.Encryption.Client.TLSSecret.KeystorePasswordKey]), "\r\n"),
		cassandraClientTLSDir, cc.Spec.Encryption.Client.TLSSecret.TruststoreFileKey, strings.TrimRight(string(clientTLSSecret.Data[cc.Spec.Encryption.Client.TLSSecret.TruststorePasswordKey]), "\r\n"))

}
