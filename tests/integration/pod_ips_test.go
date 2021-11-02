package integration

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("pod IPs", func() {
	It("should be saved in the configmap", func() {
		cc := &v1alpha1.CassandraCluster{
			ObjectMeta: cassandraObjectMeta,
			Spec: v1alpha1.CassandraClusterSpec{
				DCs: []v1alpha1.DC{
					{
						Name:     "dc1",
						Replicas: proto.Int32(3),
					},
				},
				ImagePullSecretName: "pull-secret-name",
				AdminRoleSecretName: "admin-role",
			},
		}

		createReadyCluster(cc)
		currentPodsList := &v1.PodList{}
		cassandraPodLabels := client.MatchingLabels(labels.ComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra))
		Expect(k8sClient.List(ctx, currentPodsList, client.InNamespace(cc.Namespace), cassandraPodLabels)).To(Succeed())
		currentIPsCM := &v1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.PodIPsConfigMap(cc.Name), Namespace: cc.Namespace}, currentIPsCM)).To(Succeed())

		for _, pod := range currentPodsList.Items {
			Expect(currentIPsCM.Data).To(HaveKeyWithValue(pod.Name, pod.Status.PodIP))
		}

		// emulate pod failure and coming back with a new IP but not ready yet
		firstPod := currentPodsList.Items[0]
		newPodIP := "10.0.0.43"
		firstPod.Status.PodIP = newPodIP
		firstPod.Status.ContainerStatuses[0].Ready = false
		Expect(k8sClient.Status().Update(ctx, &firstPod)).To(Succeed())

		Eventually(func() bool {
			Expect(k8sClient.List(ctx, currentPodsList, client.InNamespace(cc.Namespace), client.MatchingLabels(labels.ComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra))))
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.PodIPsConfigMap(cc.Name), Namespace: cc.Namespace}, currentIPsCM))

			for _, pod := range currentPodsList.Items {
				if pod.Name == firstPod.Name { //first pod should have the old IP until it's ready
					if currentIPsCM.Data[pod.Name] == pod.Status.PodIP {
						return false
					}
					continue
				}

				if currentIPsCM.Data[pod.Name] != pod.Status.PodIP {
					return false
				}
			}
			return true
		}, mediumRetry, mediumRetry).Should(BeTrue(), "the IP should not be updated in the config map until the pod is ready")

		firstPod = currentPodsList.Items[0]
		firstPod.Status.ContainerStatuses[0].Ready = true
		Expect(k8sClient.Status().Update(ctx, &firstPod)).To(Succeed())

		Eventually(func() bool {
			Expect(k8sClient.List(ctx, currentPodsList, client.InNamespace(cc.Namespace), client.MatchingLabels(labels.ComponentLabels(cc, v1alpha1.CassandraClusterComponentCassandra))))
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.PodIPsConfigMap(cc.Name), Namespace: cc.Namespace}, currentIPsCM))

			for _, pod := range currentPodsList.Items {
				if currentIPsCM.Data[pod.Name] != pod.Status.PodIP {
					return false
				}
			}
			return true
		}, mediumTimeout, mediumRetry).Should(BeTrue(), "configmap should be updated with the new IP since the pod is ready")
	})
})
