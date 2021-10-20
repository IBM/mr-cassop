package integration

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("cassandra cluster seed pods", func() {
	cc := &v1alpha1.CassandraCluster{
		ObjectMeta: cassandraObjectMeta,
		Spec: v1alpha1.CassandraClusterSpec{
			DCs: []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(6),
				},
				{
					Name:     "dc2",
					Replicas: proto.Int32(6),
				},
			},
			AdminRoleSecretName: "admin-role",
			ImagePullSecretName: "pullSecretName",
		},
	}

	It("should have seed label set", func() {
		createReadyCluster(cc)

		By("Default seed number settings")
		expectSeedPodsLabelsBeSet(cc, []string{
			"test-cassandra-cluster-cassandra-dc1-0",
			"test-cassandra-cluster-cassandra-dc1-1",
			"test-cassandra-cluster-cassandra-dc2-0",
			"test-cassandra-cluster-cassandra-dc2-1",
		})

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: cc.Name}, cc)).To(Succeed())
		cc.Spec.Cassandra = &v1alpha1.Cassandra{
			NumSeeds: 4,
		}
		Expect(k8sClient.Update(ctx, cc)).To(Succeed())

		By("Increased seed pods number should add seep pod label to more pods")
		expectSeedPodsLabelsBeSet(cc, []string{
			"test-cassandra-cluster-cassandra-dc1-0",
			"test-cassandra-cluster-cassandra-dc1-1",
			"test-cassandra-cluster-cassandra-dc1-2",
			"test-cassandra-cluster-cassandra-dc1-3",
			"test-cassandra-cluster-cassandra-dc2-0",
			"test-cassandra-cluster-cassandra-dc2-1",
			"test-cassandra-cluster-cassandra-dc2-2",
			"test-cassandra-cluster-cassandra-dc2-3",
		})

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: cc.Name}, cc)).To(Succeed())
		cc.Spec.Cassandra = &v1alpha1.Cassandra{
			NumSeeds: 3,
		}
		Expect(k8sClient.Update(ctx, cc)).To(Succeed())

		By("Decreased seed pods number should remove seed pod label from some pods")
		expectSeedPodsLabelsBeSet(cc, []string{
			"test-cassandra-cluster-cassandra-dc1-0",
			"test-cassandra-cluster-cassandra-dc1-1",
			"test-cassandra-cluster-cassandra-dc1-2",
			"test-cassandra-cluster-cassandra-dc2-0",
			"test-cassandra-cluster-cassandra-dc2-1",
			"test-cassandra-cluster-cassandra-dc2-2",
		})

	})
})

func expectSeedPodsLabelsBeSet(cc *v1alpha1.CassandraCluster, seedPods []string) {
	Eventually(func() bool {
		matchPodLabels := client.MatchingLabels(labels.ComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra))
		cassandraPods := &v1.PodList{}
		Expect(k8sClient.List(ctx, cassandraPods, client.InNamespace(cc.Namespace), matchPodLabels)).To(Succeed())

		for _, pod := range cassandraPods.Items {
			if util.Contains(seedPods, pod.Name) {
				_, labelExists := pod.Labels[v1alpha1.CassandraClusterSeed]
				if !labelExists {
					return false
				}
			} else {
				_, labelExists := pod.Labels[v1alpha1.CassandraClusterSeed]
				if labelExists {
					return false
				}
			}
		}
		return true
	}, mediumTimeout, mediumRetry).Should(BeTrue(), "only seed pods should have seed label")
}
