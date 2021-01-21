package integration

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("prober deployment", func() {
	cc := &v1alpha1.CassandraCluster{
		ObjectMeta: cassandraObjectMeta,
		Spec: v1alpha1.CassandraClusterSpec{
			DCs: []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(3),
				},
			},
			ImagePullSecretName: "imagePullSecret",
		},
	}

	Context("when cassandracluster created with only required values", func() {
		It("should be created with defaulted values", func() {
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())
			initializeReadyCluster()

			deployment := &appsv1.Deployment{}
			proberLabels := map[string]string{
				"cassandra-cluster-component": "prober",
				"cassandra-cluster-instance":  "test-cassandra-cluster",
			}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: names.ProberDeployment(cc), Namespace: cc.Namespace}, deployment)
			}, mediumTimeout, mediumRetry).Should(Succeed())

			Expect(deployment.Labels).To(BeEquivalentTo(proberLabels))
			Expect(deployment.Spec.Replicas).To(Equal(proto.Int32(1)))
			Expect(deployment.Spec.Selector.MatchLabels).To(BeEquivalentTo(proberLabels))
			Expect(deployment.Spec.Template.Labels).To(Equal(proberLabels))
			Expect(deployment.OwnerReferences[0].Controller).To(Equal(proto.Bool(true)))
			Expect(deployment.OwnerReferences[0].Kind).To(Equal("CassandraCluster"))
			Expect(deployment.OwnerReferences[0].APIVersion).To(Equal("db.ibm.com/v1alpha1"))
			Expect(deployment.OwnerReferences[0].Name).To(Equal(cc.Name))
			Expect(deployment.OwnerReferences[0].BlockOwnerDeletion).To(Equal(proto.Bool(true)))
			proberContainer, found := getContainerByName(deployment.Spec.Template.Spec, "prober")
			Expect(found).To(BeTrue())
			Expect(proberContainer.Image).To(Equal(operatorConfig.DefaultProberImage), "default values")
			Expect(proberContainer.ImagePullPolicy).To(Equal(v1.PullIfNotPresent), "default values")
			jolokiaContainer, found := getContainerByName(deployment.Spec.Template.Spec, "jolokia")
			Expect(found).To(BeTrue())
			Expect(jolokiaContainer.Image).To(Equal(operatorConfig.DefaultJolokiaImage), "default values")
			Expect(jolokiaContainer.ImagePullPolicy).To(Equal(v1.PullIfNotPresent), "default values")
		})
	})
})
