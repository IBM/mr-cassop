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
	batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
)

func (r *CassandraClusterReconciler) reconcileKwatcher(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	if err := r.reconcileKwatcherKeyspaceConfigMap(ctx, cc); err != nil {
		return errors.Wrap(err, "failed to reconcile kwatcher configmaps")
	}

	if err := r.reconcileKwatcherRepairJobs(ctx, cc); err != nil {
		return errors.Wrap(err, "failed to reconcile kwatcher jobs")
	}

	if err := r.reconcileKwatcherJobPacementConfigMap(ctx, cc); err != nil {
		return errors.Wrap(err, "failed to reconcile kwatcher jobs")
	}

	if err := r.reconcileKwatcherServiceAccount(ctx, cc); err != nil {
		return errors.Wrap(err, "failed to reconcile kwatcher serviceaccount")
	}

	if err := r.reconcileKwatcherRole(ctx, cc); err != nil {
		return errors.Wrap(err, "failed to reconcile kwatcher role")
	}

	if err := r.reconcileKwatcherRoleBinding(ctx, cc); err != nil {
		return errors.Wrap(err, "failed to reconcile kwatcher rolebinding")
	}

	if err := r.reconcileKwatcherClusterRole(ctx, cc); err != nil {
		return errors.Wrap(err, "failed to reconcile kwatcher clusterrole")
	}

	if err := r.reconcileKwatcherClusterRoleBinding(ctx, cc); err != nil {
		return errors.Wrap(err, "failed to reconcile kwatcher clusterrolebinding")
	}

	for _, dc := range cc.Spec.DCs {
		if err := r.reconcileKwatcherDeployment(ctx, cc, dc.Name); err != nil {
			return errors.Wrap(err, "failed to reconcile kwatcher deployment")
		}
	}

	return nil
}

func (r *CassandraClusterReconciler) reconcileKwatcherDeployment(ctx context.Context, cc *dbv1alpha1.CassandraCluster, dcName string) error {
	kwatcherLabels := labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentKwatcher)
	kwatcherLabels = labels.WithDCLabel(kwatcherLabels, dcName)
	percent25 := intstr.FromInt(25)
	desiredDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.KwatcherDeployment(cc, dcName),
			Namespace: cc.Namespace,
			Labels:    kwatcherLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: proto.Int32(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: kwatcherLabels,
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
					Labels: kwatcherLabels,
				},
				Spec: v1.PodSpec{
					ImagePullSecrets:              imagePullSecrets(cc),
					RestartPolicy:                 v1.RestartPolicyAlways,
					TerminationGracePeriodSeconds: proto.Int64(30),
					DNSPolicy:                     v1.DNSClusterFirst,
					SecurityContext:               &v1.PodSecurityContext{},
					ServiceAccountName:            names.KwatcherServiceAccount(cc),
					Containers: []v1.Container{
						{
							Name:            "kwatcher",
							Image:           cc.Spec.Kwatcher.Image,
							ImagePullPolicy: cc.Spec.Kwatcher.ImagePullPolicy,
							Resources:       cc.Spec.Kwatcher.Resources,
							Command: []string{
								"./kwatcher",
								"-namespace", cc.Namespace,
								"-appname", cc.Name,
								"-hosts", names.DC(cc, dcName),
								"-dcname", dcName,
								"-statefulsetname", names.DC(cc, dcName),
								"-redact",
								"-repairjobimage", cc.Spec.Cassandra.Image,
								"-port", "9042",
							},
							Env: []v1.EnvVar{
								{Name: "USERS_DIR", Value: cassandraRolesDir},
							},
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: v1.TerminationMessageReadFile,
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cc, desiredDeployment, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualDeployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: names.KwatcherDeployment(cc, dcName), Namespace: cc.Namespace}, actualDeployment)
	if err != nil && apierrors.IsNotFound(err) {
		r.Log.Info("Creating kwatcher deployment")
		err = r.Create(ctx, desiredDeployment)
		if err != nil {
			return errors.Wrap(err, "Failed to create deployment")
		}
	} else if err != nil {
		return errors.Wrap(err, "Failed to get deployment")
	} else {
		desiredDeployment.Annotations = actualDeployment.Annotations
		if !compare.EqualDeployment(desiredDeployment, actualDeployment) {
			r.Log.Info("Updating kwatcher deployment")
			r.Log.Debug(compare.DiffDeployment(actualDeployment, desiredDeployment))
			actualDeployment.Spec = desiredDeployment.Spec
			actualDeployment.Labels = desiredDeployment.Labels
			if err = r.Update(ctx, actualDeployment); err != nil {
				return errors.Wrap(err, "failed to update deployment")
			}
		} else {
			r.Log.Debugf("No updates to kwatcher deployment")
		}
	}

	return nil
}

// sets contoller reference for created jobs so that they get deleted when cassandracluster is deleted
func (r *CassandraClusterReconciler) reconcileKwatcherRepairJobs(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	for _, keyspaceName := range cc.Spec.SystemKeyspaces.Names {
		job := &batch.Job{}
		jobName := types.NamespacedName{Name: fmt.Sprintf("%s-repair-job-%s", cc.Name, strings.Replace(string(keyspaceName), "_", "-", -1)), Namespace: cc.Namespace}
		err := r.Get(ctx, jobName, job)
		if err == nil { //if found
			if len(job.OwnerReferences) != 0 { //controller reference already set
				continue
			}
			if err := controllerutil.SetControllerReference(cc, job, r.Scheme); err != nil {
				return errors.Wrap(err, "Cannot set controller reference")
			}
			r.Log.Debugf("Updating controller reference for job %q", jobName)
			err := r.Update(ctx, job)
			if err != nil {
				return errors.Wrap(err, "Cannot update job")
			}
			continue
		} else if apierrors.IsNotFound(err) {
			continue
		} else {
			return errors.Wrap(err, "Cannot get job info")
		}
	}

	return nil
}
