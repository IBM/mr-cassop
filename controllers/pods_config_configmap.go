package controllers

import (
	"context"
	"fmt"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/prober"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
)

func (r *CassandraClusterReconciler) reconcileCassandraPodsConfigMap(ctx context.Context, cc *v1alpha1.CassandraCluster, proberClient prober.ProberClient) error {

	var cassandraIP string
	var cassandraSeeds []string

	podList := &v1.PodList{}
	cassandraClusterPodLabels := labels.ComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra)
	err := r.List(context.Background(), podList, client.InNamespace(cc.Namespace), client.MatchingLabels(cassandraClusterPodLabels))
	if err != nil {
		return errors.Wrap(err, "Cannot get Cassandra pods list")
	}

	nodeList := &v1.NodeList{}
	err = r.List(context.Background(), nodeList)
	if err != nil {
		return errors.Wrap(err, "Cannot get cluster nodes list")
	}

	cmData := make(map[string]string)

	for _, p := range podList.Items {
		nodeName := p.Spec.NodeName
		podName := p.Name
		podUID := p.UID
		entryName := podName + "_" + string(podUID) + ".env"

		// Find the node where the pod was started
		for _, node := range nodeList.Items {
			if node.Name == nodeName {

				if cc.Spec.Cassandra.ZonesAsRacks {
					cmData[entryName] += fmt.Sprintln("export CASSANDRA_RACK=" + node.Labels[v1.LabelTopologyZone])
					// CASSANDRA_ENDPOINT_SNITCH is required to tell Cassandra to read values from cassandra-rackdc.properties, otherwise C* ignores this file.
					cmData[entryName] += fmt.Sprintln("export CASSANDRA_ENDPOINT_SNITCH=GossipingPropertyFileSnitch")
				}

				if cc.Spec.HostPort.Enabled {
					if cc.Spec.HostPort.UseExternalHostIP {
						cassandraIP = util.GetNodeIP(v1.NodeExternalIP, node.Status.Addresses)
					} else {
						cassandraIP = util.GetNodeIP(v1.NodeInternalIP, node.Status.Addresses)
					}

					cassandraSeeds, err = proberClient.Seeds(ctx)
					if err != nil {
						return errors.Wrap(err, "Cannot obtain seed list from prober")
					}
				} else {
					cassandraIP = p.Status.PodIP
					cassandraSeeds = getSeedsList(cc)
				}

				if len(cassandraIP) == 0 {
					return errors.New("Cannot obtain cassandra ip")
				}

				// We need to hold the config entry creation until pod is Running and ip is assigned
				if len(p.Status.PodIP) == 0 {
					return errors.New("Cannot obtain pod ip")
				}

				cmData[entryName] += fmt.Sprintln("export CASSANDRA_IP=" + cassandraIP)
				cmData[entryName] += fmt.Sprintln("export CASSANDRA_BROADCAST_ADDRESS=" + cassandraIP)
				cmData[entryName] += fmt.Sprintln("export CASSANDRA_BROADCAST_RPC_ADDRESS=" + p.Status.PodIP)
			}
		}
	}

	// Add seeds list for each pod config
	for i := range cmData {
		cmData[i] += fmt.Sprintln("export CASSANDRA_SEEDS=" + strings.Join(cassandraSeeds, ","))
	}

	desiredCM := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.PodsConfigConfigmap(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra),
		},
		Data: cmData,
	}

	if err := controllerutil.SetControllerReference(cc, desiredCM, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}
	if err := r.reconcileConfigMap(ctx, desiredCM); err != nil {
		return err
	}
	return nil
}
