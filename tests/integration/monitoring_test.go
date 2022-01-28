package integration

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Cassandra monitoring", func() {
	Context("when monitoring is not defined", func() {
		It("should be empty", func() {
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
					Cassandra:           &v1alpha1.Cassandra{},
				},
			}
			createReadyCluster(cc)
			actualCC := getCassandraCluster(cc)
			Expect(actualCC.Spec.Cassandra.Monitoring.Enabled).To(BeFalse())
			Expect(actualCC.Spec.Cassandra.Monitoring.Agent).To(BeEmpty())
			Expect(actualCC.Spec.Cassandra.Monitoring.ServiceMonitor.Enabled).To(BeFalse())
			Expect(actualCC.Spec.Cassandra.Monitoring.ServiceMonitor.Namespace).To(BeEmpty())
			Expect(actualCC.Spec.Cassandra.Monitoring.ServiceMonitor.Labels).To(BeEmpty())
			Expect(actualCC.Spec.Cassandra.Monitoring.ServiceMonitor.ScrapeInterval).To(BeEmpty())
		})
	})

	Context("when monitoring is enabled with tlp agent", func() {
		It("should create tlp volume, volume mount, and port", func() {
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
					Cassandra: &v1alpha1.Cassandra{
						Monitoring: v1alpha1.Monitoring{
							Enabled: true,
							Agent:   v1alpha1.CassandraAgentTlp,
						},
					},
				},
			}
			createReadyCluster(cc)
			checkVolume("prometheus-config", "dc1")
			checkPort("agent", "dc1", v1alpha1.TlpPort)
		})
	})

	Context("when monitoring is enabled with datastax agent", func() {
		It("should create datastax volume, volume mount, and port", func() {
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
					Cassandra: &v1alpha1.Cassandra{
						Monitoring: v1alpha1.Monitoring{
							Enabled: true,
							Agent:   v1alpha1.CassandraAgentDatastax,
						},
					},
				},
			}
			createReadyCluster(cc)
			checkVolume("collectd-config", "dc1")
			checkPort("agent", "dc1", v1alpha1.DatastaxPort)
		})
	})

	Context("when monitoring is enabled with instaclustr agent", func() {
		It("should create instaclustr port", func() {
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
					Cassandra: &v1alpha1.Cassandra{
						Monitoring: v1alpha1.Monitoring{
							Enabled: true,
							Agent:   v1alpha1.CassandraAgentInstaclustr,
						},
					},
				},
			}
			createReadyCluster(cc)
			checkPort("agent", "dc1", v1alpha1.InstaclustrPort)
		})
	})
})

func checkPort(name, dc string, port int32) {
	service := &v1.Service{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.DCService(cassandraObjectMeta.Name, dc), Namespace: cassandraObjectMeta.Namespace}, service)).To(Succeed())

	servicePort, found := getServicePortByName(service.Spec.Ports, name)
	Expect(servicePort.Port).To(Equal(port))
	Expect(found).To(BeTrue())
}
