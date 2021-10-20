package integration

import (
	"fmt"
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Cassandra Server TLS encryption", func() {
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

	serverTLSSecretName := "cassandra-cluster-tls"

	tlsSecretData := make(map[string][]byte)
	tlsSecretData["keystore.jks"] = []byte("keystore.jks")
	tlsSecretData["keystore.password"] = []byte("somePassword")
	tlsSecretData["truststore.jks"] = []byte("truststore.jks")
	tlsSecretData["truststore.password"] = []byte("somePassword")

	baseTLSSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverTLSSecretName,
			Namespace: cassandraObjectMeta.Namespace,
		},
		Data: tlsSecretData,
	}

	Context("when server encryption is enabled", func() {
		It("server TLS Secret should be used", func() {
			serverTLSSecret := baseTLSSecret.DeepCopy()
			Expect(k8sClient.Create(ctx, serverTLSSecret)).To(Succeed())

			cc := baseCC.DeepCopy()
			cc.Spec.Encryption.Server = v1alpha1.Server{
				InternodeEncryption: "dc",
				TLSSecret: v1alpha1.TLSSecret{
					Name: serverTLSSecretName,
				},
			}

			createReadyCluster(cc)

			// Check C* configuration
			configConfigmap := &v1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.ConfigMap(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, configConfigmap)).To(Succeed())
			cassandraYaml := make(map[string]interface{})
			err := yaml.Unmarshal([]byte(configConfigmap.Data["cassandra.yaml"]), &cassandraYaml)
			Expect(err).ToNot(HaveOccurred())

			serverEncryptionOptions := cassandraYaml["server_encryption_options"].(map[string]interface{})
			Expect(serverEncryptionOptions["keystore"]).To(BeEquivalentTo("/etc/cassandra-server-tls/keystore.jks"))
			Expect(serverEncryptionOptions["keystore_password"]).To(BeEquivalentTo("somePassword"))
			Expect(serverEncryptionOptions["truststore"]).To(BeEquivalentTo("/etc/cassandra-server-tls/truststore.jks"))
			Expect(serverEncryptionOptions["truststore_password"]).To(BeEquivalentTo("somePassword"))
			Expect(serverEncryptionOptions["protocol"]).To(BeEquivalentTo("TLS"))
			Expect(serverEncryptionOptions["store_type"]).To(BeEquivalentTo("JKS"))

			checkTLSChecksum(cc, serverTLSSecretName, "", "SERVER_TLS_SHA1")

			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.DC(cc.Name, "dc1"), Namespace: cc.Namespace}, sts)).To(Succeed())

			cassandraContainer, found := getContainerByName(sts.Spec.Template.Spec, "cassandra")
			Expect(found).To(BeTrue())

			_, found = getVolumeByName(sts.Spec.Template.Spec.Volumes, "server-keystore")
			Expect(found).To(BeTrue())

			_, found = getVolumeMountByName(cassandraContainer.VolumeMounts, "server-keystore")
			Expect(found).To(BeTrue())

			When("TLS secret is changed", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverTLSSecretName, Namespace: cassandraObjectMeta.Namespace}, serverTLSSecret)).To(Succeed())
				data := serverTLSSecret.Data
				data["keystore.password"] = []byte("someChangedPassword")
				serverTLSSecret.Data = data
				Expect(k8sClient.Update(ctx, serverTLSSecret)).To(Succeed())

				checksum := util.Sha1(fmt.Sprintf("%v", serverTLSSecret.Data))
				checkTLSChecksum(cc, serverTLSSecretName, checksum, "SERVER_TLS_SHA1")
			})
		})
	})

	var _ = AfterEach(func() {
		Expect(k8sClient.Delete(ctx, baseTLSSecret)).To(Succeed())
	})
})

func checkTLSChecksum(cc *v1alpha1.CassandraCluster, secretName string, checksum string, envName string) {
	sts := &appsv1.StatefulSet{}
	tlsSecret := &v1.Secret{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cassandraObjectMeta.Namespace}, tlsSecret)).To(Succeed())
	if checksum == "" {
		checksum = util.Sha1(fmt.Sprintf("%v", tlsSecret.Data))
	}

	Eventually(func() string {
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.DC(cc.Name, "dc1"), Namespace: cc.Namespace}, sts)).To(Succeed())
		for _, env := range sts.Spec.Template.Spec.Containers[0].Env {
			if env.Name == envName {
				return env.Value
			}
		}
		return ""
	}, mediumTimeout, shortRetry).Should(BeEquivalentTo(checksum))
}
