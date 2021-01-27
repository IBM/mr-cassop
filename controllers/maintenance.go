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
	if err = r.Status().Update(ctx, actualCC); err != nil {
		return errors.Wrap(err, "Failed to update maintenance status")
	}
	return nil
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
		return errors.Wrap(err, "Cannot set controller reference")
	}
	actualCM := &v1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: desiredCM.Name, Namespace: desiredCM.Namespace}, actualCM)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Infof("Creating %s", desiredCM.Name)
		if err = r.Create(ctx, desiredCM); err != nil {
			return errors.Wrapf(err, "Unable to create %s", desiredCM.Name)
		}
	} else if err != nil {
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

func (r *CassandraClusterReconciler) getCassandraPods(ctx context.Context, namespace string) (*v1.PodList, error) {
	pods := &v1.PodList{}
	err := r.List(ctx, pods, client.HasLabels{CassandraEndpointLabels}, client.InNamespace(namespace))
	if err != nil {
		return nil, errors.Wrap(err, "Could not list pods")
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
	err = r.Status().Patch(ctx, pod, podPatch)
	if err != nil {
		return errors.Wrapf(err, "Could not patch pod status for pod %s", podName)
	}
	return nil
}

func (r *CassandraClusterReconciler) patchConfigMap(ctx context.Context, namespace string, mapName string, data map[string]string) error {
	configMap, err := r.getConfigMap(ctx, mapName, namespace)
	if err != nil {
		return err
	}
	mapPatch := client.MergeFrom(configMap.DeepCopy())
	configMap.Data = data
	err = r.Patch(ctx, configMap, mapPatch)
	if err != nil {
		return errors.Wrapf(err, "Could not patch config map %s", mapName)
	}
	return nil
}

func (r *CassandraClusterReconciler) checkMaintenanceRunning(pod v1.Pod) (bool, error) {
	for _, container := range pod.Status.InitContainerStatuses {
		if container.Name == "maintenance-mode" && container.State.Running != nil {
			return true, nil
		}
	}
	return false, nil
}

func (r *CassandraClusterReconciler) checkCassandraRunning(pod v1.Pod) (bool, error) {
	for _, container := range pod.Status.ContainerStatuses {
		if container.Name == "cassandra" && container.State.Running != nil {
			return true, nil
		}
	}
	return false, nil
}

func (r *CassandraClusterReconciler) processMaintenanceRequest(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	pods, err := r.getCassandraPods(ctx, cc.Namespace)
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		isRunning, err := r.checkMaintenanceRunning(pod)
		if err != nil {
			return err
		}
		if inMaintenanceSpec(cc.Spec.Maintenance, pod.Name) && !isRunning {
			err := r.updateMaintenanceMode(ctx, cc, true, pod)
			if err != nil {
				return err
			}
		} else if isRunning {
			err := r.updateMaintenanceMode(ctx, cc, false, pod)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *CassandraClusterReconciler) updateMaintenanceMode(ctx context.Context, cc *dbv1alpha1.CassandraCluster, enabled bool, pod v1.Pod) error {
	isRunning, err := r.checkCassandraRunning(pod)
	if err != nil {
		return err
	}
	if err := r.updateMaintenanceConfigMap(ctx, cc.Namespace, names.MaintenanceConfigMap(cc.Name), enabled, pod.Name); err != nil {
		return err
	}
	if enabled && isRunning {
		if err := r.patchPodStatusPhase(ctx, cc.Namespace, pod.Name, v1.PodFailed); err != nil {
			return err
		}
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
	if err := r.patchConfigMap(ctx, namespace, configMap.Name, data); err != nil {
		return err
	}
	return nil
}

func (r *CassandraClusterReconciler) generateMaintenanceStatus(ctx context.Context, cc *dbv1alpha1.CassandraCluster) ([]dbv1alpha1.Maintenance, error) {
	var status []dbv1alpha1.Maintenance
	pods, err := r.getCassandraPods(ctx, cc.Namespace)
	if err != nil {
		return nil, err
	}
	for _, pod := range pods.Items {
		dcName := pod.Labels[dbv1alpha1.CassandraClusterDC]
		isRunning, err := r.checkMaintenanceRunning(pod)
		if err != nil {
			return nil, err
		}
		if isRunning {
			if containsDc(status, dcName) {
				for i, entry := range status {
					if entry.DC == dcName {
						status[i].Pods = append(status[i].Pods, pod.Name)
						break
					}
				}
			} else {
				status = append(status, dbv1alpha1.Maintenance{
					DC:   dcName,
					Pods: []string{pod.Name},
				})
			}
		}
	}
	for _, entry := range status {
		sort.Strings(entry.Pods)
	}
	return status, nil
}

func inMaintenanceSpec(m []dbv1alpha1.Maintenance, podName string) bool {
	for _, entry := range m {
		for _, pod := range entry.Pods {
			if pod == podName {
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
