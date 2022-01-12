package controllers

import (
	"context"
	"fmt"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/compare"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/reaper"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CassandraClusterReconciler) reconcileReaperPrerequisites(ctx context.Context, cc *dbv1alpha1.CassandraCluster, cqlClient cql.CqlClient, allDCs []dbv1alpha1.DC) error {
	if err := r.reconcileShiroConfigMap(ctx, cc); err != nil {
		return errors.Wrap(err, "Error reconciling shiro configmap")
	}

	if err := r.reconcileReaperKeyspace(cc, cqlClient, allDCs); err != nil {
		return errors.Wrap(err, "Error reconciling reaper keyspace")
	}

	return nil
}

func (r *CassandraClusterReconciler) reconcileReaper(ctx context.Context, cc *dbv1alpha1.CassandraCluster) (ctrl.Result, error) {
	for index, dc := range cc.Spec.DCs {
		if err := r.reconcileReaperDeployment(ctx, cc, dc); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile reaper deployment")
		}

		if err := r.reconcileReaperService(ctx, cc); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile reaper service")
		}

		if err := r.reconcileReaperServiceMonitor(ctx, cc); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "Failed to reconcile reaper service monitor")
		}

		if index == 0 { // Wait for 1st reaper deployment to finish, otherwise we can get an error 'Schema migration is locked by another instance'
			reaperDeployment := &appsv1.Deployment{}
			err := r.Get(ctx, types.NamespacedName{Name: names.ReaperDeployment(cc.Name, dc.Name), Namespace: cc.Namespace}, reaperDeployment)
			if err != nil {
				return ctrl.Result{}, errors.Wrap(err, "Failed to get reaper deployment")
			}

			if reaperDeployment.Status.ReadyReplicas != dbv1alpha1.ReaperReplicasNumber {
				r.Log.Infof("Waiting for the first reaper deployment to be ready. Trying again in %s...", r.Cfg.RetryDelay)
				return ctrl.Result{RequeueAfter: r.Cfg.RetryDelay}, nil
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *CassandraClusterReconciler) reconcileReaperDeployment(ctx context.Context, cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC) error {
	adminRoleSecret := &v1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: names.ActiveAdminSecret(cc.Name)}, adminRoleSecret)
	if err != nil {
		return errors.Wrap(err, "can't get admin role secret")
	}
	adminSecretChecksum := util.Sha1(fmt.Sprintf("%v", adminRoleSecret.Data))

	clientTLSSecret := &v1.Secret{}

	if cc.Spec.Encryption.Client.Enabled {
		clientTLSSecret, err = r.getSecret(ctx, cc.Spec.Encryption.Client.TLSSecret.Name, cc.Namespace)
		if err != nil {
			return err
		}
	}

	reaperLabels := labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentReaper)
	reaperLabels = labels.WithDCLabel(reaperLabels, dc.Name)
	percent25 := intstr.FromInt(25)
	desiredDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ReaperDeployment(cc.Name, dc.Name),
			Namespace: cc.Namespace,
			Labels:    reaperLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: proto.Int32(dbv1alpha1.ReaperReplicasNumber),
			Selector: &metav1.LabelSelector{
				MatchLabels: reaperLabels,
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &percent25,
					MaxSurge:       &percent25,
				},
			},
			RevisionHistoryLimit:    proto.Int32(10),
			ProgressDeadlineSeconds: proto.Int32(1200),
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: reaperLabels,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						reaperContainer(cc, dc, adminSecretChecksum, clientTLSSecret),
					},
					Volumes:                       reaperVolumes(cc),
					ImagePullSecrets:              imagePullSecrets(cc),
					Tolerations:                   cc.Spec.Reaper.Tolerations,
					NodeSelector:                  cc.Spec.Reaper.NodeSelector,
					RestartPolicy:                 v1.RestartPolicyAlways,
					TerminationGracePeriodSeconds: proto.Int64(30),
					DNSPolicy:                     v1.DNSClusterFirst,
					SecurityContext:               &v1.PodSecurityContext{},
				},
			},
		},
	}

	r.Log.Debug("Reconciling Reaper Deployment")

	desiredDeployment.Spec.Template.Spec.Volumes = append(desiredDeployment.Spec.Template.Spec.Volumes, cassandraConfigVolume(cc))
	if err = controllerutil.SetControllerReference(cc, desiredDeployment, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: names.ReaperDeployment(cc.Name, dc.Name), Namespace: cc.Namespace}, actualDeployment)
	if err != nil && apierrors.IsNotFound(err) {
		r.Log.Info("Creating reaper deployment")
		err = r.Create(ctx, desiredDeployment)
		if err != nil {
			return errors.Wrap(err, "Failed to create deployment")
		}
	} else if err != nil {
		return errors.Wrap(err, "Failed to get deployment")
	} else {
		desiredDeployment.Annotations = util.MergeMap(actualDeployment.Annotations, desiredDeployment.Annotations)
		if !compare.EqualDeployment(desiredDeployment, actualDeployment) {
			r.Log.Info("Updating reaper deployment")
			r.Log.Debug(compare.DiffDeployment(actualDeployment, desiredDeployment))
			actualDeployment.Labels = util.MergeMap(actualDeployment.Labels, desiredDeployment.Labels)
			actualDeployment.Spec = desiredDeployment.Spec

			if err = r.Update(ctx, actualDeployment); err != nil {
				return errors.Wrap(err, "Failed to update deployment")
			}
		} else {
			r.Log.Debugf("No updates to reaper deployment")
		}
	}

	return nil
}

func (r *CassandraClusterReconciler) reconcileReaperService(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	desiredService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ReaperService(cc.Name),
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentReaper),
			Namespace: cc.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       "app",
					Protocol:   v1.ProtocolTCP,
					Port:       dbv1alpha1.ReaperAppPort,
					TargetPort: intstr.FromInt(dbv1alpha1.ReaperAppPort),
				},
				{
					Name:       "admin",
					Protocol:   v1.ProtocolTCP,
					Port:       dbv1alpha1.ReaperAdminPort,
					TargetPort: intstr.FromInt(dbv1alpha1.ReaperAdminPort),
				},
			},
			ClusterIP:                v1.ClusterIPNone,
			Type:                     v1.ServiceTypeClusterIP,
			SessionAffinity:          v1.ServiceAffinityNone,
			PublishNotReadyAddresses: true,
			Selector:                 labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentReaper),
		},
	}

	if err := controllerutil.SetControllerReference(cc, desiredService, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualService := &v1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: names.ReaperService(cc.Name), Namespace: cc.Namespace}, actualService)
	if err != nil && apierrors.IsNotFound(err) {
		r.Log.Infof("Creating reaper service")
		err = r.Create(ctx, desiredService)
		if err != nil {
			return errors.Wrapf(err, "Failed to create reaper service")
		}
	} else if err != nil {
		return errors.Wrapf(err, "Failed to get reaper service")
	} else {
		// ClusterIP is immutable once created, so always enforce the same as existing
		desiredService.Spec.ClusterIP = actualService.Spec.ClusterIP
		desiredService.Spec.ClusterIPs = actualService.Spec.ClusterIPs
		desiredService.Spec.IPFamilies = actualService.Spec.IPFamilies
		desiredService.Spec.IPFamilyPolicy = actualService.Spec.IPFamilyPolicy
		desiredService.Spec.InternalTrafficPolicy = actualService.Spec.InternalTrafficPolicy
		if !compare.EqualService(desiredService, actualService) {
			r.Log.Infof("Updating reaper service")
			r.Log.Debugf(compare.DiffService(actualService, desiredService))
			actualService.Spec = desiredService.Spec
			actualService.Labels = desiredService.Labels
			actualService.Annotations = desiredService.Annotations
			if err = r.Update(ctx, actualService); err != nil {
				return errors.Wrapf(err, "failed to update reaper service")
			}
		} else {
			r.Log.Debugf("No updates to reaper service")
		}
	}
	return nil
}

func reaperContainer(cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC, adminSecretChecksum string, clientTLSSecret *v1.Secret) v1.Container {
	container := v1.Container{
		Name:            "reaper",
		Image:           cc.Spec.Reaper.Image,
		ImagePullPolicy: cc.Spec.Reaper.ImagePullPolicy,
		Ports: []v1.ContainerPort{
			{
				Name:          "app",
				ContainerPort: dbv1alpha1.ReaperAppPort,
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "admin",
				ContainerPort: dbv1alpha1.ReaperAdminPort,
				Protocol:      v1.ProtocolTCP,
			},
		},
		ReadinessProbe: &v1.Probe{
			ProbeHandler: v1.ProbeHandler{
				HTTPGet: &v1.HTTPGetAction{
					Port:   intstr.FromString("admin"),
					Path:   "/ping",
					Scheme: v1.URISchemeHTTP,
				},
			},
			TimeoutSeconds:      1,
			PeriodSeconds:       10,
			SuccessThreshold:    1,
			FailureThreshold:    3,
			InitialDelaySeconds: 60,
		},
		LivenessProbe: &v1.Probe{
			ProbeHandler: v1.ProbeHandler{
				HTTPGet: &v1.HTTPGetAction{
					Port:   intstr.FromString("admin"),
					Path:   "/ping",
					Scheme: v1.URISchemeHTTP,
				},
			},
			TimeoutSeconds:      1,
			PeriodSeconds:       10,
			SuccessThreshold:    1,
			FailureThreshold:    3,
			InitialDelaySeconds: 600, //first init may take a long time because of the DB migration
		},
		Resources: cc.Spec.Reaper.Resources,
		Env:       reaperEnvironment(cc, dc, adminSecretChecksum, clientTLSSecret),
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      "shiro-config",
				MountPath: "/shiro/shiro.ini",
				SubPath:   "shiro.ini",
			},
		},
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: v1.TerminationMessageReadFile,
	}

	if cc.Spec.Encryption.Client.Enabled {
		container.VolumeMounts = append(container.VolumeMounts, v1.VolumeMount{
			Name:      cassandraClientTLSVolumeName,
			MountPath: cassandraClientTLSDir,
		})
	}

	return container
}

func reaperVolumes(cc *dbv1alpha1.CassandraCluster) []v1.Volume {
	volume := []v1.Volume{
		{
			Name:         "reaper-auth",
			VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}},
		},
		{
			Name: "shiro-config",
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: names.ShiroConfigMap(cc.Name),
					},
					DefaultMode: proto.Int32(v1.ConfigMapVolumeSourceDefaultMode),
				},
			},
		},
	}

	if cc.Spec.Encryption.Client.Enabled {
		volume = append(volume, v1.Volume{
			Name: cassandraClientTLSVolumeName,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: cc.Spec.Encryption.Client.TLSSecret.Name,
					Items: []v1.KeyToPath{
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
		})
	}

	return volume
}

func (r CassandraClusterReconciler) reaperInitialization(ctx context.Context, cc *dbv1alpha1.CassandraCluster, reaperClient reaper.ReaperClient) error {
	seed := getSeedHostname(cc, cc.Spec.DCs[0].Name, 0, true)
	clusterExists, err := reaperClient.ClusterExists(ctx)
	if err != nil {
		return err
	}
	if !clusterExists {
		r.Log.Infof("Cluster %s does not exist in reaper. Adding cluster seed...", cc.Name)
		if err = reaperClient.AddCluster(ctx, seed); err != nil {
			return err
		}

		r.Log.Infof("starting a repair for %s keyspace", cc.Spec.Reaper.Keyspace)
		err = reaperClient.RunRepair(ctx, cc.Spec.Reaper.Keyspace, repairCauseReaperInit)
		if err != nil {
			return errors.Wrapf(err, "failed to run repair on %s keyspace", cc.Spec.Reaper.Keyspace)
		}

		// we had to modify system_auth keyspace before we had reaper in order to bootstrap the cluster. So run the missing repair now.
		r.Log.Info("starting a repair for system_auth keyspace")
		err = reaperClient.RunRepair(ctx, keyspaceSystemAuth, repairCauseReaperInit)
		if err != nil {
			return errors.Wrap(err, "failed to run repair on system_auth keyspace")
		}
	}
	return nil
}
