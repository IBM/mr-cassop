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

var _ = Describe("pod restart checksum", func() {
	clusterNodeTLSSecretName := "my-cluster-tls-node"
	clientNodeTLSSecretName := "my-client-tls-node"

	cc := &v1alpha1.CassandraCluster{
		ObjectMeta: cassandraObjectMeta,
		Spec: v1alpha1.CassandraClusterSpec{
			DCs: []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(3),
				},
			},
			Encryption: v1alpha1.Encryption{
				Server: v1alpha1.ServerEncryption{
					InternodeEncryption: "dc",
					NodeTLSSecret: v1alpha1.NodeTLSSecret{
						Name: clusterNodeTLSSecretName,
					},
				},
				Client: v1alpha1.ClientEncryption{
					Enabled: true,
					NodeTLSSecret: v1alpha1.NodeTLSSecret{
						Name: clientNodeTLSSecretName,
					},
				},
			},
			AdminRoleSecretName: "admin-role",
			ImagePullSecretName: "pull-secret-name",
		},
	}

	nodeTLSSecretData := make(map[string][]byte)
	nodeTLSSecretData["ca.crt"] = caCrtBytes
	nodeTLSSecretData["tls.crt"] = tlsCrtBytes
	nodeTLSSecretData["tls.key"] = tlsKeyBytes
	nodeTLSSecretData["keystore.p12"] = keystoreBytes
	nodeTLSSecretData["truststore.p12"] = truststoreBytes
	nodeTLSSecretData["keystore.password"] = []byte(keystorePass)
	nodeTLSSecretData["truststore.password"] = []byte(keystorePass)

	clusterNodeTLSSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterNodeTLSSecretName,
			Namespace: cassandraObjectMeta.Namespace,
		},
		Data: nodeTLSSecretData,
	}

	clientNodeTLSSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientNodeTLSSecretName,
			Namespace: cassandraObjectMeta.Namespace,
		},
		Data: nodeTLSSecretData,
	}

	Context("in cassandra statefulset", func() {
		It("should change depending on resource change", func() {
			Expect(k8sClient.Delete(ctx, clusterNodeTLSSecret)) // Todo: figure out how come it's already created
			Expect(k8sClient.Delete(ctx, clientNodeTLSSecret))
			Expect(k8sClient.Create(ctx, clusterNodeTLSSecret)).To(Succeed())
			Expect(k8sClient.Create(ctx, clientNodeTLSSecret)).To(Succeed())

			createReadyCluster(cc)

			// Check C* configuration
			cassandraConfig := &v1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.ConfigMap(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, cassandraConfig)).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: clusterNodeTLSSecretName, Namespace: cc.Namespace}, clusterNodeTLSSecret))
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: clientNodeTLSSecretName, Namespace: cc.Namespace}, clientNodeTLSSecret))

			checksumContainer := map[string]string{
				"cassandra.yaml":          cassandraConfig.Data["cassandra.yaml"],
				"cluster-node-tls-secret": fmt.Sprintf("%v", clusterNodeTLSSecret.Data),
				"client-node-tls-secret":  fmt.Sprintf("%v", clientNodeTLSSecret.Data),
			}

			initialChecksum := util.Sha1(fmt.Sprintf("%v", checksumContainer))

			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: names.DC(cassandraObjectMeta.Name, cc.Spec.DCs[0].Name), Namespace: cassandraObjectMeta.Namespace}, sts)
			}).Should(Succeed())

			Expect(sts.Spec.Template.Spec.Containers[0].Env).To(ContainElement(v1.EnvVar{Name: "POD_RESTART_CHECKSUM", Value: initialChecksum}))

			By("client Node TLS Secret update should change the checksum")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: clientNodeTLSSecretName, Namespace: cc.Namespace}, clientNodeTLSSecret)).To(Succeed())
			clientNodeTLSSecret.Data["ca.crt"] = []byte("changed-data")

			checksumContainer["client-node-tls-secret"] = fmt.Sprintf("%v", clientNodeTLSSecret.Data)
			checksum := util.Sha1(fmt.Sprintf("%v", checksumContainer))

			Expect(k8sClient.Update(ctx, clientNodeTLSSecret)).To(Succeed())

			Eventually(func() []v1.EnvVar {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.DC(cassandraObjectMeta.Name, cc.Spec.DCs[0].Name), Namespace: cassandraObjectMeta.Namespace}, sts)).To(Succeed())
				return sts.Spec.Template.Spec.Containers[0].Env
			}, mediumTimeout, mediumRetry).Should(ContainElement(v1.EnvVar{Name: "POD_RESTART_CHECKSUM", Value: checksum}))

			By("server TLS secret update should change the checksum")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.ConfigMap(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, cassandraConfig)).To(Succeed())
			cassandraYaml := make(map[string]interface{})
			err := yaml.Unmarshal([]byte(cassandraConfig.Data["cassandra.yaml"]), &cassandraYaml)
			Expect(err).ToNot(HaveOccurred())
			cassandraYaml["server_encryption_options"].(map[string]interface{})["keystore_password"] = "changed-data"
			cassandraYamlBytes, err := yaml.Marshal(cassandraYaml)
			Expect(err).ToNot(HaveOccurred())
			cassandraConfig.Data["cassandra.yaml"] = string(cassandraYamlBytes)

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: clusterNodeTLSSecretName, Namespace: cc.Namespace}, clusterNodeTLSSecret)).To(Succeed())
			clusterNodeTLSSecret.Data["keystore.password"] = []byte("changed-data")
			checksumContainer["cluster-node-tls-secret"] = fmt.Sprintf("%v", clusterNodeTLSSecret.Data)
			checksumContainer["cassandra.yaml"] = string(cassandraYamlBytes)
			checksum = util.Sha1(fmt.Sprintf("%v", checksumContainer))
			Expect(k8sClient.Update(ctx, clusterNodeTLSSecret)).To(Succeed())

			Eventually(func() []v1.EnvVar {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.DC(cassandraObjectMeta.Name, cc.Spec.DCs[0].Name), Namespace: cassandraObjectMeta.Namespace}, sts)).To(Succeed())
				return sts.Spec.Template.Spec.Containers[0].Env
			}, mediumTimeout, mediumRetry).Should(ContainElement(v1.EnvVar{Name: "POD_RESTART_CHECKSUM", Value: checksum}))

		})
	})

	var _ = AfterEach(func() {
		Expect(k8sClient.Delete(ctx, clusterNodeTLSSecret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, clientNodeTLSSecret)).To(Succeed())
	})
})
