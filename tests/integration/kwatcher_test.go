package integration

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/names"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"time"
)

var _ = Describe("kwatcher deployment", func() {
	cc := &v1alpha1.CassandraCluster{
		ObjectMeta: cassandraObjectMeta,
		Spec: v1alpha1.CassandraClusterSpec{
			DCs: []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(3),
				},
			},
			ImagePullSecretName: "pullSecretName",
		},
	}

	Context("when cassandracluster created with only required values", func() {
		It("should be created with defaulted values", func() {
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())

			mockProberClient.err = nil
			mockProberClient.readyAllDCs = true
			mockProberClient.ready = true
			mockNodetoolClient.err = nil
			mockCQLClient.err = nil
			mockCQLClient.cassandraUsers = []cql.CassandraUser{{Role: "cassandra", IsSuperuser: true}}
			mockCQLClient.keyspaces = []cql.Keyspace{{
				Name: "system_auth",
				Replication: map[string]string{
					"class": "org.apache.cassandra.locator.SimpleTopologyStrategy",
				},
			}}

			deployment := &appsv1.Deployment{}

			for _, dc := range cc.Spec.DCs {
				kwatcherLabels := map[string]string{
					"cassandra-cluster-component": "kwatcher",
					"cassandra-cluster-instance":  "test-cassandra-cluster",
					"cassandra-cluster-dc":        dc.Name,
					"datacenter":                  dc.Name,
				}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: names.KwatcherDeployment(cc, dc.Name), Namespace: cc.Namespace}, deployment)
				}, time.Second*5, time.Millisecond*100).Should(Succeed())

				Expect(deployment.Labels).To(BeEquivalentTo(kwatcherLabels))
				Expect(deployment.Spec.Replicas).To(Equal(proto.Int32(1)))
				Expect(deployment.Spec.Selector.MatchLabels).To(BeEquivalentTo(kwatcherLabels))
				Expect(deployment.Spec.Template.Labels).To(Equal(kwatcherLabels))
				Expect(deployment.OwnerReferences[0].UID).To(Equal(cc.UID))
				Expect(deployment.OwnerReferences[0].Controller).To(Equal(proto.Bool(true)))
				Expect(deployment.OwnerReferences[0].Kind).To(Equal("CassandraCluster"))
				Expect(deployment.OwnerReferences[0].APIVersion).To(Equal("db.ibm.com/v1alpha1"))
				Expect(deployment.OwnerReferences[0].Name).To(Equal(cc.Name))
				Expect(deployment.OwnerReferences[0].BlockOwnerDeletion).To(Equal(proto.Bool(true)))
				proberContainer, found := getContainerByName(deployment.Spec.Template.Spec, "kwatcher")
				Expect(found).To(BeTrue())
				Expect(proberContainer.Image).To(Equal(operatorConfig.DefaultKwatcherImage), "default values")
				Expect(proberContainer.ImagePullPolicy).To(Equal(v1.PullIfNotPresent), "default values")
			}
		})
	})
})
