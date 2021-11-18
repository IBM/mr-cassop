package e2e

import (
	"context"
	"strings"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Cassandra cluster", func() {
	Context("When hostPort enabled and UseExternalHostIP set to false", func() {
		It("Should be enabled and internal node ip should match cassandra ip", func() {
			testBroadcastAddress(false)
		})
	})
	Context("When hostPort enabled and UseExternalHostIP set to true", func() {
		It("Should be enabled and external node ip should match broadcast ip", func() {
			testBroadcastAddress(true)
		})
	})
})

func newCassandraCluster(useExternalHostIP bool) *v1alpha1.CassandraCluster {
	newCC := cassandraCluster.DeepCopy()
	newCC.Spec.HostPort.Enabled = true
	newCC.Spec.HostPort.UseExternalHostIP = useExternalHostIP
	newCC.Spec.Ingress.Domain = ingressDomain
	newCC.Spec.Ingress.Secret = ingressSecret
	newCC.Spec.HostPort.Ports = []string{"intra", "cql", "jmx", "thrift"}
	return newCC
}

func execOnPod(podName string, cmd []string) string {
	r, err := execPod(podName, cassandraNamespace, cmd)
	Expect(err).ToNot(HaveOccurred())
	r.stdout = strings.TrimSuffix(r.stdout, "\n")
	r.stdout = strings.TrimSpace(r.stdout)
	Expect(len(r.stderr)).To(Equal(0))

	return r.stdout
}

func checkBroadcastAddressOnAllPods(podList *v1.PodList, nodeList *v1.NodeList, addressType v1.NodeAddressType, cmd []string) {
	for _, pod := range podList.Items {
		podName := pod.Name
		nodeName := pod.Spec.NodeName

		var nodeFound = false
		for _, node := range nodeList.Items {
			if node.Name == nodeName {
				cassandraIP := util.GetNodeIP(addressType, node.Status.Addresses)
				Expect(execOnPod(podName, cmd)).To(Equal(cassandraIP))
				nodeFound = true
			}
		}
		Expect(nodeFound).To(BeTrue())
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
	Expect(err).ToNot(HaveOccurred())

	nodeList := &v1.NodeList{}
	err = restClient.List(context.Background(), nodeList)
	Expect(err).ToNot(HaveOccurred())

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
		//	"source /etc/pods-config/${POD_NAME}_${POD_UID}.sh && echo $CASSANDRA_BROADCAST_ADDRESS",
		//},
	}

	for _, cmd := range cmds {
		checkBroadcastAddressOnAllPods(podList, nodeList, addressType, cmd)
	}
}
