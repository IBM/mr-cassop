package integration

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ibm/cassandra-operator/controllers/labels"

	"github.com/google/go-cmp/cmp"

	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
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
			AdminRoleSecretName: "admin-role",
		},
	}

	Context("when cassandracluster created", func() {
		It("should be created", func() {
			createAdminSecret(cc)
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())
			sts := &appsv1.StatefulSet{}
			mockProberClient.err = nil
			mockNodetoolClient.err = nil
			mockCQLClient.err = nil
			mockCQLClient.cassandraRoles = []cql.Role{{Role: "cassandra", Password: "cassandra", Login: true, Super: true}}
			mockCQLClient.keyspaces = []cql.Keyspace{{
				Name: "system_auth",
				Replication: map[string]string{
					"class": cql.ReplicationClassNetworkTopologyStrategy,
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
				{Name: "LOGLEVEL", Value: "info"},
				{Name: "LOGFORMAT", Value: "json"},
				{Name: "JOLOKIA_PORT", Value: "8080"},
				{Name: "SERVER_PORT", Value: "8888"},
				{Name: "JMX_POLLING_INTERVAL", Value: "10s"},
				{Name: "JMX_PORT", Value: "7199"},
				{Name: "ADMIN_SECRET_NAME", Value: "test-cassandra-cluster-auth-active-admin"},
				{Name: "BASE_ADMIN_SECRET_NAME", Value: "admin-role"},
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
				cassandraContainer, found := getContainerByName(sts.Spec.Template.Spec, "cassandra")
				Expect(found).To(BeTrue())
				Expect(cassandraContainer.Args).To(BeEquivalentTo([]string{
					"bash",
					"-c",
					fmt.Sprintf("rm -rf /var/lib/cassandra/data/system/peers*\n" +
						"echo \"prefer_local=true\" >> $CASSANDRA_CONF/cassandra-rackdc.properties\n" +
						"cp /etc/cassandra-configmaps/* $CASSANDRA_CONF/\n" +
						"cp /etc/cassandra-configmaps/jvm.options $CASSANDRA_HOME/\n" +
						"source /etc/pods-config/${POD_NAME}_${POD_UID}.sh\n" +
						"replace_address=\"\"\n" +
						"old_ip=$CASSANDRA_NODE_PREVIOUS_IP\n" +
						"if [[ \"${old_ip}\" != \"\" ]]; then\n" +
						"  if [ ! -d \"/var/lib/cassandra/data\" ] || [ -z \"$(ls -A /var/lib/cassandra/data)\" ]; then\n" +
						"    replace_address=\"-Dcassandra.replace_address_first_boot=${old_ip}\"\n" +
						"    echo replacing old Cassandra node - adding arg $replace_address\n" +
						"  else\n" +
						"    echo not using replace address since the storage directory is not empty\n" +
						"  fi\n" +
						"else\n" +
						"  echo not using replace address since the node IP hasn\\'t changed\n" +
						"fi\n" +
						"/docker-entrypoint.sh -f -R " +
						"-Dcassandra.jmx.remote.port=7199 " +
						"-Dcom.sun.management.jmxremote.rmi.port=7199 " +
						"-Djava.rmi.server.hostname=$POD_IP " +
						"-Dcom.sun.management.jmxremote.authenticate=true " +
						"-Dcassandra.storagedir=/var/lib/cassandra " +
						"${replace_address} " +
						"-Dcassandra.jmx.remote.login.config=CassandraLogin " +
						"-Djava.security.auth.login.config=$CASSANDRA_HOME/conf/cassandra-jaas.config " +
						"-Dcassandra.jmx.authorizer=org.apache.cassandra.auth.jmx.AuthorizationProxy"),
				}))

				icarusContainer, found := getContainerByName(sts.Spec.Template.Spec, "icarus")
				Expect(found).To(BeTrue())
				Expect(icarusContainer.Args).To(BeEquivalentTo([]string{"--jmx-credentials=/etc/cassandra-auth-config/icarus-jmx", "--jmx-client-auth=true"}))
				Expect(icarusContainer.VolumeMounts).To(BeEquivalentTo([]v1.VolumeMount{
					{
						Name:             "data",
						ReadOnly:         false,
						MountPath:        "/var/lib/cassandra",
						SubPath:          "",
						MountPropagation: nil,
						SubPathExpr:      "",
					},
					{
						Name:             "auth-config",
						ReadOnly:         false,
						MountPath:        "/etc/cassandra-auth-config/",
						SubPath:          "",
						MountPropagation: nil,
						SubPathExpr:      "",
					},
				}))

				Expect(sts.Spec.Template.Spec.TopologySpreadConstraints).To(BeEquivalentTo([]v1.TopologySpreadConstraint{
					{
						TopologyKey:       v1.LabelTopologyZone,
						MaxSkew:           1,
						WhenUnsatisfiable: v1.ScheduleAnyway,
						LabelSelector:     metav1.SetAsLabelSelector(labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra)),
					},
				}))
			}

			By("reaper shouldn't be deployed until all DCs ready")
			Consistently(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{Name: names.ReaperDeployment(cc.Name, cc.Spec.DCs[0].Name), Namespace: cc.Namespace}, &appsv1.Deployment{})
			}, shortTimeout, shortRetry).ShouldNot(Succeed())

			By("reaper should be deployed after DCs ready")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cc.Name, Namespace: cc.Namespace}, cc)).To(Succeed())
			markAllDCsReady(cc)
			createCassandraPods(cc)
			mockReaperClient.isRunning = false
			mockReaperClient.err = nil

			activeAdminSecret := &v1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: names.ActiveAdminSecret(cc.Name), Namespace: cc.Namespace}, activeAdminSecret)
				if err != nil && errors.IsNotFound(err) { // try again if the secret was deleted and haven't got created yet
					return false
				}
				Expect(err).ToNot(HaveOccurred())
				return string(activeAdminSecret.Data[v1alpha1.CassandraOperatorAdminRole]) != v1alpha1.CassandraDefaultRole
			}, mediumTimeout, mediumRetry).Should(BeTrue(), "the active admin secret should be updated with the non default user")
			adminSecretChecksum := util.Sha1(fmt.Sprintf("%v", activeAdminSecret.Data))

			for index, dc := range cc.Spec.DCs {
				// Check if first reaper deployment has been created
				if index == 0 {
					// Wait for the operator to create the first reaper deployment
					validateNumberOfDeployments(cc.Namespace, reaperDeploymentLabels, 1)
				}

				reaperDeployment := markDeploymentAsReady(types.NamespacedName{Name: names.ReaperDeployment(cc.Name, dc.Name), Namespace: cc.Namespace})

				Expect(reaperDeployment.Spec.Template.Spec.Containers[0].Args).Should(Equal([]string(nil)))

				expectedEnvVars := []v1.EnvVar{
					{
						Name:  "ACTIVE_ADMIN_SECRET_SHA1",
						Value: adminSecretChecksum,
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
						Value: "EACH",
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
						Value: "test-cassandra-cluster",
					},
					{
						Name:  "REAPER_STORAGE_TYPE",
						Value: "cassandra",
					},
					{
						Name:  "REAPER_CASS_KEYSPACE",
						Value: "reaper",
					},
					{
						Name:  "REAPER_CASS_PORT",
						Value: fmt.Sprintf("%d", dbv1alpha1.CqlPort),
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
								Key: "admin-role",
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
								Key: "admin-password",
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
								Key: "admin-role",
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
								Key: "admin-password",
							},
						},
					},
					{Name: "REAPER_INCREMENTAL_REPAIR", Value: "false"},
					{Name: "REAPER_REPAIR_PARALELLISM", Value: "DATACENTER_AWARE"},
				}

				Expect(reaperDeployment.Spec.Template.Spec.Containers[0].Env).Should(BeEquivalentTo(expectedEnvVars), cmp.Diff(reaperDeployment.Spec.Template.Spec.Containers[0].Env, expectedEnvVars))
			}
			mockReaperClient.isRunning = true
			mockReaperClient.err = nil

			Eventually(func() []string {
				return mockReaperClient.clusters
			}, shortTimeout, shortRetry).Should(BeEquivalentTo([]string{cc.Name}))

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cc.Name, Namespace: cc.Namespace}, cc)).To(Succeed())
				return cc.Status.Ready
			}).Should(BeTrue())
		})
	})
})
