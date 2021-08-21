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
				{Name: "DEBUG", Value: "false"},
				{Name: "JOLOKIA_PORT", Value: "8080"},
				{Name: "SERVER_PORT", Value: "8888"},
				{Name: "JMX_POLL_PERIOD_SECONDS", Value: "10"},
				{Name: "JMX_PORT", Value: "7199"},
				{Name: "ADMIN_SECRET_NAME", Value: "test-cassandra-cluster-auth-active-admin"},
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
					fmt.Sprintf("cp /etc/cassandra-configmaps/* $CASSANDRA_CONF/\n" +
						"cp /etc/cassandra-configmaps/jvm.options $CASSANDRA_HOME/\n" +
						"source /etc/pods-config/${POD_NAME}_${POD_UID}.sh\n" +
						"/docker-entrypoint.sh -f -R " +
						"-Dcassandra.jmx.remote.port=7199 " +
						"-Dcom.sun.management.jmxremote.rmi.port=7199 " +
						"-Djava.rmi.server.hostname=$CASSANDRA_BROADCAST_ADDRESS " +
						"-Dcom.sun.management.jmxremote.authenticate=true " +
						"-Dcassandra.jmx.remote.login.config=CassandraLogin " +
						"-Djava.security.auth.login.config=$CASSANDRA_HOME/conf/cassandra-jaas.config " +
						"-Dcassandra.jmx.authorizer=org.apache.cassandra.auth.jmx.AuthorizationProxy"),
				}))
			}

			By("reaper shouldn't be deployed until all DCs ready")
			Consistently(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{Name: names.ReaperDeployment(cc.Name, cc.Spec.DCs[0].Name), Namespace: cc.Namespace}, &appsv1.Deployment{})
			}, shortTimeout, shortRetry).ShouldNot(Succeed())

			By("reaper should be deployed after DCs ready")
			activeAdminSecret := &v1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.ActiveAdminSecret(cc.Name), Namespace: cc.Namespace}, activeAdminSecret)).To(Succeed())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cc.Name, Namespace: cc.Namespace}, cc)).To(Succeed())
			markAllDCsReady(cc)
			createCassandraPods(cc)
			mockReaperClient.isRunning = false
			mockReaperClient.err = nil

			for index, dc := range cc.Spec.DCs {
				// Check if first reaper deployment has been created
				if index == 0 {
					// Wait for the operator to create the first reaper deployment
					validateNumberOfDeployments(cc.Namespace, reaperDeploymentLabels, 1)
				}

				reaperDeployment := markDeploymentAsReady(types.NamespacedName{Name: names.ReaperDeployment(cc.Name, dc.Name), Namespace: cc.Namespace})

				Expect(reaperDeployment.Spec.Template.Spec.Containers[0].Args).Should(Equal([]string(nil)))

				Expect(reaperDeployment.Spec.Template.Spec.Containers[0].Env).Should(BeEquivalentTo([]v1.EnvVar{
					{
						Name:  "ACTIVE_ADMIN_SECRET_VERSION",
						Value: activeAdminSecret.ResourceVersion,
					},
					{
						Name:  "REAPER_CASS_ACTIVATE_QUERY_LOGGER",
						Value: "true",
					},
					{
						Name:  "REAPER_LOGGING_ROOT_LEVEL",
						Value: "INFO",
					},
					{
						Name:  "REAPER_LOGGING_APPENDERS_CONSOLE_THRESHOLD",
						Value: "INFO",
					},
					{
						Name:  "REAPER_DATACENTER_AVAILABILITY",
						Value: "each",
					},
					{
						Name:  "REAPER_REPAIR_INTENSITY",
						Value: "1.0",
					},
					{
						Name:  "REAPER_REPAIR_MANAGER_SCHEDULING_INTERVAL_SECONDS",
						Value: "0",
					},
					{
						Name:  "REAPER_BLACKLIST_TWCS",
						Value: "false",
					},
					{
						Name:  "REAPER_CASS_CONTACT_POINTS",
						Value: fmt.Sprintf("[ %s ]", names.DC(cc.Name, dc.Name)),
					},
					{
						Name:  "REAPER_CASS_CLUSTER_NAME",
						Value: "cassandra",
					},
					{
						Name:  "REAPER_STORAGE_TYPE",
						Value: "cassandra",
					},
					{
						Name:  "REAPER_CASS_KEYSPACE",
						Value: "reaper_db",
					},
					{
						Name:  "REAPER_CASS_PORT",
						Value: "9042",
					},
					{
						Name:  "JAVA_OPTS",
						Value: "",
					},
					{
						Name:  "REAPER_CASS_AUTH_ENABLED",
						Value: "true",
					},
					{
						Name:  "REAPER_SHIRO_INI",
						Value: "/shiro/shiro.ini",
					},
					{
						Name: "REAPER_CASS_AUTH_USERNAME",
						ValueFrom: &v1.EnvVarSource{
							SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: "test-cassandra-cluster-auth-config-admin",
								},
								Key: "admin_username",
							},
						},
					},
					{
						Name: "REAPER_CASS_AUTH_PASSWORD",
						ValueFrom: &v1.EnvVarSource{
							SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: "test-cassandra-cluster-auth-config-admin",
								},
								Key: "admin_password",
							},
						},
					},
					{
						Name: "REAPER_JMX_AUTH_USERNAME",
						ValueFrom: &v1.EnvVarSource{
							SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: "test-cassandra-cluster-auth-config-admin",
								},
								Key: "admin_username",
							},
						},
					},
					{
						Name:  "REAPER_JMX_AUTH_PASSWORD",
						Value: "",
						ValueFrom: &v1.EnvVarSource{
							SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: "test-cassandra-cluster-auth-config-admin",
								},
								Key: "admin_password",
							},
						},
					},
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
