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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CassandraClusterReconciler) reconcileDCStatefulSet(ctx context.Context, cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC, restartChecksum checksumContainer) error {
	var err error

	if cc.Spec.Encryption.Server.InternodeEncryption != dbv1alpha1.InternodeEncryptionNone {
		if _, err := r.reconcileNodeTLSSecret(ctx, cc, restartChecksum, serverNode); err != nil {
			return err
		}
	}

	clientTLSSecret := &v1.Secret{}
	if cc.Spec.Encryption.Client.Enabled {
		clientTLSSecret, err = r.reconcileNodeTLSSecret(ctx, cc, restartChecksum, clientNode)
		if err != nil {
			return err
		}
	}

	if cc.Spec.HostPort.Enabled && util.Contains(cc.Spec.HostPort.Ports, "cql") && !cc.Spec.Encryption.Client.Enabled {
		warnMsg := fmt.Sprintf("Exposing cql port (%d) as hostPort without client encryption enabled is not secure", dbv1alpha1.CqlPort)
		r.Events.Warning(cc, events.EventInsecureSetup, warnMsg)
		r.Log.Warn(warnMsg)
	}
	desiredSts := cassandraStatefulSet(cc, dc, restartChecksum, clientTLSSecret)

	if err = controllerutil.SetControllerReference(cc, desiredSts, r.Scheme); err != nil {
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
		// the pod selector is immutable once set, so always enforce the same as existing
		desiredSts.Spec.Selector = actualSts.Spec.Selector
		desiredSts.Spec.Template.Labels = actualSts.Spec.Template.Labels
		// annotation can be used by things like `kubectl rollout sts restart` so don't overwrite it
		desiredSts.Spec.Template.Annotations = util.MergeMap(actualSts.Spec.Template.Annotations, desiredSts.Spec.Template.Annotations)
		// scaling is handled by the scaling logic
		desiredSts.Spec.Replicas = actualSts.Spec.Replicas
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

func cassandraStatefulSet(cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC, restartChecksum checksumContainer, clientTLSSecret *v1.Secret) *appsv1.StatefulSet {
	stsLabels := labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra)
	stsLabels = labels.WithDCLabel(stsLabels, dc.Name)
	if cc.Spec.Cassandra.Monitoring.Agent == dbv1alpha1.CassandraAgentTlp {
		stsLabels["environment"] = cc.Namespace
		stsLabels["datacenter"] = dc.Name
		stsLabels["app.kubernetes.io/name"] = cc.Name
	}
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
			PodManagementPolicy: appsv1.ParallelPodManagement,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type:          appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{Partition: proto.Int32(0)},
			},
			RevisionHistoryLimit: proto.Int32(10),
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: stsLabels,
					Annotations: map[string]string{
						// https://kubernetes.io/docs/reference/labels-annotations-taints/#kubectl-kubernetes-io-default-container
						"kubectl.kubernetes.io/default-container": "cassandra",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						cassandraContainer(cc, dc, restartChecksum, clientTLSSecret),
						icarusContainer(cc),
					},
					InitContainers: []v1.Container{
						privilegedInitContainer(cc),
						maintenanceContainer(cc),
						initContainer(cc),
					},
					ImagePullSecrets: imagePullSecrets(cc),
					Volumes: []v1.Volume{
						maintenanceVolume(cc),
						cassandraConfigVolume(cc),
						podsConfigVolume(cc),
						authVolume(cc),
					},
					ServiceAccountName:            names.CassandraServiceAccount(cc.Name),
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

	if cc.Spec.Cassandra.Monitoring.Enabled {
		if cc.Spec.Cassandra.Monitoring.Agent == dbv1alpha1.CassandraAgentTlp {
			desiredSts.Spec.Template.Spec.Volumes = append(desiredSts.Spec.Template.Spec.Volumes, prometheusVolume(cc))
		}
		if cc.Spec.Cassandra.Monitoring.Agent == dbv1alpha1.CassandraAgentDatastax {
			desiredSts.Spec.Template.Spec.Volumes = append(desiredSts.Spec.Template.Spec.Volumes, collectdVolume(cc))
		}
	}

	if cc.Spec.Cassandra.Persistence.Enabled {
		desiredSts.Spec.VolumeClaimTemplates = cassandraVolumeClaims(cc)
	} else {
		desiredSts.Spec.Template.Spec.Volumes = append(desiredSts.Spec.Template.Spec.Volumes, emptyDirDataVolume())
	}

	if cc.Spec.Encryption.Server.InternodeEncryption != dbv1alpha1.InternodeEncryptionNone {
		desiredSts.Spec.Template.Spec.Volumes = append(desiredSts.Spec.Template.Spec.Volumes, cassandraServerTLSVolume(cc))
	}

	if cc.Spec.Encryption.Client.Enabled {
		desiredSts.Spec.Template.Spec.Volumes = append(desiredSts.Spec.Template.Spec.Volumes, cassandraClientTLSVolume(cc))
	}

	if cc.Spec.TopologySpreadByZone != nil && *cc.Spec.TopologySpreadByZone {
		desiredSts.Spec.Template.Spec.TopologySpreadConstraints = []v1.TopologySpreadConstraint{
			{
				MaxSkew:           1,
				TopologyKey:       v1.LabelTopologyZone,
				WhenUnsatisfiable: v1.ScheduleAnyway,
				LabelSelector:     metav1.SetAsLabelSelector(labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra)),
			},
		}
	}

	return desiredSts
}

func cassandraVolumeClaims(cc *dbv1alpha1.CassandraCluster) []v1.PersistentVolumeClaim {
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

func commitLogVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      "commitlog",
		MountPath: cassandraCommitLogDir,
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

func authVolume(cc *dbv1alpha1.CassandraCluster) v1.Volume {
	items := []v1.KeyToPath{
		{
			Key:  dbv1alpha1.CassandraOperatorAdminRole,
			Path: dbv1alpha1.CassandraOperatorAdminRole,
		},
		{
			Key:  dbv1alpha1.CassandraOperatorAdminPassword,
			Path: dbv1alpha1.CassandraOperatorAdminPassword,
		},
		{
			Key:  "icarus-jmx",
			Path: "icarus-jmx",
		},
	}

	if cc.Spec.Encryption.Client.Enabled {
		items = append(items,
			v1.KeyToPath{
				Key:  "nodetool-ssl.properties",
				Path: "nodetool-ssl.properties",
			},
			v1.KeyToPath{
				Key:  "cqlshrc",
				Path: "cqlshrc",
			},
		)
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

	if cc.Spec.JMXAuth == jmxAuthenticationLocalFiles {
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

func cassandraServerTLSVolume(cc *dbv1alpha1.CassandraCluster) v1.Volume {
	return v1.Volume{
		Name: cassandraServerTLSVolumeName,
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: cc.Spec.Encryption.Server.NodeTLSSecret.Name,
				Items: []v1.KeyToPath{
					{
						Key:  cc.Spec.Encryption.Server.NodeTLSSecret.KeystoreFileKey,
						Path: cc.Spec.Encryption.Server.NodeTLSSecret.KeystoreFileKey,
					},
					{
						Key:  cc.Spec.Encryption.Server.NodeTLSSecret.TruststoreFileKey,
						Path: cc.Spec.Encryption.Server.NodeTLSSecret.TruststoreFileKey,
					},
				},
				DefaultMode: proto.Int32(v1.SecretVolumeSourceDefaultMode),
			},
		},
	}
}

func cassandraClientTLSVolume(cc *dbv1alpha1.CassandraCluster) v1.Volume {
	return v1.Volume{
		Name: cassandraClientTLSVolumeName,
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: cc.Spec.Encryption.Client.NodeTLSSecret.Name,
				Items: []v1.KeyToPath{
					{
						Key:  cc.Spec.Encryption.Client.NodeTLSSecret.CACrtFileKey,
						Path: cc.Spec.Encryption.Client.NodeTLSSecret.CACrtFileKey,
					},
					{
						Key:  cc.Spec.Encryption.Client.NodeTLSSecret.CrtFileKey,
						Path: cc.Spec.Encryption.Client.NodeTLSSecret.CrtFileKey,
					},
					{
						Key:  cc.Spec.Encryption.Client.NodeTLSSecret.FileKey,
						Path: cc.Spec.Encryption.Client.NodeTLSSecret.FileKey,
					},
					{
						Key:  cc.Spec.Encryption.Client.NodeTLSSecret.KeystoreFileKey,
						Path: cc.Spec.Encryption.Client.NodeTLSSecret.KeystoreFileKey,
					},
					{
						Key:  cc.Spec.Encryption.Client.NodeTLSSecret.TruststoreFileKey,
						Path: cc.Spec.Encryption.Client.NodeTLSSecret.TruststoreFileKey,
					},
				},
				DefaultMode: proto.Int32(v1.SecretVolumeSourceDefaultMode),
			},
		},
	}
}

func tlsJVMArgs(cc *dbv1alpha1.CassandraCluster, clientTLSSecret *v1.Secret) string {
	return fmt.Sprintf("-Djavax.net.ssl.keyStore=%s/%s -Djavax.net.ssl.keyStorePassword=%s -Djavax.net.ssl.trustStore=%s/%s -Djavax.net.ssl.trustStorePassword=%s",
		cassandraClientTLSDir, cc.Spec.Encryption.Client.NodeTLSSecret.KeystoreFileKey, strings.TrimRight(string(clientTLSSecret.Data[cc.Spec.Encryption.Client.NodeTLSSecret.KeystorePasswordKey]), "\r\n"),
		cassandraClientTLSDir, cc.Spec.Encryption.Client.NodeTLSSecret.TruststoreFileKey, strings.TrimRight(string(clientTLSSecret.Data[cc.Spec.Encryption.Client.NodeTLSSecret.TruststorePasswordKey]), "\r\n"))
}
