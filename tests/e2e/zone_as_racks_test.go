package e2e

import (
	"context"
	"fmt"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

var _ = Describe("Cassandra cluster", func() {

	Context("When zones as racks configuration is enabled", func() {
		It("Should be enabled and worker zone value match rack name", func() {
			newCassandraCluster := cassandraCluster.DeepCopy()
			newCassandraCluster.Spec.Cassandra.ZonesAsRacks = true

			deployCassandraCluster(newCassandraCluster)

			By("Checking reaper pods readiness...")
			for _, dc := range newCassandraCluster.Spec.DCs {
				waitForPodsReadiness(cassandraNamespace, labels.WithDCLabel(reaperPodLabels, dc.Name))
			}

			By("Obtaining auth credentials from Secret")
			activeAdminSecret, err := kubeClient.CoreV1().Secrets(cassandraNamespace).Get(context.Background(), names.ActiveAdminSecret(newCassandraCluster.Name), metav1.GetOptions{})
			if err != nil {
				Fail(fmt.Sprintf("Cannot get Secret : %s", names.ActiveAdminSecret(newCassandraCluster.Name)))
			}

			By("Check zones as racks")
			podList := &v1.PodList{}
			err = restClient.List(context.Background(), podList, client.InNamespace(cassandraNamespace), client.MatchingLabels(cassandraClusterPodLabels))
			Expect(err).ToNot(HaveOccurred())

			nodeList := &v1.NodeList{}
			err = restClient.List(context.Background(), nodeList)
			Expect(err).ToNot(HaveOccurred())

			cmd := []string{
				"sh",
				"-c",
				fmt.Sprintf("nodetool -u %s -pw \"%s\" info | grep Rack | awk '{ print $3 }'", dbv1alpha1.CassandraOperatorAdminRole, string(activeAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole])),
			}

			for _, p := range podList.Items {
				podName := p.Name
				nodeName := p.Spec.NodeName

				r, err := execPod(podName, cassandraNamespace, cmd)
				Expect(err).ToNot(HaveOccurred())

				r.stdout = strings.TrimSuffix(r.stdout, "\n")
				r.stdout = strings.TrimSpace(r.stdout)
				Expect(len(r.stderr)).To(Equal(0))

				for _, n := range nodeList.Items {
					if n.Name == nodeName {
						nodeLabels := n.Labels
						Expect(r.stdout).To(Equal(nodeLabels["topology.kubernetes.io/zone"]))
					}
				}
			}
		})
	})
	Context("When zones as racks configuration is disabled", func() {
		It("Should be disabled and default rack name is set", func() {
			newCassandraCluster := cassandraCluster.DeepCopy()
			newCassandraCluster.Spec.Cassandra.ZonesAsRacks = false

			deployCassandraCluster(newCassandraCluster)

			By("Check default rack name...")
			podList := &v1.PodList{}
			err = restClient.List(context.Background(), podList, client.InNamespace(cassandraNamespace), client.MatchingLabels(cassandraClusterPodLabels))
			Expect(err).ToNot(HaveOccurred())

			cmd := []string{
				"sh",
				"-c",
				"cat /etc/cassandra/cassandra-rackdc.properties | grep rack= | cut -f2 -d'='",
			}

			for _, p := range podList.Items {
				podName := p.Name
				r, err := execPod(podName, cassandraNamespace, cmd)
				Expect(err).ToNot(HaveOccurred())

				r.stdout = strings.TrimSuffix(r.stdout, "\n")
				r.stdout = strings.TrimSpace(r.stdout)
				Expect(len(r.stderr)).To(Equal(0))
				Expect(r.stdout).To(Equal("rack1"))
			}
		})
	})
})
