package controllers

import (
	"context"
	"strconv"

	"github.com/ibm/cassandra-operator/controllers/util"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/nodectl"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *CassandraClusterReconciler) reconcileCassandraScaling(ctx context.Context, cc *dbv1alpha1.CassandraCluster, podList *v1.PodList, nodesList *v1.NodeList) error {
	broadcastAddresses, err := getBroadcastAddresses(cc, podList.Items, nodesList.Items)
	if err != nil {
		return errors.Wrap(err, "can't get broadcast addresses")
	}

	for _, dc := range cc.Spec.DCs {
		sts := &appsv1.StatefulSet{}
		err := r.Get(ctx, types.NamespacedName{Name: names.DC(cc.Name, dc.Name), Namespace: cc.Namespace}, sts)
		if err != nil {
			return errors.Wrap(err, "can't get statefulset info")
		}
		var oldReplicas int32 = 1
		var newReplicas int32 = 1
		if sts.Spec.Replicas != nil {
			oldReplicas = *sts.Spec.Replicas
		}

		newReplicas = *dc.Replicas

		if oldReplicas == newReplicas { // no need for scaling
			continue
		}

		if oldReplicas < newReplicas { // scale up
			sts.Spec.Replicas = &newReplicas
			err = r.Update(ctx, sts)
			if err != nil {
				return errors.Wrap(err, "can't update replicas number for statefulset")
			}

			continue
		}

		// scale down
		if len(podList.Items) == 0 {
			r.Log.Warn("No pods found to perform scaledown")
			continue
		}

		decommissionPodName := names.DC(cc.Name, dc.Name) + "-" + strconv.Itoa(int(oldReplicas)-1)

		jobName := "pod-decommission-" + decommissionPodName
		if r.Jobs.Exists(jobName) && r.Jobs.IsRunning(jobName) {
			r.Log.Infof("decommission in progress, waiting to finish")
			return nil
		}

		var decommissionPod *v1.Pod
		for i, cassPod := range podList.Items {
			if cassPod.Name == decommissionPodName {
				decommissionPod = &podList.Items[i]
				break
			}
		}

		if decommissionPod == nil {
			return errors.Errorf("couldn't find pod %q to start the decommission", decommissionPodName)
		}

		adminSecret, err := r.adminRoleSecret(ctx, cc)
		if err != nil {
			r.Log.Warnf("can't get admin secret %s", adminSecret.Name)
		}

		roleName, rolePassword, err := extractCredentials(adminSecret)
		if err != nil {
			r.Log.Warn("can't extract secret data")
		}
		nctl := r.NodectlClient(jolokiaURL(cc).String(), roleName, rolePassword, r.Log)

		broadcastIP := broadcastAddresses[decommissionPod.Name]
		opMode, err := nctl.OperationMode(broadcastIP)
		if err != nil { // the node may be decommissioned
			r.Log.Warnf("Couldn't get operation mod for pod %s, checking if it is decommissioned already", decommissionPodName)
			notLiveView := 0
			for _, pod := range podList.Items {
				if pod.Name == decommissionPod.Name {
					continue
				}

				clusterView, nctlErr := nctl.ClusterView(broadcastAddresses[pod.Name])
				if nctlErr != nil {
					return errors.Wrap(nctlErr, "can't get cluster view")
				}

				if !util.Contains(clusterView.LiveNodes, broadcastIP) {
					notLiveView++
				}
			}

			r.Log.Debugf("%d nodes don't see node %s as live", notLiveView, decommissionPod.Name)
			if notLiveView >= len(podList.Items)-1 {
				*sts.Spec.Replicas = *sts.Spec.Replicas - 1
				r.Log.Infof("node %s/%s is decommissioned, scaling down the statefulset", decommissionPod.Namespace, decommissionPod.Name)
				updateErr := r.Update(ctx, sts)
				if updateErr != nil {
					return errors.Wrap(updateErr, "couldn't scale down the statefulset to remove the decommissioned node")
				}
				err = r.Jobs.RemoveJob(jobName)
				if err != nil {
					return errors.Wrap(err, "can't remove job")
				}

				return nil
			} else {
				return errors.Wrap(err, "failed to get operation mode, but some peer node(s) still see the node as ready")
			}
		}

		if opMode == nodectl.NodeOperationModeLeaving {
			r.Log.Infof("node %s/%s is being decommissioned. Waiting to finish",
				decommissionPod.Namespace, decommissionPod.Name)
			return nil
		}

		r.Log.Infof("starting decommision of node %s/%s", decommissionPod.Namespace, decommissionPod.Name)
		err = r.Jobs.Run(jobName, cc, func() error {
			decommissionErr := nctl.Decommission(broadcastIP)
			if decommissionErr != nil {
				r.Log.Errorf("failed to decommission pod, error: %s", decommissionErr.Error())
				return decommissionErr
			}

			return nil
		})
		if err != nil {
			return errors.Wrapf(err, "failed to start job to decommission pod %s", decommissionPodName)
		}

		return nil //return from the loop to scale down only one statefulset at a time
	}

	return nil
}
