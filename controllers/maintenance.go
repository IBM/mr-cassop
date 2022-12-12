package controllers

import (
	"context"
	"fmt"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/pkg/errors"
	"k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sort"
)

func (r *CassandraClusterReconciler) reconcileMaintenance(ctx context.Context, desiredCC *dbv1alpha1.CassandraCluster) error {
	err := r.processMaintenanceRequest(ctx, desiredCC)
	if err != nil {
		return errors.Wrap(err, "Failed to process maintenance mode request")
	}
	status, err := r.generateMaintenanceStatus(ctx, desiredCC)
	if err != nil {
		return errors.Wrap(err, "Failed to generate status")
	}
	r.Log.Debugf("Spec: %s, Status: %s", fmt.Sprint(desiredCC.Spec.Maintenance), fmt.Sprint(status))
	actualCC := desiredCC.DeepCopy()
	actualCC.Status.MaintenanceState = status
	return r.Status().Update(ctx, actualCC)
}

func (r *CassandraClusterReconciler) reconcileMaintenanceConfigMap(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	desiredCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.MaintenanceConfigMap(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra),
		},
		Data: map[string]string{},
	}
	if err := controllerutil.SetControllerReference(cc, desiredCM, r.Scheme); err != nil {
		return errors.Wrapf(err, "Cannot set controller reference: %s", err.Error())
	}
	actualCM := &v1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: desiredCM.Name, Namespace: desiredCM.Namespace}, actualCM)
	if err != nil {
		if kerrors.IsNotFound(err) {
			r.Log.Infof("Creating %s", desiredCM.Name)
			return r.Create(ctx, desiredCM)
		}
		return errors.Wrapf(err, "Could not get %s", desiredCM.Name)
	}
	return nil
}

func (r *CassandraClusterReconciler) getPod(ctx context.Context, namespace, podName string) (*v1.Pod, error) {
	pod := &v1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: namespace}, pod)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not get pod %s", podName)
	}
	return pod, nil
}

func (r *CassandraClusterReconciler) getCassandraPods(ctx context.Context, cc *dbv1alpha1.CassandraCluster) (*v1.PodList, error) {
	pods := &v1.PodList{}
	err := r.List(ctx, pods, client.InNamespace(cc.Namespace), client.MatchingLabels(labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra)))
	if err != nil {
		return nil, errors.Wrap(err, "Could not list Cassandra pods")
	}
	return pods, nil
}

func (r *CassandraClusterReconciler) patchPodStatusPhase(ctx context.Context, namespace string, podName string, phase v1.PodPhase) error {
	pod, err := r.getPod(ctx, namespace, podName)
	if err != nil {
		return err
	}
	podPatch := client.MergeFrom(pod.DeepCopy())
	pod.Status.Phase = phase
	return r.Status().Patch(ctx, pod, podPatch)
}

func (r *CassandraClusterReconciler) patchConfigMap(ctx context.Context, namespace string, mapName string, data map[string]string) error {
	configMap, err := r.getConfigMap(ctx, mapName, namespace)
	if err != nil {
		return err
	}
	mapPatch := client.MergeFrom(configMap.DeepCopy())
	configMap.Data = data
	return r.Patch(ctx, configMap, mapPatch)
}

func (r *CassandraClusterReconciler) checkMaintenanceRunning(pod v1.Pod) bool {
	for _, container := range pod.Status.InitContainerStatuses {
		if container.Name == "maintenance-mode" && container.State.Running != nil {
			return true
		}
	}
	return false
}

func (r *CassandraClusterReconciler) checkCassandraRunning(pod v1.Pod) bool {
	for _, container := range pod.Status.ContainerStatuses {
		if container.Name == "cassandra" && container.State.Running != nil {
			return true
		}
	}
	return false
}

func (r *CassandraClusterReconciler) processMaintenanceRequest(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	pods, err := r.getCassandraPods(ctx, cc)
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		isRunning := r.checkMaintenanceRunning(pod)
		if inMaintenanceSpec(cc.Spec.Maintenance, pod.Name) && !isRunning {
			return r.updateMaintenanceMode(ctx, cc, true, pod)
		} else if !inMaintenanceSpec(cc.Spec.Maintenance, pod.Name) && isRunning {
			return r.updateMaintenanceMode(ctx, cc, false, pod)
		}
	}
	return nil
}

func (r *CassandraClusterReconciler) updateMaintenanceMode(ctx context.Context, cc *dbv1alpha1.CassandraCluster, enabled bool, pod v1.Pod) error {
	isRunning := r.checkCassandraRunning(pod)
	if err := r.updateMaintenanceConfigMap(ctx, cc.Namespace, names.MaintenanceConfigMap(cc.Name), enabled, pod.Name); err != nil {
		return err
	}
	if enabled && isRunning {
		return r.patchPodStatusPhase(ctx, cc.Namespace, pod.Name, v1.PodFailed)
	}
	return nil
}

func (r *CassandraClusterReconciler) updateMaintenanceConfigMap(ctx context.Context, namespace string, cmName string, enabled bool, pod string) error {
	configMap, err := r.getConfigMap(ctx, cmName, namespace)
	if err != nil {
		return err
	}
	data := configMap.Data
	if data == nil {
		data = map[string]string{}
	}
	if enabled {
		data[pod] = "true"
	} else {
		delete(data, pod)
	}
	return r.patchConfigMap(ctx, namespace, configMap.Name, data)
}

func (r *CassandraClusterReconciler) generateMaintenanceStatus(ctx context.Context, cc *dbv1alpha1.CassandraCluster) ([]dbv1alpha1.Maintenance, error) {
	status := make([]dbv1alpha1.Maintenance, 0)
	pods, err := r.getCassandraPods(ctx, cc)
	if err != nil {
		return nil, err
	}
	for _, pod := range pods.Items {
		dcName := pod.Labels[dbv1alpha1.CassandraClusterDC]
		isRunning := r.checkMaintenanceRunning(pod)
		if isRunning {
			if containsDc(status, dcName) {
				for i, entry := range status {
					if entry.DC == dcName {
						status[i].Pods = append(status[i].Pods, dbv1alpha1.PodName(pod.Name))
						break
					}
				}
			} else {
				status = append(status, dbv1alpha1.Maintenance{
					DC:   dcName,
					Pods: []dbv1alpha1.PodName{dbv1alpha1.PodName(pod.Name)},
				})
			}
		}
	}
	for _, entry := range status {
		sort.Slice(entry.Pods, func(i, j int) bool {
			return string(entry.Pods[i]) < string(entry.Pods[j])
		})
	}
	return status, nil
}

func inMaintenanceSpec(m []dbv1alpha1.Maintenance, podName string) bool {
	for _, entry := range m {
		for _, pod := range entry.Pods {
			if string(pod) == podName {
				return true
			}
		}
	}
	return false
}

func containsDc(m []dbv1alpha1.Maintenance, dcName string) bool {
	for _, entry := range m {
		if entry.DC == dcName {
			return true
		}
	}
	return false
}
