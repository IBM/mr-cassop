package controllers

import (
	"context"
	"sort"
	"strconv"

	"github.com/google/go-cmp/cmp"

	"github.com/ibm/cassandra-operator/controllers/names"
	"k8s.io/apimachinery/pkg/types"

	"github.com/ibm/cassandra-operator/controllers/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ibm/cassandra-operator/controllers/util"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/nodectl"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

func (r *CassandraClusterReconciler) reconcileCassandraScaling(ctx context.Context, cc *dbv1alpha1.CassandraCluster, podList *v1.PodList, nodesList *v1.NodeList, allDCs []dbv1alpha1.DC, adminRoleSecret *v1.Secret) (bool, error) {
	broadcastAddresses, err := getBroadcastAddresses(cc, podList.Items, nodesList.Items)
	if err != nil {
		return false, errors.Wrap(err, "can't get broadcast addresses")
	}

	stsList := &appsv1.StatefulSetList{}
	err = r.List(ctx, stsList, client.InNamespace(cc.Namespace), client.MatchingLabels(labels.Cassandra(cc)))
	if err != nil {
		return false, errors.Wrap(err, "can't get statefulsets")
	}

	if len(stsList.Items) == 0 {
		r.Log.Warn("couldn't find any statefulset to reconcile scaling")
		return false, nil
	}

	dcToReplicas := dcsMap(cc)
	desiredSTSNames, desiredSts := desiredDCsSTS(cc, stsList.Items)
	stsToDecommissionNames, stsToDecommission := decommissionDCsSTS(cc, stsList.Items)

	for _, stsName := range desiredSTSNames {
		sts := desiredSts[stsName]
		var oldReplicas int32 = 1
		var newReplicas int32 = 1
		if sts.Spec.Replicas != nil {
			oldReplicas = *sts.Spec.Replicas
		}

		newReplicas = dcToReplicas[sts.Labels[dbv1alpha1.CassandraClusterDC]]

		if oldReplicas == newReplicas { // no need for scaling
			continue
		}

		if oldReplicas < newReplicas { // scale up
			sts.Spec.Replicas = &newReplicas
			err = r.Update(ctx, &sts)
			if err != nil {
				return true, errors.Wrap(err, "can't update replicas number for statefulset")
			}

			continue
		}

		// scale down
		if len(podList.Items) == 0 {
			r.Log.Warn("No pods found to perform scaledown")
			continue
		}

		decommissionPodName := sts.Name + "-" + strconv.Itoa(int(oldReplicas)-1)
		r.Log.Debugf("handling decommission for node %s", decommissionPodName)
		err = r.handlePodDecommission(ctx, cc, sts, broadcastAddresses, decommissionPodName, podList)
		if err != nil {
			return false, errors.Wrap(err, "failed to handle pod decommission")
		}

		return true, nil //return from the loop to scale down only one statefulset at a time
	}

	if len(stsToDecommissionNames) == 0 {
		return false, nil
	}

	err = r.handleDCsDecommission(ctx, cc, stsToDecommissionNames, stsToDecommission, allDCs, adminRoleSecret, broadcastAddresses, podList)
	if err != nil {
		return false, errors.Wrap(err, "failed to decommission DCs")
	}

	return true, nil
}

func (r CassandraClusterReconciler) handleDCsDecommission(ctx context.Context, cc *dbv1alpha1.CassandraCluster, stsNames []string, stsList map[string]appsv1.StatefulSet, allDCs []dbv1alpha1.DC, adminRoleSecret *v1.Secret, broadcastAddresses map[string]string, podList *v1.PodList) error {
	r.Log.Infof("Decommissioning DCs %v", stsNames)

	keyspacesToReconcile := desiredKeyspacesToReconcile(cc)
	cassandraOperatorAdminRole, cassandraOperatorAdminPassword, err := extractCredentials(adminRoleSecret)
	if err != nil {
		return err
	}
	cqlClient, err := r.CqlClient(newCassandraConfig(cc, cassandraOperatorAdminRole, cassandraOperatorAdminPassword, r.Log))
	if err != nil {
		return errors.Wrap(err, "can't create cql client")
	}
	defer cqlClient.CloseSession()

	currentKeyspaces, err := cqlClient.GetKeyspacesInfo()
	if err != nil {
		return errors.Wrap(err, "can't get keyspace info")
	}

	for _, systemKeyspace := range keyspacesToReconcile {
		// system_auth should be reconciled only after scaledown is finished, otherwise will get quorum errors on JMX requests
		if systemKeyspace == keyspaceSystemAuth {
			continue
		}
		keyspaceInfo, found := getKeyspaceByName(currentKeyspaces, string(systemKeyspace))
		if !found {
			r.Log.Warnf("Keyspace %q doesn't exists", systemKeyspace)
			continue
		}

		desiredOptions := desiredReplicationOptions(cc, string(systemKeyspace), allDCs)
		if !cmp.Equal(keyspaceInfo.Replication, desiredOptions) {
			r.Log.Infof("Updating keyspace %q with replication options %v", systemKeyspace, desiredOptions)
			err = cqlClient.UpdateRF(string(systemKeyspace), desiredOptions)
			if err != nil {
				return errors.Wrapf(err, "failed to alter %q keyspace", systemKeyspace)
			}
		}
	}

	for _, stsName := range stsNames {
		sts := stsList[stsName]
		if sts.Spec.Replicas != nil && *sts.Spec.Replicas == 0 { //all pods for DC are decommissioned already
			err = r.removeDC(ctx, cc, sts)
			if err != nil {
				return errors.Wrapf(err, "failed to remove dc %s", sts.Labels[dbv1alpha1.CassandraClusterDC])
			}

			return nil
		}
		decommissionPodName := sts.Name + "-" + strconv.Itoa(int(*sts.Spec.Replicas)-1)
		err = r.handlePodDecommission(ctx, cc, sts, broadcastAddresses, decommissionPodName, podList)
		if err != nil {
			return errors.Wrap(err, "failed to handle pod decommission")
		}
	}

	return nil
}

func (r CassandraClusterReconciler) handlePodDecommission(ctx context.Context, cc *dbv1alpha1.CassandraCluster, sts appsv1.StatefulSet, broadcastAddresses map[string]string, decommissionPodName string, podList *v1.PodList) error {
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
	r.Log.Debugf("checking operation mode for node %s", decommissionPod.Name)
	opMode, err := nctl.OperationMode(ctx, broadcastIP)
	if err != nil { // the node may be decommissioned
		r.Log.Warnf("Couldn't get operation mod for pod %s, checking if it is decommissioned already. Err: %s", decommissionPodName, err.Error())

		decommissioned, err := r.podDecommissioned(ctx, cc, nctl, podList.Items, *decommissionPod, broadcastAddresses)
		if err != nil {
			return errors.Wrap(err, "failed to check if the pod is decommissioned")
		}
		if decommissioned {
			*sts.Spec.Replicas = *sts.Spec.Replicas - 1
			r.Log.Infof("node %s/%s is decommissioned, scaling down the statefulset", decommissionPod.Namespace, decommissionPod.Name)
			updateErr := r.Update(ctx, &sts)
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
		decommissionCtx := context.Background() //reconcile context may cancel the job sooner that needed
		decommissionErr := nctl.Decommission(decommissionCtx, broadcastIP)
		if decommissionErr != nil {
			r.Log.Errorf("failed to decommission pod, error: %s", decommissionErr.Error())
			return decommissionErr
		}

		return nil
	})
	if err != nil {
		return errors.Wrapf(err, "failed to start job to decommission pod %s", decommissionPodName)
	}

	return nil
}

func (r CassandraClusterReconciler) podDecommissioned(ctx context.Context, cc *dbv1alpha1.CassandraCluster, nctl nodectl.Nodectl, pods []v1.Pod, decommissionPod v1.Pod, broadcastAddresses map[string]string) (bool, error) {
	quorum := 0 //number of pods that need to agree that the pods is decommissioned
	for _, pod := range pods {
		podDC := pod.Labels[dbv1alpha1.CassandraClusterDC]
		for _, dc := range cc.Spec.DCs {
			if podDC == dc.Name {
				quorum++
				break
			}
		}
	}
	quorum-- //one of the pods can be in the process of decommission

	desiredDCs := dcsMap(cc)

	notLiveView := 0
	for _, pod := range pods {
		_, exists := desiredDCs[pod.Labels[dbv1alpha1.CassandraClusterDC]]

		// if it's the pod being decommissioned, or a pod from a DC being decommissioned
		if pod.Name == decommissionPod.Name || !exists {
			continue
		}

		r.Log.Debugf("checking cluster view for node %s", pod.Name)
		clusterView, nctlErr := nctl.ClusterView(ctx, broadcastAddresses[pod.Name])
		if nctlErr != nil {
			return false, errors.Wrap(nctlErr, "can't get cluster view")
		}

		if !util.Contains(clusterView.LiveNodes, broadcastAddresses[decommissionPod.Name]) {
			notLiveView++
		}
	}

	r.Log.Debugf("%d nodes don't see node %s as live", notLiveView, decommissionPod.Name)
	return notLiveView >= quorum, nil
}

func (r CassandraClusterReconciler) removeDC(ctx context.Context, cc *dbv1alpha1.CassandraCluster, sts appsv1.StatefulSet) error {
	dcName := sts.Labels[dbv1alpha1.CassandraClusterDC]

	reaperDeploy := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: names.ReaperDeployment(cc.Name, dcName), Namespace: cc.Namespace}, reaperDeploy)
	if err == nil {
		err = r.Delete(ctx, reaperDeploy)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	svc := &v1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: names.DCService(cc.Name, dcName), Namespace: cc.Namespace}, svc)
	if err == nil {
		err = r.Delete(ctx, svc)
		if err == nil {
			return errors.WithStack(err)
		}
	}

	err = r.Delete(ctx, &sts)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func dcsMap(cc *dbv1alpha1.CassandraCluster) map[string]int32 {
	dcToReplicas := make(map[string]int32)
	for _, dc := range cc.Spec.DCs {
		dcToReplicas[dc.Name] = *dc.Replicas
	}

	return dcToReplicas
}

func desiredDCsSTS(cc *dbv1alpha1.CassandraCluster, allSTS []appsv1.StatefulSet) (desiredSTSNames []string, desiredSTS map[string]appsv1.StatefulSet) {
	desiredSTS = make(map[string]appsv1.StatefulSet)
	for i, sts := range allSTS {
		for _, dc := range cc.Spec.DCs {
			if sts.Labels[dbv1alpha1.CassandraClusterDC] == dc.Name {
				desiredSTS[allSTS[i].Name] = allSTS[i]
				desiredSTSNames = append(desiredSTSNames, sts.Name)
				break
			}
		}
	}

	sort.Strings(desiredSTSNames)

	return desiredSTSNames, desiredSTS
}

func decommissionDCsSTS(cc *dbv1alpha1.CassandraCluster, allSTS []appsv1.StatefulSet) (stsToDecommissionNames []string, stsToDecommission map[string]appsv1.StatefulSet) {
	stsToDecommission = make(map[string]appsv1.StatefulSet)
	for i, sts := range allSTS {
		desiredSTS := false
		for _, dc := range cc.Spec.DCs {
			if sts.Labels[dbv1alpha1.CassandraClusterDC] == dc.Name {
				desiredSTS = true
				break
			}
		}
		if !desiredSTS {
			stsToDecommission[sts.Name] = allSTS[i]
			stsToDecommissionNames = append(stsToDecommissionNames, sts.Name)
		}
	}

	sort.Strings(stsToDecommissionNames)

	return stsToDecommissionNames, stsToDecommission
}
