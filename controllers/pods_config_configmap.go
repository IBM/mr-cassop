package controllers

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/prober"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CassandraClusterReconciler) reconcileCassandraPodsConfigMap(ctx context.Context, cc *v1alpha1.CassandraCluster, proberClient prober.ProberClient) error {
	desiredCM, err := r.createConfigIfNotExists(ctx, cc)
	if err != nil {
		return err
	}

	podList, err := r.getCassandraPods(ctx, cc)
	if err != nil {
		return errors.Wrap(err, "Cannot get Cassandra pods list")
	}

	cmData := make(map[string]string)
	broadcastAddresses := make(map[string]string)

	for _, pod := range podList.Items {
		entryName := pod.Name + "_" + string(pod.UID) + ".sh"

		// Hold the config entry creation until pod is Running and ip is assigned
		if len(pod.Status.PodIP) == 0 {
			return errors.New("Cannot obtain pod ip")
		}

		// Get the node where the pod was started
		nodeName := pod.Spec.NodeName
		node := &v1.Node{}
		if r.Get(ctx, client.ObjectKey{Name: nodeName}, node) != nil {
			return errors.Wrap(err, "Cannot get node: "+nodeName)
		}

		if cc.Spec.Cassandra.ZonesAsRacks {
			cmData[entryName] += fmt.Sprintln("export CASSANDRA_RACK=" + node.Labels[v1.LabelTopologyZone])
			// GossipingPropertyFileSnitch: rack and datacenter for the local node are defined in cassandra-rackdc.properties.
			cmData[entryName] += fmt.Sprintln("export CASSANDRA_ENDPOINT_SNITCH=GossipingPropertyFileSnitch")
		}

		broadcastAddress := getBroadcastAddress(cc, node, pod)
		if len(broadcastAddress) == 0 {
			return errors.New("Cannot obtain cassandra broadcast address")
		}
		broadcastAddresses[pod.Name] = broadcastAddress

		cmData[entryName] += fmt.Sprintln("export CASSANDRA_BROADCAST_ADDRESS=" + broadcastAddress)
		cmData[entryName] += fmt.Sprintln("export CASSANDRA_BROADCAST_RPC_ADDRESS=" + pod.Status.PodIP)
	}

	if !cmp.Equal(broadcastAddresses, cc.Status.NodesBroadcastAddresses) {
		cc.Status.NodesBroadcastAddresses = broadcastAddresses
		if err := r.Status().Update(ctx, cc); err != nil {
			return errors.Wrap(err, "Error updating CR Status")
		}
	}

	cassandraSeeds := getLocalSeedsHostnames(cc)
	if proberClient.UpdateSeeds(ctx, cassandraSeeds) != nil {
		return errors.Wrap(err, "Prober request to update local seeds failed.")
	}
	if cc.Spec.HostPort.Enabled {
		// GET /localseeds of external DCs
		cassandraSeeds = []string{} // clear array to prevent duplicate local seeds
		for _, ingressDomain := range cc.Spec.Prober.DCsIngressDomains {
			seeds, err := proberClient.GetSeeds(ctx, names.ProberIngressDomain(cc.Name, ingressDomain, cc.Namespace))
			if err != nil {
				r.Log.Warnw("Failed Request to DC's ingress", "ingressDomain", ingressDomain, "error", err)
				continue
			}
			cassandraSeeds = append(cassandraSeeds, seeds...)
		}
	}

	// Add seeds list for each pod config
	for i := range cmData {
		cmData[i] += fmt.Sprintln("export CASSANDRA_SEEDS=" + strings.Join(cassandraSeeds, ","))
	}

	desiredCM.Data = cmData
	if err := controllerutil.SetControllerReference(cc, desiredCM, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}
	return r.reconcileConfigMap(ctx, desiredCM)
}

func (r *CassandraClusterReconciler) createConfigIfNotExists(ctx context.Context, cc *v1alpha1.CassandraCluster) (*v1.ConfigMap, error) {
	desiredCM := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.PodsConfigConfigmap(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra),
		},
	}
	actualCM := &v1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: desiredCM.Name, Namespace: desiredCM.Namespace}, actualCM)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Infof("Creating %s", desiredCM.Name)
		if err = r.Create(ctx, desiredCM); err != nil {
			return nil, errors.Wrapf(err, "Unable to create %s", desiredCM.Name)
		}
	}
	return desiredCM, nil
}

func getBroadcastAddress(cc *v1alpha1.CassandraCluster, node *v1.Node, pod v1.Pod) string {
	var broadcastAddress string
	if cc.Spec.HostPort.Enabled {
		if cc.Spec.HostPort.UseExternalHostIP {
			broadcastAddress = util.GetNodeIP(v1.NodeExternalIP, node.Status.Addresses)
		} else {
			broadcastAddress = util.GetNodeIP(v1.NodeInternalIP, node.Status.Addresses)
		}
	} else {
		broadcastAddress = pod.Status.PodIP
	}
	return broadcastAddress
}
