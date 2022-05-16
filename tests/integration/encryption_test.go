package integration

import (
	"fmt"
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Cassandra TLS encryption", func() {
	baseCC := &v1alpha1.CassandraCluster{
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
			AdminRoleSecretName: "admin-role",
			ImagePullSecretName: "pull-secret-name",
		},
	}

	configConfigmap := &v1.ConfigMap{}
	caTLSSecret := &v1.Secret{}
	nodeTLSSecret := &v1.Secret{}

	Context("when server encryption is enabled", func() {
		It("when user doesn't provide Cluster TLS Secrets", func() {
			cc := baseCC.DeepCopy()
			cc.Spec.Encryption.Server = v1alpha1.ServerEncryption{
				InternodeEncryption: "dc",
			}

			createReadyCluster(cc)

			// Check C* TLS Secret fields
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraClusterTLSCA(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, caTLSSecret)).To(Succeed())
			Expect(caTLSSecret.Data).To(HaveKey("ca.crt"))
			Expect(caTLSSecret.Data).To(HaveKey("ca.key"))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraClusterTLSNode(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, nodeTLSSecret)).To(Succeed())
			Expect(nodeTLSSecret.Data).To(HaveKey("ca.crt"))
			Expect(nodeTLSSecret.Data).To(HaveKey("tls.crt"))
			Expect(nodeTLSSecret.Data).To(HaveKey("tls.key"))
			Expect(nodeTLSSecret.Data).To(HaveKey("keystore.p12"))
			Expect(nodeTLSSecret.Data).To(HaveKey("keystore.password"))
			Expect(nodeTLSSecret.Data).To(HaveKey("truststore.p12"))
			Expect(nodeTLSSecret.Data).To(HaveKey("truststore.password"))

			// Check C* configuration
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.ConfigMap(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, configConfigmap)).To(Succeed())
			cassandraYaml := make(map[string]interface{})
			err := yaml.Unmarshal([]byte(configConfigmap.Data["cassandra.yaml"]), &cassandraYaml)
			Expect(err).ToNot(HaveOccurred())

			encryptionOptions := cassandraYaml["server_encryption_options"].(map[string]interface{})
			Expect(encryptionOptions["keystore"]).To(BeEquivalentTo("/etc/cassandra-server-tls/keystore.p12"))
			Expect(encryptionOptions["keystore_password"]).To(BeEquivalentTo(keystorePass))
			Expect(encryptionOptions["truststore"]).To(BeEquivalentTo("/etc/cassandra-server-tls/truststore.p12"))
			Expect(encryptionOptions["truststore_password"]).To(BeEquivalentTo(keystorePass))
			Expect(encryptionOptions["protocol"]).To(BeEquivalentTo("TLS"))
			Expect(encryptionOptions["store_type"]).To(BeEquivalentTo("PKCS12"))

			checkVolume("server-keystore", "dc1")
		})

		It("when user provided cluster TLS CA Secret", func() {
			caTLSSecretName := "my-cluster-tls-ca"

			caTLSSecretData := make(map[string][]byte)
			caTLSSecretData["ca.crt"] = caCrtBytes
			caTLSSecretData["ca.key"] = caKeyBytes

			caTLSSecret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      caTLSSecretName,
					Namespace: cassandraObjectMeta.Namespace,
				},
				Data: caTLSSecretData,
				Type: v1.SecretTypeOpaque,
			}

			caDataSha1Sum := util.Sha1(fmt.Sprintf("%v", caTLSSecret.Data))

			Expect(k8sClient.Create(ctx, caTLSSecret)).To(Succeed())

			cc := baseCC.DeepCopy()
			cc.Spec.Encryption.Server = v1alpha1.ServerEncryption{
				InternodeEncryption: "dc",
				CATLSSecret: v1alpha1.CATLSSecret{
					Name: caTLSSecretName,
				},
			}

			createReadyCluster(cc)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: caTLSSecret.Name, Namespace: cassandraObjectMeta.Namespace}, caTLSSecret)).To(Succeed())
			Expect(caTLSSecret.Annotations[v1alpha1.CassandraClusterChecksum]).To(Equal(caDataSha1Sum))
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraClusterTLSNode(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, nodeTLSSecret)).To(Succeed())
			Expect(nodeTLSSecret.Data).To(HaveKey("ca.crt"))
			Expect(nodeTLSSecret.Data).To(HaveKey("tls.crt"))
			Expect(nodeTLSSecret.Data).To(HaveKey("tls.key"))
			Expect(nodeTLSSecret.Data).To(HaveKey("keystore.p12"))
			Expect(nodeTLSSecret.Data).To(HaveKey("keystore.password"))
			Expect(nodeTLSSecret.Data).To(HaveKey("truststore.p12"))
			Expect(nodeTLSSecret.Data).To(HaveKey("truststore.password"))

			checkVolume("server-keystore", "dc1")

			// Check hashsum after TLS CA Secret update
			caTLSSecret.Data["ca2.crt"] = ca2CrtBytes
			caDataSha1Sum = util.Sha1(fmt.Sprintf("%v", caTLSSecret.Data))
			Expect(k8sClient.Update(ctx, caTLSSecret)).To(Succeed())

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: caTLSSecret.Name, Namespace: cassandraObjectMeta.Namespace}, caTLSSecret)).To(Succeed())
				return caTLSSecret.Annotations[v1alpha1.CassandraClusterChecksum] == caDataSha1Sum
			}, longTimeout, shortRetry).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, caTLSSecret)).To(Succeed())
		})
		It("when user provided Cluster TLS Node Secret", func() {
			nodeTLSSecretName := "my-cluster-tls-node"

			nodeTLSSecretData := make(map[string][]byte)
			nodeTLSSecretData["ca.crt"] = caCrtBytes
			nodeTLSSecretData["tls.crt"] = tlsCrtBytes
			nodeTLSSecretData["tls.key"] = tlsKeyBytes
			nodeTLSSecretData["keystore.p12"] = keystoreBytes
			nodeTLSSecretData["truststore.p12"] = truststoreBytes
			nodeTLSSecretData["keystore.password"] = []byte(keystorePass)
			nodeTLSSecretData["truststore.password"] = []byte(keystorePass)

			nodeTLSSecret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nodeTLSSecretName,
					Namespace: cassandraObjectMeta.Namespace,
				},
				Data: nodeTLSSecretData,
				Type: v1.SecretTypeOpaque,
			}

			Expect(k8sClient.Create(ctx, nodeTLSSecret)).To(Succeed())

			cc := baseCC.DeepCopy()
			cc.Spec.Encryption.Server = v1alpha1.ServerEncryption{
				InternodeEncryption: "dc",
				NodeTLSSecret: v1alpha1.NodeTLSSecret{
					Name: nodeTLSSecretName,
				},
			}

			createReadyCluster(cc)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraClusterTLSNode(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, nodeTLSSecret))
			Expect(nodeTLSSecret.Data).To(HaveKey("ca.crt"))
			Expect(nodeTLSSecret.Data).To(HaveKey("tls.crt"))
			Expect(nodeTLSSecret.Data).To(HaveKey("tls.key"))
			Expect(nodeTLSSecret.Data).To(HaveKey("keystore.p12"))
			Expect(nodeTLSSecret.Data).To(HaveKey("keystore.password"))
			Expect(nodeTLSSecret.Data).To(HaveKey("truststore.p12"))
			Expect(nodeTLSSecret.Data).To(HaveKey("truststore.password"))

			checkVolume("server-keystore", "dc1")

			Expect(k8sClient.Delete(ctx, nodeTLSSecret)).To(Succeed())
		})
	})

	Context("when client encryption is enabled", func() {
		It("when user didn't provide Client TLS Secrets", func() {
			cc := baseCC.DeepCopy()
			cc.Spec.Encryption.Client = v1alpha1.ClientEncryption{
				Enabled: true,
			}

			createReadyCluster(cc)

			// Check C* TLS Secret fields
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraClientTLSCA(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, caTLSSecret)).To(Succeed())
			Expect(caTLSSecret.Data).To(HaveKey("ca.crt"))
			Expect(caTLSSecret.Data).To(HaveKey("ca.key"))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraClientTLSNode(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, nodeTLSSecret)).To(Succeed())
			Expect(nodeTLSSecret.Data).To(HaveKey("ca.crt"))
			Expect(nodeTLSSecret.Data).To(HaveKey("tls.crt"))
			Expect(nodeTLSSecret.Data).To(HaveKey("tls.key"))
			Expect(nodeTLSSecret.Data).To(HaveKey("keystore.p12"))
			Expect(nodeTLSSecret.Data).To(HaveKey("keystore.password"))
			Expect(nodeTLSSecret.Data).To(HaveKey("truststore.p12"))
			Expect(nodeTLSSecret.Data).To(HaveKey("truststore.password"))

			// Check C* configuration
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.ConfigMap(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, configConfigmap)).To(Succeed())
			cassandraYaml := make(map[string]interface{})
			err := yaml.Unmarshal([]byte(configConfigmap.Data["cassandra.yaml"]), &cassandraYaml)
			Expect(err).ToNot(HaveOccurred())

			encryptionOptions := cassandraYaml["client_encryption_options"].(map[string]interface{})
			Expect(encryptionOptions["enabled"]).To(BeTrue())
			Expect(encryptionOptions["optional"]).To(BeFalse())
			Expect(encryptionOptions["keystore"]).To(BeEquivalentTo("/etc/cassandra-client-tls/keystore.p12"))
			Expect(encryptionOptions["keystore_password"]).To(BeEquivalentTo(keystorePass))
			Expect(encryptionOptions["truststore"]).To(BeEquivalentTo("/etc/cassandra-client-tls/truststore.p12"))
			Expect(encryptionOptions["truststore_password"]).To(BeEquivalentTo(keystorePass))
			Expect(encryptionOptions["protocol"]).To(BeEquivalentTo("TLS"))
			Expect(encryptionOptions["store_type"]).To(BeEquivalentTo("PKCS12"))

			checkVolume("client-keystore", "dc1")
		})
		It("when user provided client TLS CA Secret", func() {
			cc := baseCC.DeepCopy()
			cc.Spec.Encryption.Client = v1alpha1.ClientEncryption{
				Enabled: true,
			}

			createReadyCluster(cc)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraClientTLSNode(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, nodeTLSSecret))
			Expect(nodeTLSSecret.Data).To(HaveKey("ca.crt"))
			Expect(nodeTLSSecret.Data).To(HaveKey("tls.crt"))
			Expect(nodeTLSSecret.Data).To(HaveKey("tls.key"))
			Expect(nodeTLSSecret.Data).To(HaveKey("keystore.p12"))
			Expect(nodeTLSSecret.Data).To(HaveKey("keystore.password"))
			Expect(nodeTLSSecret.Data).To(HaveKey("truststore.p12"))
			Expect(nodeTLSSecret.Data).To(HaveKey("truststore.password"))

			checkVolume("client-keystore", "dc1")
		})
		It("when user provided Client TLS Node Secret", func() {
			nodeTLSSecretName := "my-client-tls-node"

			nodeTLSSecretData := make(map[string][]byte)
			nodeTLSSecretData["ca.crt"] = caCrtBytes
			nodeTLSSecretData["tls.crt"] = tlsCrtBytes
			nodeTLSSecretData["tls.key"] = tlsKeyBytes
			nodeTLSSecretData["keystore.p12"] = keystoreBytes
			nodeTLSSecretData["truststore.p12"] = truststoreBytes
			nodeTLSSecretData["keystore.password"] = []byte(keystorePass)
			nodeTLSSecretData["truststore.password"] = []byte(keystorePass)

			nodeTLSSecret = &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nodeTLSSecretName,
					Namespace: cassandraObjectMeta.Namespace,
				},
				Data: nodeTLSSecretData,
				Type: v1.SecretTypeOpaque,
			}

			Expect(k8sClient.Create(ctx, nodeTLSSecret)).To(Succeed())

			cc := baseCC.DeepCopy()
			cc.Spec.Encryption.Client = v1alpha1.ClientEncryption{
				Enabled: true,
				NodeTLSSecret: v1alpha1.NodeTLSSecret{
					Name: nodeTLSSecretName,
				},
			}

			createReadyCluster(cc)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraClientTLSNode(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, nodeTLSSecret))
			Expect(nodeTLSSecret.Data).To(HaveKey("ca.crt"))
			Expect(nodeTLSSecret.Data).To(HaveKey("tls.crt"))
			Expect(nodeTLSSecret.Data).To(HaveKey("tls.key"))
			Expect(nodeTLSSecret.Data).To(HaveKey("keystore.p12"))
			Expect(nodeTLSSecret.Data).To(HaveKey("keystore.password"))
			Expect(nodeTLSSecret.Data).To(HaveKey("truststore.p12"))
			Expect(nodeTLSSecret.Data).To(HaveKey("truststore.password"))

			checkVolume("client-keystore", "dc1")

			Expect(k8sClient.Delete(ctx, nodeTLSSecret)).To(Succeed())
		})
	})
	Context("when both server and client encryption are enabled", func() {
		It("when user didn't provide both Cluster and Client TLS Secrets", func() {
			cc := baseCC.DeepCopy()
			cc.Spec.Encryption.Server = v1alpha1.ServerEncryption{
				InternodeEncryption: "dc",
			}
			cc.Spec.Encryption.Client = v1alpha1.ClientEncryption{
				Enabled: true,
			}

			createReadyCluster(cc)

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraClusterTLSCA(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, caTLSSecret)).To(Succeed())
			Expect(caTLSSecret.Data).To(HaveKey("ca.crt"))
			Expect(caTLSSecret.Data).To(HaveKey("ca.key"))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraClusterTLSNode(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, nodeTLSSecret)).To(Succeed())
			Expect(nodeTLSSecret.Data).To(HaveKey("ca.crt"))
			Expect(nodeTLSSecret.Data).To(HaveKey("tls.crt"))
			Expect(nodeTLSSecret.Data).To(HaveKey("tls.key"))
			Expect(nodeTLSSecret.Data).To(HaveKey("keystore.p12"))
			Expect(nodeTLSSecret.Data).To(HaveKey("keystore.password"))
			Expect(nodeTLSSecret.Data).To(HaveKey("truststore.p12"))
			Expect(nodeTLSSecret.Data).To(HaveKey("truststore.password"))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraClientTLSCA(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, caTLSSecret)).To(Succeed())
			Expect(caTLSSecret.Data).To(HaveKey("ca.crt"))
			Expect(caTLSSecret.Data).To(HaveKey("ca.key"))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.CassandraClientTLSNode(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, nodeTLSSecret))
			Expect(nodeTLSSecret.Data).To(HaveKey("ca.crt"))
			Expect(nodeTLSSecret.Data).To(HaveKey("tls.crt"))
			Expect(nodeTLSSecret.Data).To(HaveKey("tls.key"))
			Expect(nodeTLSSecret.Data).To(HaveKey("keystore.p12"))
			Expect(nodeTLSSecret.Data).To(HaveKey("keystore.password"))
			Expect(nodeTLSSecret.Data).To(HaveKey("truststore.p12"))
			Expect(nodeTLSSecret.Data).To(HaveKey("truststore.password"))

			checkVolume("server-keystore", "dc1")
			checkVolume("client-keystore", "dc1")
		})
	})
})

func checkVolume(volumeName string, dc string) {
	sts := &appsv1.StatefulSet{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.DC(cassandraObjectMeta.Name, dc), Namespace: cassandraObjectMeta.Namespace}, sts)).To(Succeed())

	//cassandraContainer, found := getContainerByName(sts.Spec.Template.Spec, keystorePass)
	//Expect(found).To(BeTrue())

	//_, found = getVolumeByName(sts.Spec.Template.Spec.Volumes, volumeName)
	//Expect(found).To(BeTrue())

	//_, found = getVolumeMountByName(cassandraContainer.VolumeMounts, volumeName)
	//Expect(found).To(BeTrue())
}
