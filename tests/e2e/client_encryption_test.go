package e2e

import (
	"encoding/base64"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("TLS Encryption test", func() {
	ccName := "client-tls"

	AfterEach(func() {
		cleanupResources(ccName, cfg.operatorNamespace)
	})

	Context("if Client TLS CA provided", func() {
		It("cluster should start", func() {
			testAdminRole := "alice"
			testAdminPassword := "testpassword"

			testAdminRoleSecretName := ccName + "-admin-role"
			caTLSSecretName := ccName + "-ca"

			cc := newCassandraClusterTmpl(ccName, cfg.operatorNamespace)
			createSecret(cc.Namespace, testAdminRoleSecretName, map[string][]byte{
				dbv1alpha1.CassandraOperatorAdminRole:     []byte(testAdminRole),
				dbv1alpha1.CassandraOperatorAdminPassword: []byte(testAdminPassword),
			})

			cc.Spec.AdminRoleSecretName = testAdminRoleSecretName

			cc.Spec.Encryption.Client = dbv1alpha1.ClientEncryption{
				Enabled: true,
				CATLSSecret: dbv1alpha1.CATLSSecret{
					Name: caTLSSecretName,
				},
			}

			caCrtBytes, _ := base64.StdEncoding.DecodeString(caCrtEncoded)
			caKeyBytes, _ := base64.StdEncoding.DecodeString(caKeyEncoded)

			caTLSSecret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      caTLSSecretName,
					Namespace: cfg.operatorNamespace,
				},
				Data: map[string][]byte{
					"ca.crt": caCrtBytes,
					"ca.key": caKeyBytes,
				},
			}

			Expect(kubeClient.Create(ctx, caTLSSecret)).To(Succeed())
			deployCassandraCluster(cc)
		})
	})
})
