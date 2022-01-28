package integration

import (
	"fmt"

	"sigs.k8s.io/yaml"

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
)

var _ = Describe("pod restart checksum", func() {
	serverTLSSecretName := "cassandra-cluster-server-tls"
	clientTLSSecretName := "cassandra-cluster-client-tls"

	tlsTestData := map[string][]byte{
		"keystore.jks":        []byte("keystore.jks"),
		"keystore.password":   []byte("somePassword"),
		"truststore.jks":      []byte("truststore.jks"),
		"truststore.password": []byte("somePassword"),
		"ca.crt":              []byte("some_data"),
		"tls.crt":             []byte("some_data"),
		"tls.key":             []byte("some_data"),
	}

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
					TLSSecret: v1alpha1.TLSSecret{
						Name: serverTLSSecretName,
					},
				},
				Client: v1alpha1.ClientEncryption{
					Enabled: true,
					TLSSecret: v1alpha1.ClientTLSSecret{
						TLSSecret: v1alpha1.TLSSecret{
							Name: clientTLSSecretName,
						},
					},
				},
			},
			AdminRoleSecretName: "admin-role",
			ImagePullSecretName: "pull-secret-name",
		},
	}

	serverTLSSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverTLSSecretName,
			Namespace: cassandraObjectMeta.Namespace,
		},
		Data: tlsTestData,
	}

	clientTLSSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientTLSSecretName,
			Namespace: cassandraObjectMeta.Namespace,
		},
		Data: tlsTestData,
	}

	Context("in cassandra statefulset", func() {
		It("should change depending on resource change", func() {
			Expect(k8sClient.Create(ctx, serverTLSSecret)).To(Succeed())
			Expect(k8sClient.Create(ctx, clientTLSSecret)).To(Succeed())

			createReadyCluster(cc)

			// Check C* configuration
			cassandraConfig := &v1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.ConfigMap(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, cassandraConfig)).To(Succeed())

			checksumContainer := map[string]string{
				"cassandra.yaml":    cassandraConfig.Data["cassandra.yaml"],
				"server-tls-secret": fmt.Sprintf("%v", serverTLSSecret.Data),
				"client-tls-secret": fmt.Sprintf("%v", clientTLSSecret.Data),
			}

			initialChecksum := util.Sha1(fmt.Sprintf("%v", checksumContainer))

			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: names.DC(cassandraObjectMeta.Name, cc.Spec.DCs[0].Name), Namespace: cassandraObjectMeta.Namespace}, sts)
			}).Should(Succeed())

			Expect(sts.Spec.Template.Spec.Containers[0].Env).To(ContainElement(v1.EnvVar{Name: "POD_RESTART_CHECKSUM", Value: initialChecksum}))

			By("client TLS secret update should change the checksum")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: clientTLSSecretName, Namespace: cc.Namespace}, clientTLSSecret)).To(Succeed())
			clientTLSSecret.Data["ca.crt"] = []byte("changed-data")
			checksumContainer["client-tls-secret"] = fmt.Sprintf("%v", clientTLSSecret.Data)
			checksum := util.Sha1(fmt.Sprintf("%v", checksumContainer))
			Expect(k8sClient.Update(ctx, clientTLSSecret)).To(Succeed())

			Eventually(func() []v1.EnvVar {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.DC(cassandraObjectMeta.Name, cc.Spec.DCs[0].Name), Namespace: cassandraObjectMeta.Namespace}, sts)).To(Succeed())
				return sts.Spec.Template.Spec.Containers[0].Env
			}, mediumTimeout, mediumRetry).Should(ContainElement(v1.EnvVar{Name: "POD_RESTART_CHECKSUM", Value: checksum}))

			By("server TLS secret update should change the checksum")

			cassandraConfig = &v1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.ConfigMap(cassandraObjectMeta.Name), Namespace: cassandraObjectMeta.Namespace}, cassandraConfig)).To(Succeed())
			cassandraYaml := make(map[string]interface{})
			err := yaml.Unmarshal([]byte(cassandraConfig.Data["cassandra.yaml"]), &cassandraYaml)
			Expect(err).ToNot(HaveOccurred())
			cassandraYaml["server_encryption_options"].(map[string]interface{})["keystore_password"] = "changed-data"
			cassandraYamlBytes, err := yaml.Marshal(cassandraYaml)
			Expect(err).ToNot(HaveOccurred())
			cassandraConfig.Data["cassandra.yaml"] = string(cassandraYamlBytes)

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverTLSSecretName, Namespace: cc.Namespace}, serverTLSSecret)).To(Succeed())
			serverTLSSecret.Data["keystore.password"] = []byte("changed-data")
			checksumContainer["server-tls-secret"] = fmt.Sprintf("%v", serverTLSSecret.Data)
			checksumContainer["cassandra.yaml"] = string(cassandraYamlBytes)
			checksum = util.Sha1(fmt.Sprintf("%v", checksumContainer))
			Expect(k8sClient.Update(ctx, serverTLSSecret)).To(Succeed())

			Eventually(func() []v1.EnvVar {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: names.DC(cassandraObjectMeta.Name, cc.Spec.DCs[0].Name), Namespace: cassandraObjectMeta.Namespace}, sts)).To(Succeed())
				return sts.Spec.Template.Spec.Containers[0].Env
			}, mediumTimeout, mediumRetry).Should(ContainElement(v1.EnvVar{Name: "POD_RESTART_CHECKSUM", Value: checksum}))

		})
	})

	var _ = AfterEach(func() {
		Expect(k8sClient.Delete(ctx, clientTLSSecret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, serverTLSSecret)).To(Succeed())
	})
})
