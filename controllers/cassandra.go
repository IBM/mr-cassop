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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
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
	stsLabels["datacenter"] = dc.Name
	desiredSts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.DC(cc, dc.Name),
			Namespace: cc.Namespace,
			Labels:    stsLabels,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName:         names.DCService(cc, dc.Name),
			Replicas:            dc.Replicas,
			Selector:            &metav1.LabelSelector{MatchLabels: labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra)},
			PodManagementPolicy: appsv1.OrderedReadyPodManagement,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type:          appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{Partition: proto.Int32(0)},
			},
			RevisionHistoryLimit: proto.Int32(10),
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra),
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
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
							},
							Args: []string{
								"bash",
								"-c",
								getCassandraRunCommand(cc),
							},
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
											fmt.Sprintf("http --check-status --timeout 2 --body GET %s/healthz/$POD_IP", names.ProberService(cc)),
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
											strings.Join([]string{
												"nodetool disablegossip && sleep 5",
												"nodetool disablethrift && sleep 5",
												"nodetool disablebinary && sleep 5",
												"/scripts/probe.sh drain",
											}, "\n"),
										},
									},
								},
							},
							VolumeMounts: []v1.VolumeMount{
								usersVolumeMount(),
								scriptsVolumeMount(),
								jmxSecretVolumeMount(),
								cassandraDCConfigVolumeMount(),
								{
									Name:      "data",
									MountPath: "/var/lib/cassandra",
								},
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
									ContainerPort: cc.Spec.Cassandra.JMXPort,
									Protocol:      v1.ProtocolTCP,
								},
								{
									Name:          "cql",
									ContainerPort: 9042,
									Protocol:      v1.ProtocolTCP,
								},
								{
									Name:          "thrift",
									ContainerPort: 9160,
									Protocol:      v1.ProtocolTCP,
								},
							},
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: v1.TerminationMessageReadFile,
						},
					},
					ImagePullSecrets: imagePullSecrets(cc),
					Volumes: []v1.Volume{
						usersVolume(cc),
						scriptsVolume(cc),
						jmxSecretVolume(cc),
						cassandraDCConfigVolume(cc, dc),
						{
							Name: "data",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
					RestartPolicy:                 v1.RestartPolicyAlways,
					TerminationGracePeriodSeconds: proto.Int64(30),
					DNSPolicy:                     v1.DNSClusterFirst,
					SecurityContext:               &v1.PodSecurityContext{},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cc, desiredSts, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualSts := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: names.DC(cc, dc.Name), Namespace: cc.Namespace}, actualSts)
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

func (r *CassandraClusterReconciler) reconcileDCService(ctx context.Context, cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC) error {
	desiredService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.DCService(cc, dc.Name),
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra),
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
					Port:       cc.Spec.Cassandra.JMXPort,
					TargetPort: intstr.FromInt(int(cc.Spec.Cassandra.JMXPort)),
					NodePort:   0,
				},
				{
					Name:       "cql",
					Protocol:   v1.ProtocolTCP,
					Port:       9042,
					TargetPort: intstr.FromInt(9042),
					NodePort:   0,
				},
				{
					Name:       "thrift",
					Protocol:   v1.ProtocolTCP,
					Port:       9160,
					TargetPort: intstr.FromInt(9160),
					NodePort:   0,
				},
			},
			ClusterIP:                v1.ClusterIPNone,
			Type:                     v1.ServiceTypeClusterIP,
			SessionAffinity:          v1.ServiceAffinityNone,
			PublishNotReadyAddresses: true,
			Selector:                 labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra),
		},
	}

	actualService := &v1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: names.DCService(cc, dc.Name), Namespace: cc.Namespace}, actualService)
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
			seedsList = append(seedsList, fmt.Sprintf(
				"%s-%d.%s.%s.svc.cluster.local",
				names.DC(cc, dc.Name), i, names.DCService(cc, dc.Name), cc.Namespace),
			)
		}
	}

	return seedsList
}

func getCassandraRunCommand(cc *dbv1alpha1.CassandraCluster) string {
	commands := make([]string, 0)
	commands = append(commands, "cp /etc/cassandra-configmaps/* $CASSANDRA_CONF")
	commands = append(commands, "cp /etc/cassandra-configmaps/jvm.options $CASSANDRA_HOME")
	commands = append(commands, "until stat $CASSANDRA_CONF/cassandra.yaml; do sleep 5; done")
	commands = append(commands, "echo \"broadcast_address: $POD_IP\" >> $CASSANDRA_CONF/cassandra.yaml")
	commands = append(commands, "echo \"broadcast_rpc_address: $POD_IP\" >> $CASSANDRA_CONF/cassandra.yaml")
	commands = append(commands, fmt.Sprintf(
		"exec cassandra -R -f "+
			"-Dcassandra.jmx.remote.port=%d "+
			"-Dcom.sun.management.jmxremote.rmi.port=%d "+
			"-Dcom.sun.management.jmxremote.authenticate=internal"+
			"-Djava.rmi.server.hostname=$POD_IP",
		cc.Spec.Cassandra.JMXPort, cc.Spec.Cassandra.JMXPort))

	return strings.Join(commands, "\n") + "\n"
}

func imagePullSecrets(cc *dbv1alpha1.CassandraCluster) []v1.LocalObjectReference {
	return []v1.LocalObjectReference{
		{
			Name: cc.Spec.ImagePullSecretName,
		},
	}
}
