package controllers

import (
	"context"
	"fmt"
	"github.com/ibm/cassandra-operator/controllers/prober"
	"github.com/ibm/cassandra-operator/controllers/util"
	"sort"
	"strings"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/pkg/errors"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var (
	ErrPodNotScheduled = errors.New("One of pods is not scheduled yet")
	ErrRegionNotReady  = errors.New("One of the regions is not ready")
)

func (r *CassandraClusterReconciler) reconcileCassandraPodsConfigMap(ctx context.Context, cc *v1alpha1.CassandraCluster, proberClient prober.ProberClient) error {
	desiredCM := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.PodsConfigConfigmap(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra),
		},
	}
	if err := controllerutil.SetControllerReference(cc, desiredCM, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	podsConfigMapData, err := r.podsConfigMapData(ctx, cc, proberClient)
	if err != nil {
		return err
	}

	desiredCM.Data = podsConfigMapData
	return r.reconcileConfigMap(ctx, desiredCM)
}

func (r *CassandraClusterReconciler) podsConfigMapData(ctx context.Context, cc *v1alpha1.CassandraCluster, proberClient prober.ProberClient) (map[string]string, error) {
	podList, err := r.getCassandraPods(ctx, cc)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot get Cassandra pods list")
	}

	if len(podList.Items) == 0 {
		return nil, nil // the statefulset may not be created yet
	}

	broadcastAddresses, err := r.getBroadcastAddresses(ctx, cc)
	if err != nil {
		return nil, errors.Wrap(err, "error getting broadcast addresses")
	}

	seedsList, err := r.getSeedsList(ctx, cc, broadcastAddresses, proberClient)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get seeds list")
	}

	currentRegionPaused, nextDCToInit, err := r.getInitOrderInfo(ctx, cc, proberClient)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get init order info")
	}

	cmData := make(map[string]string)
	for _, pod := range podList.Items {
		entryName := pod.Name + "_" + string(pod.UID) + ".sh"

		// Hold the config entry creation until pod is Running and ip is assigned
		if len(pod.Status.PodIP) == 0 {
			return nil, ErrPodNotScheduled
		}

		if cc.Spec.Cassandra.ZonesAsRacks {
			// Get the node where the pod was started
			nodeName := pod.Spec.NodeName
			node := &v1.Node{}
			if err := r.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
				return nil, errors.Wrap(err, "Cannot get node: "+nodeName)
			}
			cmData[entryName] += fmt.Sprintln("export CASSANDRA_RACK=" + node.Labels[v1.LabelTopologyZone])
			// GossipingPropertyFileSnitch: rack and datacenter for the local node are defined in cassandra-rackdc.properties.
			cmData[entryName] += fmt.Sprintln("export CASSANDRA_ENDPOINT_SNITCH=GossipingPropertyFileSnitch")
		}

		broadcastAddress := broadcastAddresses[pod.Name]
		if len(broadcastAddress) == 0 {
			return nil, errors.Wrap(err, "Cannot get node broadcast address: "+pod.Name)
		}
		cmData[entryName] += fmt.Sprintln("export CASSANDRA_BROADCAST_ADDRESS=" + broadcastAddress)
		cmData[entryName] += fmt.Sprintln("export CASSANDRA_BROADCAST_RPC_ADDRESS=" + pod.Status.PodIP)
		cmData[entryName] += fmt.Sprintln("export CASSANDRA_SEEDS=" + strings.Join(seedsList, ","))

		pausePodInit := false
		if currentRegionPaused { // should pause all pods if the region is on pause
			pausePodInit = true
		} else if nextDCToInit == "" { // should not pause any pod if all DCs are ready
			pausePodInit = false
		} else { // start pod if it's in the next selected DC
			pausePodInit = nextDCToInit != pod.Labels[v1alpha1.CassandraClusterDC]
			if !dcSeedPodsReady(podList.Items, nextDCToInit) && !isSeedPod(pod) {
				pausePodInit = true
			}
		}

		cmData[entryName] += fmt.Sprintln("export PAUSE_INIT=" + fmt.Sprint(pausePodInit))
	}

	return cmData, nil
}

func dcSeedPodsReady(pods []v1.Pod, dc string) bool {
	if len(pods) == 0 {
		return false
	}
	for _, pod := range pods {
		if pod.Labels[v1alpha1.CassandraClusterDC] == dc && isSeedPod(pod) {
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if !containerStatus.Ready {
					return false
				}
			}
		}
	}

	return true
}

func (r *CassandraClusterReconciler) getInitOrderInfo(ctx context.Context, cc *v1alpha1.CassandraCluster, proberClient prober.ProberClient) (currentRegionPaused bool, nextLocalDCToInit string, err error) {
	unreadyLocalDCs, err := r.unreadyDCs(ctx, cc)
	if err != nil {
		return false, "", errors.Wrap(err, "Can't get not ready DCs info")
	}
	nextLocalDCToInit = getNextLocalDCToInit(cc, unreadyLocalDCs)

	if !cc.Spec.HostPort.Enabled {
		return false, nextLocalDCToInit, nil
	}

	if len(cc.Spec.Prober.ExternalDCsIngressDomains) == 0 {
		return false, "", nil
	}

	clusterRegionsStatuses, err := r.getRemoteClusterRegionsStatuses(ctx, cc, proberClient)
	if err != nil {
		return false, "", err
	}

	currentRegionHost := names.ProberIngressDomain(cc.Name, cc.Spec.Prober.Ingress.Domain, cc.Namespace)
	clusterRegionsStatuses[currentRegionHost] = len(unreadyLocalDCs) == 0

	nextRegionToInit := getNextRegionToInit(cc, clusterRegionsStatuses)
	if nextRegionToInit == "" { //all regions are ready
		r.Log.Debug("All regions are ready")
		return false, "", nil
	}

	if clusterRegionsStatuses[currentRegionHost] { //region is ready
		r.Log.Debugf("Current region (%q) is initialized", currentRegionHost)
		return false, "", nil
	}

	if nextRegionToInit != currentRegionHost {
		r.Log.Infof("Current region is on pause. Waiting for region %q to be ready", nextRegionToInit)
		return true, "", nil
	}

	nextLocalDCToInit = getNextLocalDCToInit(cc, unreadyLocalDCs)
	r.Log.Infof("Current region (%q) is initializing. DC %q is initializing", currentRegionHost, nextLocalDCToInit)
	return false, nextLocalDCToInit, nil
}

func (r *CassandraClusterReconciler) getRemoteClusterRegionsStatuses(ctx context.Context, cc *v1alpha1.CassandraCluster, proberClient prober.ProberClient) (clusterRegionsStatuses map[string]bool, err error) {
	clusterRegionsStatuses = make(map[string]bool)
	for _, domain := range cc.Spec.Prober.ExternalDCsIngressDomains {
		proberHost := names.ProberIngressDomain(cc.Name, domain, cc.Namespace)
		regionsReady, err := proberClient.DCsReady(ctx, proberHost)
		if err != nil {
			r.Log.Warnf(fmt.Sprintf("Unable to get DC status from prober %q. Err: %#v", proberHost, err))
			return nil, ErrRegionNotReady
		}
		clusterRegionsStatuses[proberHost] = regionsReady
	}

	return clusterRegionsStatuses, nil
}

func getAllRegionsHosts(cc *v1alpha1.CassandraCluster) []string {
	allRegionsHosts := make([]string, 0, len(cc.Spec.Prober.ExternalDCsIngressDomains)+1)
	for _, domain := range cc.Spec.Prober.ExternalDCsIngressDomains {
		allRegionsHosts = append(allRegionsHosts, names.ProberIngressDomain(cc.Name, domain, cc.Namespace))
	}
	currentRegionHost := names.ProberIngressDomain(cc.Name, cc.Spec.Prober.Ingress.Domain, cc.Namespace)
	allRegionsHosts = append(allRegionsHosts, currentRegionHost)
	sort.Strings(allRegionsHosts)
	return allRegionsHosts
}

func getNextLocalDCToInit(cc *v1alpha1.CassandraCluster, unreadyLocalDCs []string) string {
	for _, dc := range cc.Spec.DCs {
		if util.Contains(unreadyLocalDCs, dc.Name) {
			return dc.Name
		}
	}

	return ""
}

func getNextRegionToInit(cc *v1alpha1.CassandraCluster, clusterDCsStatuses map[string]bool) string {
	allRegionsHosts := getAllRegionsHosts(cc)
	for _, region := range allRegionsHosts {
		if !clusterDCsStatuses[region] {
			return region
		}
	}
	return ""
}

func (r *CassandraClusterReconciler) getSeedsList(ctx context.Context, cc *v1alpha1.CassandraCluster, broadcastAddresses map[string]string, proberClient prober.ProberClient) ([]string, error) {
	cassandraSeeds := getLocalSeedsHostnames(cc, broadcastAddresses)

	if err := proberClient.UpdateSeeds(ctx, cassandraSeeds); err != nil {
		return nil, errors.Wrap(err, "Prober request to update local seeds failed.")
	}
	if cc.Spec.HostPort.Enabled {
		// GET /localseeds of external DCs
		for _, ingressDomain := range cc.Spec.Prober.ExternalDCsIngressDomains {
			seeds, err := proberClient.GetSeeds(ctx, names.ProberIngressDomain(cc.Name, ingressDomain, cc.Namespace))
			if err != nil {
				r.Log.Warnw("Failed Request to DC's ingress", "ingressDomain", ingressDomain, "error", err)
				continue
			}
			cassandraSeeds = append(cassandraSeeds, seeds...)
		}
	}

	return cassandraSeeds, nil
}

func (r *CassandraClusterReconciler) getBroadcastAddresses(ctx context.Context, cc *v1alpha1.CassandraCluster) (map[string]string, error) {
	podList, err := r.getCassandraPods(ctx, cc)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get Cassandra pods list")
	}
	broadcastAddresses := make(map[string]string)

	for _, pod := range podList.Items {
		addr, err := r.getPodBroadcastAddress(ctx, cc, pod)
		if err != nil || len(addr) == 0 { // Error until pod is Running and ip is assigned
			return nil, errors.Wrap(err, "cannot get pod's broadcast address")
		}
		broadcastAddresses[pod.Name] = addr
	}

	return broadcastAddresses, nil
}

func (r *CassandraClusterReconciler) getPodBroadcastAddress(ctx context.Context, cc *v1alpha1.CassandraCluster, pod v1.Pod) (string, error) {
	if !cc.Spec.HostPort.Enabled {
		return pod.Status.PodIP, nil
	}

	// Get the node where the pod was started
	nodeName := pod.Spec.NodeName
	if nodeName == "" {
		return "", ErrPodNotScheduled
	}
	node := &v1.Node{}
	if err := r.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
		return "", errors.Wrap(err, "Cannot get node: "+nodeName)
	}

	if cc.Spec.HostPort.UseExternalHostIP {
		return util.GetNodeIP(v1.NodeExternalIP, node.Status.Addresses), nil
	}

	return util.GetNodeIP(v1.NodeInternalIP, node.Status.Addresses), nil
}
