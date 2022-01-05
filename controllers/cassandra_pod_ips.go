package controllers

import (
	"context"

	"github.com/google/go-cmp/cmp"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/compare"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CassandraClusterReconciler) reconcilePodIPsConfigMap(ctx context.Context, cc *v1alpha1.CassandraCluster, pods []v1.Pod, broadcastAddresses map[string]string) (map[string]string, error) {
	desiredCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.PodIPsConfigMap(cc.Name),
			Namespace: cc.Namespace,
		},
	}

	err := controllerutil.SetControllerReference(cc, desiredCM, r.Scheme)
	if err != nil {
		return nil, errors.Wrap(err, "can't set controller reference")
	}

	actualCM := &v1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: names.PodIPsConfigMap(cc.Name), Namespace: cc.Namespace}, actualCM)
	if err != nil && apierrors.IsNotFound(err) {
		err = r.Create(ctx, desiredCM)
		if err != nil {
			return nil, errors.Wrap(err, "can't create pod IPs configmap")
		}
		return desiredCM.Data, nil
	} else if err != nil {
		return nil, errors.Wrap(err, "can't get pod IPs configmap")
	} else {
		if actualCM.Data == nil {
			actualCM.Data = make(map[string]string)
		}

		// copy the previous data to save IPs for pods that doesn't exist anymore (scale down)
		data := util.MergeMap(make(map[string]string, len(broadcastAddresses)), actualCM.Data)

		for _, pod := range pods {
			if data[pod.Name] != broadcastAddresses[pod.Name] && broadcastAddresses[pod.Name] != "" && podReady(pod) {
				data[pod.Name] = broadcastAddresses[pod.Name]

			}
		}

		desiredCM.Annotations = util.MergeMap(actualCM.Annotations, desiredCM.Annotations)
		desiredCM.Data = data
		if !compare.EqualConfigMap(actualCM, desiredCM) {
			r.Log.Info("Updating pod IPs configmap")
			r.Log.Debugf(compare.DiffConfigMap(actualCM, desiredCM))
			r.Log.Debugf(cmp.Diff(actualCM.Data, desiredCM.Data))
			actualCM.Data = data
			actualCM.Labels = desiredCM.Labels
			actualCM.OwnerReferences = desiredCM.OwnerReferences
			actualCM.Annotations = desiredCM.Annotations
			err = r.Update(ctx, actualCM)
			if err != nil {
				return nil, errors.Wrap(err, "can't update pod IPs configmap")
			}
		} else {
			r.Log.Debugf("No updates for pod IPs configmap")
		}
	}

	return actualCM.Data, nil
}

func podReady(pod v1.Pod) bool {
	if len(pod.Status.ContainerStatuses) == 0 {
		return false
	}
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if !containerStatus.Ready {
			return false
		}
	}

	return true
}
