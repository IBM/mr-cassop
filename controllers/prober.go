package controllers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/compare"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
)

func (r *CassandraClusterReconciler) reconcileProber(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	if err := r.reconcileProberServiceAccount(ctx, cc); err != nil {
		return errors.Wrap(err, "Error reconciling prober serviceaccount")
	}

	if err := r.reconcileProberRole(ctx, cc); err != nil {
		return errors.Wrap(err, "Error reconciling prober role")
	}

	if err := r.reconcileProberRoleBinding(ctx, cc); err != nil {
		return errors.Wrap(err, "Error reconciling prober rolebinding")
	}

	if err := r.reconcileProberDeployment(ctx, cc); err != nil {
		return errors.Wrap(err, "failed to reconcile prober deployment")
	}

	if err := r.reconcileProberService(ctx, cc); err != nil {
		return errors.Wrap(err, "failed to reconcile prober service")
	}

	if cc.Spec.HostPort.Enabled {
		if err := r.reconcileProberIngress(ctx, cc); err != nil {
			return errors.Wrap(err, "failed to reconcile prober ingress")
		}
	}

	return nil
}

func (r *CassandraClusterReconciler) reconcileProberDeployment(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	proberLabels := labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentProber)
	percent25 := intstr.FromInt(25)
	desiredDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ProberDeployment(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentProber),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: proto.Int32(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: proberLabels,
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &percent25,
					MaxSurge:       &percent25,
				},
			},
			RevisionHistoryLimit:    proto.Int32(10),
			ProgressDeadlineSeconds: proto.Int32(600),
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: proberLabels,
				},
				Spec: v1.PodSpec{
					ImagePullSecrets: imagePullSecrets(cc),
					Volumes: []v1.Volume{
						rolesVolume(cc),
					},
					RestartPolicy:                 v1.RestartPolicyAlways,
					TerminationGracePeriodSeconds: proto.Int64(30),
					DNSPolicy:                     v1.DNSClusterFirst,
					SecurityContext:               &v1.PodSecurityContext{},
					ServiceAccountName:            names.ProberServiceAccount(cc.Name),
					Containers: []v1.Container{
						proberContainer(cc),
						jolokiaContainer(cc),
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cc, desiredDeployment, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualDeployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: names.ProberDeployment(cc.Name), Namespace: cc.Namespace}, actualDeployment)
	if err != nil && apierrors.IsNotFound(err) {
		r.Log.Info("Creating prober deployment")
		err = r.Create(ctx, desiredDeployment)
		if err != nil {
			return errors.Wrap(err, "Failed to create deployment")
		}
	} else if err != nil {
		return errors.Wrap(err, "Failed to get deployment")
	} else {
		desiredDeployment.Annotations = actualDeployment.Annotations
		if !compare.EqualDeployment(desiredDeployment, actualDeployment) {
			r.Log.Info("Updating prober deployment")
			r.Log.Debug(compare.DiffDeployment(actualDeployment, desiredDeployment))
			actualDeployment.Spec = desiredDeployment.Spec
			actualDeployment.Labels = desiredDeployment.Labels
			if err = r.Update(ctx, actualDeployment); err != nil {
				return errors.Wrap(err, "failed to update deployment")
			}
		} else {
			r.Log.Debugf("No updates to prober deployment")
		}
	}

	return nil
}

func (r *CassandraClusterReconciler) reconcileProberService(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	desiredService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ProberService(cc.Name),
			Namespace: cc.Namespace,
		},
		Spec: v1.ServiceSpec{
			Type:     v1.ServiceTypeClusterIP,
			Selector: labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentProber),
			Ports: []v1.ServicePort{
				{
					Port:       80,
					Name:       "prober",
					TargetPort: intstr.FromString("prober-server"),
					Protocol:   v1.ProtocolTCP,
				},
			},
			SessionAffinity: v1.ServiceAffinityNone,
		},
	}

	if err := controllerutil.SetControllerReference(cc, desiredService, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualService := &v1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: names.ProberService(cc.Name), Namespace: cc.Namespace}, actualService)
	if err != nil && apierrors.IsNotFound(err) {
		r.Log.Info("Creating prober Service")
		err = r.Create(ctx, desiredService)
		if err != nil {
			return errors.Wrap(err, "Failed to create service")
		}
	} else if err != nil {
		return errors.Wrap(err, "Failed to get service")
	} else {
		// ClusterIP is immutable once created, so always enforce the same as existing
		desiredService.Spec.ClusterIP = actualService.Spec.ClusterIP
		desiredService.Spec.ClusterIPs = actualService.Spec.ClusterIPs
		desiredService.Spec.IPFamilies = actualService.Spec.IPFamilies
		desiredService.Spec.IPFamilyPolicy = actualService.Spec.IPFamilyPolicy
		if !compare.EqualService(desiredService, actualService) {
			r.Log.Info("Updating prober service")
			r.Log.Debugf(compare.DiffService(actualService, desiredService))
			actualService.Spec = desiredService.Spec
			actualService.Labels = desiredService.Labels
			actualService.Annotations = desiredService.Annotations
			if err = r.Update(ctx, actualService); err != nil {
				return errors.Wrap(err, "failed to update service")
			}
		} else {
			r.Log.Debugf("No updates to prober service")
		}
	}

	return nil
}

func proberContainer(cc *dbv1alpha1.CassandraCluster) v1.Container {
	return v1.Container{
		Name:            "prober",
		Image:           cc.Spec.Prober.Image,
		ImagePullPolicy: cc.Spec.Prober.ImagePullPolicy,
		Resources:       cc.Spec.Prober.Resources,
		Env: []v1.EnvVar{
			{Name: "POD_NAMESPACE", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.namespace"}}},
			{Name: "DEBUG", Value: fmt.Sprintf("%t", cc.Spec.Prober.Debug)},
			{Name: "JOLOKIA_PORT", Value: strconv.Itoa(dbv1alpha1.JolokiaContainerPort)},
			{Name: "SERVER_PORT", Value: strconv.Itoa(dbv1alpha1.ProberContainerPort)},
			{Name: "JMX_POLL_PERIOD_SECONDS", Value: "10"},
			{Name: "JMX_PORT", Value: fmt.Sprintf("%d", dbv1alpha1.JmxPort)},
			{Name: "ROLES_DIR", Value: cassandraRolesDir},
		},
		Ports: []v1.ContainerPort{
			{
				Name:          "prober-server",
				ContainerPort: dbv1alpha1.ProberContainerPort,
				Protocol:      v1.ProtocolTCP,
			},
		},
		ReadinessProbe: &v1.Probe{
			Handler: v1.Handler{
				HTTPGet: &v1.HTTPGetAction{
					Port:   intstr.FromString("prober-server"),
					Path:   "/ping",
					Scheme: v1.URISchemeHTTP,
				},
			},
			TimeoutSeconds:   1,
			PeriodSeconds:    10,
			SuccessThreshold: 1,
			FailureThreshold: 3,
		},
		VolumeMounts: []v1.VolumeMount{
			rolesVolumeMount(),
		},
		WorkingDir:               "/",
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: v1.TerminationMessageReadFile,
	}
}

func jolokiaContainer(cc *dbv1alpha1.CassandraCluster) v1.Container {
	return v1.Container{
		Name:            "jolokia",
		Image:           cc.Spec.Prober.Jolokia.Image,
		ImagePullPolicy: cc.Spec.Prober.Jolokia.ImagePullPolicy,
		Resources:       cc.Spec.Prober.Jolokia.Resources,
		Ports: []v1.ContainerPort{
			{
				Name:          "jolokia",
				ContainerPort: dbv1alpha1.JolokiaContainerPort,
				Protocol:      v1.ProtocolTCP,
			},
		},
		ReadinessProbe: &v1.Probe{
			Handler: v1.Handler{
				HTTPGet: &v1.HTTPGetAction{
					Port:   intstr.FromString("jolokia"),
					Path:   "/jolokia",
					Scheme: v1.URISchemeHTTP,
				},
			},
			TimeoutSeconds:   1,
			PeriodSeconds:    10,
			SuccessThreshold: 1,
			FailureThreshold: 3,
		},
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: v1.TerminationMessageReadFile,
	}
}
