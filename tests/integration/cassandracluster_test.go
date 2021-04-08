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
	"strings"
)

var _ = Describe("prober, statefulsets and reaper", func() {
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
			ImagePullSecretName: "pull-secret-name",
		},
	}

	Context("when cassandracluster created", func() {
		It("should be created", func() {
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())
			sts := &appsv1.StatefulSet{}
			mockProberClient.err = nil
			mockNodetoolClient.err = nil
			mockCQLClient.err = nil
			mockCQLClient.cassandraRoles = []cql.Role{{Role: "cassandra", Super: true}}
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
				return k8sClient.Get(context.Background(), types.NamespacedName{Name: names.ProberDeployment(cc.Name), Namespace: cc.Namespace}, proberDeployment)
			}, mediumTimeout, mediumRetry).Should(Succeed())
			Expect(proberDeployment.Spec.Template.Spec.Containers[0].Env).To(BeEquivalentTo([]v1.EnvVar{
				{Name: "POD_NAMESPACE", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.namespace"}}},
				{Name: "LOCAL_DCS", Value: "[{\"name\":\"dc1\",\"replicas\":3},{\"name\":\"dc2\",\"replicas\":3}]"},
				{Name: "DEBUG", Value: "false"},
				{Name: "HOSTPORT_ENABLED", Value: "false"},
				{Name: "CASSANDRA_ENDPOINT_LABELS", Value: "cassandra-cluster-component=cassandra,cassandra-cluster-instance=test-cassandra-cluster"},
				{Name: "CASSANDRA_LOCAL_SEEDS_HOSTNAMES", Value: "test-cassandra-cluster-cassandra-dc1-0.test-cassandra-cluster-cassandra-dc1.default.svc.cluster.local,test-cassandra-cluster-cassandra-dc1-1.test-cassandra-cluster-cassandra-dc1.default.svc.cluster.local,test-cassandra-cluster-cassandra-dc2-0.test-cassandra-cluster-cassandra-dc2.default.svc.cluster.local,test-cassandra-cluster-cassandra-dc2-1.test-cassandra-cluster-cassandra-dc2.default.svc.cluster.local"},
				{Name: "CASSANDRA_NUM_SEEDS", Value: "2"},
				{Name: "EXTERNAL_DCS_INGRESS_DOMAINS", Value: "null"},
				{Name: "ALL_DCS_INGRESS_DOMAINS", Value: "null"},
				{Name: "LOCAL_DC_INGRESS_DOMAIN", Value: ""},
				{Name: "JOLOKIA_PORT", Value: "8080"},
				{Name: "JOLOKIA_RESPONSE_TIMEOUT", Value: "10000"},
				{Name: "PROBER_SUBDOMAIN", Value: "default-test-cassandra-cluster-cassandra-prober"},
				{Name: "SERVER_PORT", Value: "8888"},
				{Name: "JMX_POLL_PERIOD_SECONDS", Value: "10"},
				{Name: "JMX_PROXY_URL", Value: "http://localhost:8080/jolokia"},
				{Name: "JMX_PORT", Value: "7199"},
				{Name: "USERS_DIR", Value: "/etc/cassandra-roles"},
			}))

			By("cassandra dcs should not exist until prober is ready")
			Consistently(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{Name: names.DC(cc.Name, cc.Spec.DCs[0].Name), Namespace: cc.Namespace}, sts)
			}, shortTimeout, shortRetry).ShouldNot(Succeed())

			mockProberClient.ready = true

			By("should be created after prober becomes ready")
			for _, dc := range cc.Spec.DCs {
				mockProberClient.ready = true
				Eventually(func() error {
					return k8sClient.Get(context.Background(), types.NamespacedName{Name: names.DC(cc.Name, dc.Name), Namespace: cc.Namespace}, sts)
				}, mediumTimeout, mediumRetry).Should(Succeed())

				By("Cassandra run command should be set correctly")
				Expect(sts.Spec.Template.Spec.Containers[0].Args).To(BeEquivalentTo([]string{
					"bash",
					"-c",
					`cp /etc/cassandra-configmaps/* $CASSANDRA_CONF
cp /etc/cassandra-configmaps/jvm.options $CASSANDRA_HOME
COUNT=1; ATTEMPTS=14; until stat /etc/pods-config/${POD_NAME}_${POD_UID}.env || [[ $COUNT -eq $ATTEMPTS ]]; do echo -e "Waiting... Attempt $(( COUNT++ ))..."; sleep 5; done; [[ $COUNT -eq $ATTEMPTS ]] && echo "Could not access mount" && exit 1
source /etc/pods-config/${POD_NAME}_${POD_UID}.env
./docker-entrypoint.sh -f -R -Dcassandra.jmx.remote.port=7199 -Dcom.sun.management.jmxremote.rmi.port=7199 -Dcom.sun.management.jmxremote.authenticate=false -Djava.rmi.server.hostname=$CASSANDRA_IP
`,
				}))
			}

			By("reaper shouldn't be deployed until all DCs ready")
			Consistently(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{Name: names.ReaperDeployment(cc.Name, cc.Spec.DCs[0].Name), Namespace: cc.Namespace}, &appsv1.Deployment{})
			}, shortTimeout, shortRetry).ShouldNot(Succeed())

			By("reaper should be deployed after DCs ready")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cc.Name, Namespace: cc.Namespace}, cc)).To(Succeed())
			cc.Status.ReadyAllDCs = true
			Expect(k8sClient.Status().Update(ctx, cc)).To(Succeed())
			mockReaperClient.isRunning = false
			mockReaperClient.err = nil

			for index, dc := range cc.Spec.DCs {
				// Check if first reaper deployment has been created
				if index == 0 {
					// Wait for the operator to create the first reaper deployment
					validateNumberOfDeployments(cc.Namespace, reaperDeploymentLabels, 1)
				}

				reaperDeployment := markDeploymentAsReady(types.NamespacedName{Name: names.ReaperDeployment(cc.Name, dc.Name), Namespace: cc.Namespace})

				Expect(reaperDeployment.Spec.Template.Spec.Containers[0].Args).Should(Equal([]string{
					"sh",
					"-c",
					strings.Join([]string{
						"export $(cat /etc/reaper-auth/auth.env);",
						"/usr/local/bin/entrypoint.sh cassandra-reaper;",
					}, "\n"),
				}))

				Expect(reaperDeployment.Spec.Template.Spec.Containers[0].Env).Should(BeEquivalentTo([]v1.EnvVar{
					{Name: "REAPER_CASS_ACTIVATE_QUERY_LOGGER", Value: "true"},
					{Name: "REAPER_LOGGING_ROOT_LEVEL", Value: "INFO"},
					{Name: "REAPER_LOGGING_APPENDERS_CONSOLE_THRESHOLD", Value: "INFO"},
					{Name: "REAPER_DATACENTER_AVAILABILITY", Value: "each"},
					{Name: "REAPER_REPAIR_INTENSITY", Value: "1.0"},
					{Name: "REAPER_REPAIR_MANAGER_SCHEDULING_INTERVAL_SECONDS", Value: "0"},
					{Name: "REAPER_BLACKLIST_TWCS", Value: "false"},
					{Name: "REAPER_CASS_CONTACT_POINTS", Value: fmt.Sprintf("[ %s ]", names.DC(cc.Name, dc.Name))},
					{Name: "REAPER_CASS_CLUSTER_NAME", Value: "cassandra"},
					{Name: "REAPER_STORAGE_TYPE", Value: "cassandra"},
					{Name: "REAPER_CASS_KEYSPACE", Value: "reaper_db"},
					{Name: "REAPER_CASS_PORT", Value: "9042"},
					{Name: "JAVA_OPTS", Value: ""},
					{Name: "REAPER_SHIRO_INI", Value: "/shiro/shiro.ini"},
				}))
			}
			mockReaperClient.isRunning = true
			mockReaperClient.err = nil

			Eventually(func() bool {
				return mockReaperClient.clusterExists
			}, shortTimeout, shortRetry).Should(BeTrue())
		})
	})
})
