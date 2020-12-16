package integration

import (
	"fmt"
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

var _ = Describe("operator configmaps", func() {
	Context("when tests start", func() {
		It("should exist", func() {
			for _, cmName := range []string{names.OperatorCassandraConfigCM(), names.OperatorProberSourcesCM(), names.OperatorScriptsCM(), names.OperatorShiroCM()} {
				cm := &v1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: operatorConfig.Namespace}, cm)
				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("ConfigMap %q should exist", cmName))
			}
		})
	})
})

var _ = Describe("prober, statefulsets, kwatcher and reaper", func() {
	cc := &v1alpha1.CassandraCluster{
		ObjectMeta: cassandraObjectMeta,
		Spec: v1alpha1.CassandraClusterSpec{
			DCs: []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(3),
				},
				{
					Name:     "dc2",
					Replicas: proto.Int32(6),
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

			for _, dc := range cc.Spec.DCs {
				sts := &appsv1.StatefulSet{}
				cassandraLabels := map[string]string{
					"cassandra-cluster-component": "cassandra",
					"cassandra-cluster-instance":  "test-cassandra-cluster",
					"cassandra-cluster-dc":        dc.Name,
					"datacenter":                  dc.Name,
				}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: names.DC(cc, dc.Name), Namespace: cc.Namespace}, sts)
				}, time.Second*5, time.Millisecond*100).Should(Succeed())

				Expect(sts.Labels).To(BeEquivalentTo(cassandraLabels))
				Expect(sts.Spec.Replicas).To(BeEquivalentTo(dc.Replicas))
				Expect(sts.Spec.Selector.MatchLabels).To(BeEquivalentTo(cassandraLabels))
				Expect(sts.Spec.ServiceName).To(Equal("test-cassandra-cluster" + "-cassandra-" + dc.Name))
				Expect(sts.Spec.Template.Labels).To(Equal(cassandraLabels))
				Expect(sts.OwnerReferences[0].UID).To(Equal(cc.UID))
				Expect(sts.OwnerReferences[0].Controller).To(Equal(proto.Bool(true)))
				Expect(sts.OwnerReferences[0].Kind).To(Equal("CassandraCluster"))
				Expect(sts.OwnerReferences[0].APIVersion).To(Equal("db.ibm.com/v1alpha1"))
				Expect(sts.OwnerReferences[0].Name).To(Equal(cc.Name))
				Expect(sts.OwnerReferences[0].BlockOwnerDeletion).To(Equal(proto.Bool(true)))
				cassandraContainer, found := getContainerByName(sts.Spec.Template.Spec, "cassandra")
				Expect(found).To(BeTrue())
				Expect(cassandraContainer.Image).To(Equal(operatorConfig.DefaultCassandraImage), "default values")
				Expect(cassandraContainer.ImagePullPolicy).To(Equal(v1.PullIfNotPresent), "default values")
			}
		})
	})
})
