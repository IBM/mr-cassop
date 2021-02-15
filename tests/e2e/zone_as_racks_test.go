package e2e

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"

	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

var _ = Describe("Cassandra cluster", func() {
	Context("When zones as racks configuration is enabled", func() {
		It("Should be enabled and worker zone value match rack name", func() {
			newCassandraCluster := cassandraCluster.DeepCopy()
			newCassandraCluster.Spec.Cassandra.ZonesAsRacks = true

			deployCassandraCluster(newCassandraCluster)

			By("Check zones as racks")
			podList := &v1.PodList{}
			err = restClient.List(context.Background(), podList, client.InNamespace(cassandraNamespace), client.MatchingLabels(cassandraClusterPodLabels))
			if err != nil {
				Fail(fmt.Sprintf("Error occurred: %s", err))
			}

			nodeList := &v1.NodeList{}
			err = restClient.List(context.Background(), nodeList)
			if err != nil {
				Fail(fmt.Sprintf("Error occurred: %s", err))
			}

			cmd := []string{
				"sh",
				"-c",
				"nodetool info | grep Rack | awk '{ print $3 }'",
			}

			for _, p := range podList.Items {
				podName := p.Name
				nodeName := p.Spec.NodeName
				r := execPod(podName, cassandraNamespace, cmd)
				if len(r.stderr) != 0 {
					Fail(fmt.Sprintf("Error occurred: %s", r.stderr))
				}

				r.stdout = strings.TrimSuffix(r.stdout, "\n")
				r.stdout = strings.TrimSpace(r.stdout)

				for _, n := range nodeList.Items {
					if n.Name == nodeName {
						nodeLabels := n.Labels
						Expect(r.stdout).To(Equal(nodeLabels["topology.kubernetes.io/zone"]))
					}
				}
			}
		})
	})
})
