package e2e

import (
	"context"
	"fmt"
	"strings"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func newCassandraCluster(useExternalHostIP bool) *v1alpha1.CassandraCluster {
	newCassandraCluster := cassandraCluster.DeepCopy()
	newCassandraCluster.Spec.HostPort.Enabled = true
	newCassandraCluster.Spec.HostPort.UseExternalHostIP = useExternalHostIP
	newCassandraCluster.Spec.Prober.Ingress.Domain = ingressDomain
	newCassandraCluster.Spec.Prober.Ingress.Secret = ingressSecret
	newCassandraCluster.Spec.Prober.DCsIngressDomains = []string{
		ingressDomain,
		"stub.us-south.containers.appdomain.cloud",
	}
	newCassandraCluster.Spec.HostPort.Ports = []string{"intra", "cql", "jmx", "thrift"}
	return newCassandraCluster
}

func execOnPod(podName string, cmd []string) string {
	r := execPod(podName, cassandraNamespace, cmd)

	r.stdout = strings.TrimSuffix(r.stdout, "\n")
	r.stdout = strings.TrimSpace(r.stdout)

	if len(r.stderr) != 0 {
		Fail(fmt.Sprintf("Error occurred: %s", r.stderr))
	}

	return r.stdout
}

func checkBroadcastAddressOnAllPods(podList *v1.PodList, nodeList *v1.NodeList, addressType v1.NodeAddressType, cmd []string) {
	for _, pod := range podList.Items {
		podName := pod.Name
		nodeName := pod.Spec.NodeName

		var nodeNotFound = true
		for _, node := range nodeList.Items {
			if node.Name == nodeName {
				cassandraIP := util.GetNodeIP(addressType, node.Status.Addresses)
				Expect(execOnPod(podName, cmd)).To(Equal(cassandraIP))
				nodeNotFound = false
			}
		}
		if nodeNotFound {
			Fail(fmt.Sprintf("Matching node not found for pod.Spec.NodeName: %s", nodeName))
		}
	}
}

func testBroadcastAddress(useExternalHostIP bool) {
	var addressType = v1.NodeInternalIP
	if useExternalHostIP {
		addressType = v1.NodeExternalIP
	}

	deployCassandraCluster(newCassandraCluster(useExternalHostIP))

	By("Check hostPort")
	podList := &v1.PodList{}
	err = restClient.List(context.Background(), podList, client.InNamespace(cassandraNamespace), client.MatchingLabels(cassandraClusterPodLabels))
	if err != nil {
		Fail(fmt.Sprintf("Error occured: %s", err))
	}

	nodeList := &v1.NodeList{}
	err = restClient.List(context.Background(), nodeList)
	if err != nil {
		Fail(fmt.Sprintf("Error occured: %s", err))
	}

	cmds := [][]string{
		{
			"sh",
			"-c",
			"cat /etc/cassandra/cassandra.yaml | grep 'broadcast_address:' | cut -f2 -d':'",
		},
		// Todo: try to add test cmd below later. Currently it produces: Error occurred: command terminated with exit code 127
		//{
		//	"sh",
		//	"-c",
		//	"source /etc/pods-config/${POD_NAME}_${POD_UID}.env && echo $CASSANDRA_BROADCAST_ADDRESS",
		//},
	}

	for _, cmd := range cmds {
		checkBroadcastAddressOnAllPods(podList, nodeList, addressType, cmd)
	}
}

var _ = Describe("Cassandra cluster", func() {
	Context("When hostPort enabled and UseExternalHostIP set to false", func() {
		It("Should be enabled and internal node ip should match cassandra ip", func() {
			testBroadcastAddress(false)
		})
	})
	// Disable test while UseExternalHostIP: true isn't implemented in prober
	//Context("When hostPort enabled and UseExternalHostIP set to true", func() {
	//	It("Should be enabled and external node ip should match cassandra ip", func() {
	//		testBroadcastAddress(true)
	//	})
	//})
})
