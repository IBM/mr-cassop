package integration

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
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

	tlsSecret := &v1.Secret{}
	configConfigmap := &v1.ConfigMap{}

	Context("when server encryption is enabled", func() {
		It("server TLS Secret should be used", func() {

			tlsSecretName := "cassandra-cluster-tls"

			tlsSecretData := make(map[string][]byte)
			tlsSecretData["keystore.jks"] = []byte("keystore.jks")
			tlsSecretData["keystore.password"] = []byte("somePassword")
			tlsSecretData["truststore.jks"] = []byte("truststore.jks")
			tlsSecretData["truststore.password"] = []byte("somePassword")

			tlsSecret = &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tlsSecretName,
					Namespace: cassandraObjectMeta.Namespace,
				},
				Data: tlsSecretData,
			}

			Expect(k8sClient.Create(ctx, tlsSecret)).To(Succeed())

			cc := baseCC.DeepCopy()
			cc.Spec.Encryption.Server = v1alpha1.ServerEncryption{
				InternodeEncryption: "dc",
				TLSSecret: v1alpha1.TLSSecret{
					Name: tlsSecretName,
				},
			}

			createReadyCluster(cc)

			// Check C* configuration
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.ConfigMap(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, configConfigmap)).To(Succeed())
			cassandraYaml := make(map[string]interface{})
			err := yaml.Unmarshal([]byte(configConfigmap.Data["cassandra.yaml"]), &cassandraYaml)
			Expect(err).ToNot(HaveOccurred())

			encryptionOptions := cassandraYaml["server_encryption_options"].(map[string]interface{})
			Expect(encryptionOptions["keystore"]).To(BeEquivalentTo("/etc/cassandra-server-tls/keystore.jks"))
			Expect(encryptionOptions["keystore"]).To(BeEquivalentTo("/etc/cassandra-server-tls/keystore.jks"))
			Expect(encryptionOptions["keystore_password"]).To(BeEquivalentTo("somePassword"))
			Expect(encryptionOptions["truststore"]).To(BeEquivalentTo("/etc/cassandra-server-tls/truststore.jks"))
			Expect(encryptionOptions["truststore_password"]).To(BeEquivalentTo("somePassword"))
			Expect(encryptionOptions["protocol"]).To(BeEquivalentTo("TLS"))
			Expect(encryptionOptions["store_type"]).To(BeEquivalentTo("JKS"))

			checkVolume("server-keystore", "dc1")
		})
	})

	Context("when client encryption is enabled", func() {
		It("client TLS Secret should be used", func() {

			tlsSecretName := "cassandra-client-tls"

			tlsSecretData := make(map[string][]byte)
			tlsSecretData["keystore.jks"] = []byte("keystore.jks")
			tlsSecretData["keystore.password"] = []byte("somePassword")
			tlsSecretData["truststore.jks"] = []byte("truststore.jks")
			tlsSecretData["truststore.password"] = []byte("somePassword")
			tlsSecretData["ca.crt"] = []byte("some_data")
			tlsSecretData["tls.crt"] = []byte("some_data")
			tlsSecretData["tls.key"] = []byte("some_data")

			tlsSecret = &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tlsSecretName,
					Namespace: cassandraObjectMeta.Namespace,
				},
				Data: tlsSecretData,
			}

			Expect(k8sClient.Create(ctx, tlsSecret)).To(Succeed())

			cc := baseCC.DeepCopy()
			cc.Spec.Encryption.Client = v1alpha1.ClientEncryption{
				Enabled: true,
				TLSSecret: v1alpha1.ClientTLSSecret{
					TLSSecret: v1alpha1.TLSSecret{
						Name: tlsSecretName,
					},
				},
			}

			createReadyCluster(cc)

			// Check C* configuration
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.ConfigMap(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, configConfigmap)).To(Succeed())
			cassandraYaml := make(map[string]interface{})
			err := yaml.Unmarshal([]byte(configConfigmap.Data["cassandra.yaml"]), &cassandraYaml)
			Expect(err).ToNot(HaveOccurred())

			encryptionOptions := cassandraYaml["client_encryption_options"].(map[string]interface{})
			Expect(encryptionOptions["enabled"]).To(BeTrue())
			Expect(encryptionOptions["optional"]).To(BeFalse())
			Expect(encryptionOptions["keystore"]).To(BeEquivalentTo("/etc/cassandra-client-tls/keystore.jks"))
			Expect(encryptionOptions["keystore_password"]).To(BeEquivalentTo("somePassword"))
			Expect(encryptionOptions["truststore"]).To(BeEquivalentTo("/etc/cassandra-client-tls/truststore.jks"))
			Expect(encryptionOptions["truststore_password"]).To(BeEquivalentTo("somePassword"))
			Expect(encryptionOptions["protocol"]).To(BeEquivalentTo("TLS"))
			Expect(encryptionOptions["store_type"]).To(BeEquivalentTo("JKS"))

			checkVolume("client-keystore", "dc1")
		})
	})

	var _ = AfterEach(func() {
		Expect(k8sClient.Delete(ctx, tlsSecret)).To(Succeed())
	})
})

func checkVolume(volumeName string, dc string) {
	sts := &appsv1.StatefulSet{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.DC(cassandraObjectMeta.Name, dc), Namespace: cassandraObjectMeta.Namespace}, sts)).To(Succeed())

	cassandraContainer, found := getContainerByName(sts.Spec.Template.Spec, "cassandra")
	Expect(found).To(BeTrue())

	_, found = getVolumeByName(sts.Spec.Template.Spec.Volumes, volumeName)
	Expect(found).To(BeTrue())

	_, found = getVolumeMountByName(cassandraContainer.VolumeMounts, volumeName)
	Expect(found).To(BeTrue())
}
