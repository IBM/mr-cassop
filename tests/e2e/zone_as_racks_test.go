package e2e

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Cassandra cluster", func() {
	Context("When zones as racks configuration is enabled", func() {
		ccName := "zones-as-racks-enabled"
		AfterEach(func() {
			cleanupResources(ccName, cfg.operatorNamespace)
		})
		It("Should be enabled and worker zone value match rack name", func() {
			cc := newCassandraClusterTmpl(ccName, cfg.operatorNamespace)
			cc.Spec.Cassandra.ZonesAsRacks = true
			deployCassandraCluster(cc)

			By("Obtaining auth credentials from Secret")
			activeAdminSecret := &v1.Secret{}
			Expect(kubeClient.Get(ctx, types.NamespacedName{Name: names.ActiveAdminSecret(cc.Name), Namespace: cc.Namespace}, activeAdminSecret))

			By("Check zones as racks")
			podList := &v1.PodList{}
			Expect(kubeClient.List(ctx, podList, client.InNamespace(cc.Namespace), client.MatchingLabels(labels.Cassandra(cc)))).To(Succeed())

			nodeList := &v1.NodeList{}
			Expect(kubeClient.List(ctx, nodeList)).To(Succeed())

			cmd := []string{
				"sh",
				"-c",
				// We use `-Dcom.sun.jndi.rmiURLParsing=legacy` because of the bug with JDK 1.8.0_332 https://issues.apache.org/jira/browse/CASSANDRA-17581
				fmt.Sprintf("nodetool -Dcom.sun.jndi.rmiURLParsing=legacy -u %s -pw \"%s\" info | grep Rack | awk '{ print $3 }'", activeAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole], string(activeAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminPassword])),
			}

			for _, p := range podList.Items {
				r, err := execPod(p.Name, p.Namespace, cmd, "cassandra")
				Expect(err).ToNot(HaveOccurred())

				r.stdout = strings.TrimSuffix(r.stdout, "\n")
				r.stdout = strings.TrimSpace(r.stdout)
				Expect(len(r.stderr)).To(Equal(0))

				for _, n := range nodeList.Items {
					if n.Name == p.Spec.NodeName {
						nodeLabels := n.Labels
						Expect(r.stdout).To(Equal(nodeLabels["topology.kubernetes.io/zone"]))
					}
				}
			}
		})
	})
	Context("When zones as racks configuration is disabled", func() {
		ccName := "zones-as-racks-disabled"
		AfterEach(func() {
			cleanupResources(ccName, cfg.operatorNamespace)
		})
		It("Should be disabled and default rack name is set", func() {
			cc := newCassandraClusterTmpl(ccName, cfg.operatorNamespace)
			cc.Spec.Cassandra.ZonesAsRacks = false
			deployCassandraCluster(cc)

			By("Check default rack name...")
			podList := &v1.PodList{}
			Expect(kubeClient.List(ctx, podList, client.InNamespace(cc.Namespace), client.MatchingLabels(labels.Cassandra(cc)))).To(Succeed())

			cmd := []string{
				"sh",
				"-c",
				"cat /etc/cassandra/cassandra-rackdc.properties | grep rack= | cut -f2 -d'='",
			}

			for _, p := range podList.Items {
				r, err := execPod(p.Name, p.Namespace, cmd, "cassandra")
				Expect(err).ToNot(HaveOccurred())

				r.stdout = strings.TrimSuffix(r.stdout, "\n")
				r.stdout = strings.TrimSpace(r.stdout)
				Expect(len(r.stderr)).To(Equal(0))
				Expect(r.stdout).To(Equal("rack1"))
			}
		})
	})
})
