package integration

import (
	"context"
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
			for _, cmName := range []string{names.OperatorCassandraConfigCM(), names.OperatorProberSourcesCM(), names.OperatorScriptsCM()} {
				cm := &v1.ConfigMap{}
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cmName, Namespace: operatorConfig.Namespace}, cm)
				Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("ConfigMap %q should exist", cmName))
			}
		})
	})
})

var _ = Describe("prober, statefulsets and kwatcher", func() {
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
					Replicas: proto.Int32(3),
				},
			},
			Cassandra: v1alpha1.Cassandra{
				NumSeeds: 2,
				UsersDir: "/etc/cassandra-users",
				JMXPort:  3200,
				Auth: v1alpha1.CassandraAuth{
					User:     "cassandra",
					Password: "cassandra",
				},
				Image:           "cassandra/image",
				ImagePullPolicy: "Never",
			},
			InternalAuth: true,
			Kwatcher: v1alpha1.Kwatcher{
				Enabled:         true,
				Image:           "kwather/image",
				ImagePullPolicy: "Never",
			},
			Prober: v1alpha1.Prober{
				Image:           "prober/image",
				ImagePullPolicy: "Never",
				ServerPort:      9090,
				Debug:           false,
				Jolokia: v1alpha1.Jolokia{
					Image:           "jolokia/image",
					ImagePullPolicy: "Never",
				},
			},
			CQLConfigMapLabelKey: "cql-cm",
			HostPortEnabled:      false,
			ImagePullSecretName:  "pull-secret-name",
			SystemKeyspaces: v1alpha1.SystemKeyspaces{
				Names: []string{"system_auth"},
				DCs: []v1alpha1.SystemKeyspaceDC{{
					Name: "dc1",
					RF:   2,
				}},
			},
		},
	}

	Context("when cassandracluster created", func() {
		It("should be created", func() {
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())
			sts := &appsv1.StatefulSet{}
			mockProberClient.err = nil
			mockNodetoolClient.err = nil
			mockCQLClient.err = nil
			mockCQLClient.cassandraUsers = []cql.CassandraUser{{Role: "cassandra", IsSuperuser: true}}
			mockCQLClient.keyspaces = []cql.Keyspace{{
				Name: "system_auth",
				Replication: map[string]string{
					"class": "org.apache.cassandra.locator.NetworkTopologyStrategy",
					"dc1":   "3",
				},
			}}

			By("prober should be deployed")
			proberDeployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{Name: names.ProberDeployment(cc), Namespace: cc.Namespace}, proberDeployment)
			}, time.Second*5, time.Millisecond*100).Should(Succeed())
			Expect(proberDeployment.Spec.Template.Spec.Containers[0].Env).To(BeEquivalentTo([]v1.EnvVar{
				{Name: "POD_NAMESPACE", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.namespace"}}},
				{Name: "LOCAL_DCS", Value: "[{\"name\":\"dc1\",\"replicas\":3},{\"name\":\"dc2\",\"replicas\":3}]"},
				{Name: "DEBUG", Value: "false"},
				{Name: "HOSTPORT_ENABLED", Value: "false"},
				{Name: "CASSANDRA_ENDPOINT_LABELS", Value: "cassandra-cluster-component=cassandra,cassandra-cluster-instance=test-cassandra-cluster"},
				{Name: "CASSANDRA_LOCAL_SEEDS_HOSTNAMES", Value: "test-cassandra-cluster-cassandra-dc1-0.test-cassandra-cluster-cassandra-dc1.default.svc.cluster.local,test-cassandra-cluster-cassandra-dc1-1.test-cassandra-cluster-cassandra-dc1.default.svc.cluster.local,test-cassandra-cluster-cassandra-dc2-0.test-cassandra-cluster-cassandra-dc2.default.svc.cluster.local,test-cassandra-cluster-cassandra-dc2-1.test-cassandra-cluster-cassandra-dc2.default.svc.cluster.local"},
				{Name: "JMX_PROXY_URL", Value: "http://localhost:8080/jolokia"},
				{Name: "EXTERNAL_DCS_INGRESS_DOMAINS", Value: "[]"},
				{Name: "ALL_DCS_INGRESS_DOMAINS", Value: "null"},
				{Name: "LOCAL_DC_INGRESS_DOMAIN", Value: ""},
				{Name: "CASSANDRA_NUM_SEEDS", Value: "2"},
				{Name: "JOLOKIA_PORT", Value: "8080"},
				{Name: "JOLOKIA_RESPONSE_TIMEOUT", Value: "10000"},
				{Name: "PROBER_SUBDOMAIN", Value: "default-test-cassandra-cluster-cassandra-prober"},
				{Name: "SERVER_PORT", Value: "8888"},
				{Name: "JMX_POLL_PERIOD_SECONDS", Value: "10"},
				{Name: "JMX_PORT", Value: "3200"},
				{Name: "USERS_DIR", Value: "/etc/cassandra-users"},
			}))

			By("cassandra dcs should not exist until prober is ready")
			Consistently(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{Name: names.DC(cc, cc.Spec.DCs[0].Name), Namespace: cc.Namespace}, sts)
			}, time.Second*1, time.Millisecond*100).ShouldNot(Succeed())

			mockProberClient.ready = true
			mockProberClient.readyAllDCs = false

			By("should be created after prober becomes ready")
			for _, dc := range cc.Spec.DCs {
				mockProberClient.ready = true
				Eventually(func() error {
					return k8sClient.Get(context.Background(), types.NamespacedName{Name: names.DC(cc, dc.Name), Namespace: cc.Namespace}, sts)
				}, time.Second*5, time.Millisecond*100).Should(Succeed())

				By("Cassandra run command should be set correctly")
				Expect(sts.Spec.Template.Spec.Containers[0].Args).To(BeEquivalentTo([]string{
					"bash",
					"-c",
					`cp /etc/cassandra-configmaps/* $CASSANDRA_CONF
cp /etc/cassandra-configmaps/jvm.options $CASSANDRA_HOME
until stat $CASSANDRA_CONF/cassandra.yaml; do sleep 5; done
echo "broadcast_address: $POD_IP" >> $CASSANDRA_CONF/cassandra.yaml
echo "broadcast_rpc_address: $POD_IP" >> $CASSANDRA_CONF/cassandra.yaml
exec cassandra -R -f -Dcassandra.jmx.remote.port=3200 -Dcom.sun.management.jmxremote.rmi.port=3200 -Dcom.sun.management.jmxremote.authenticate=internal-Djava.rmi.server.hostname=$POD_IP
`,
				}))
			}

			By("kwatcher shouldn't be deployed until DCs ready")
			Consistently(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{Name: names.KwatcherDeployment(cc, cc.Spec.DCs[0].Name), Namespace: cc.Namespace}, &appsv1.Deployment{})
			}, time.Second*1, time.Millisecond*100).ShouldNot(Succeed())

			By("kwatcher should be deployed after DCs ready")
			mockProberClient.readyAllDCs = true
			for _, dc := range cc.Spec.DCs {
				kwatcherDeploy := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(context.Background(), types.NamespacedName{Name: names.KwatcherDeployment(cc, dc.Name), Namespace: cc.Namespace}, kwatcherDeploy)
				}, time.Second*5, time.Millisecond*100).Should(Succeed())

				Expect(kwatcherDeploy.Spec.Template.Spec.Containers[0].Command).Should(Equal([]string{
					"./kwatcher",
					"-namespace", "default",
					"-appname", "test-cassandra-cluster",
					"-hosts", "test-cassandra-cluster-cassandra-" + dc.Name,
					"-dcname", dc.Name,
					"-statefulsetname", "test-cassandra-cluster-cassandra-" + dc.Name,
					"-redact",
					"-repairjobimage", "cassandra/image",
					"-port", "9042",
				}))
			}
		})
	})
})
