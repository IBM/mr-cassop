package integration

import (
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/names"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("secure admin account", func() {
	Context("on cluster init", func() {
		It("should be created", func() {
			cc := &v1alpha1.CassandraCluster{
				ObjectMeta: cassandraObjectMeta,
				Spec: v1alpha1.CassandraClusterSpec{
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(3),
						},
					},
					ImagePullSecretName: "pull-secret-name",
					AdminRoleSecretName: "admin-role",
				},
			}

			createAdminSecret(cc)
			Expect(k8sClient.Create(ctx, cc)).To(Succeed())

			By("initializing a new cluster")
			activeAdminSecret := &v1.Secret{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: names.ActiveAdminSecret(cc.Name), Namespace: cc.Namespace}, activeAdminSecret)
			}, mediumTimeout, mediumRetry).Should(Succeed())

			Expect(activeAdminSecret.Data).To(And(
				HaveKeyWithValue("admin-role", []byte("cassandra")),
				HaveKeyWithValue("admin-password", []byte("cassandra")),
			))

			authConfigSecret := &v1.Secret{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: names.AdminAuthConfigSecret(cc.Name)}, authConfigSecret)
			}, mediumTimeout, mediumRetry).Should(Succeed())
			Expect(authConfigSecret.Data).To(And(
				HaveKeyWithValue("admin-role", []byte("cassandra")),
				HaveKeyWithValue("admin-password", []byte("cassandra")),
				HaveKeyWithValue("cqlshrc", []byte(`
[authentication]
username = cassandra
password = cassandra
[connection]
hostname = 127.0.0.1
port = 9042
`)),
			))

			markMocksAsReady(cc)
			waitForDCsToBeCreated(cc)
			markAllDCsReady(cc)
			createCassandraPods(cc)

			authRoleSecret := &v1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cc.Spec.AdminRoleSecretName, Namespace: cc.Namespace}, authRoleSecret)).To(Succeed())

			Eventually(func() map[string][]byte {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: names.AdminAuthConfigSecret(cc.Name)}, authConfigSecret)).To(Succeed())
				return authConfigSecret.Data
			}, mediumTimeout, mediumRetry).Should(And(
				HaveKeyWithValue("admin-role", authRoleSecret.Data["admin-role"]),
				HaveKeyWithValue("admin-password", authRoleSecret.Data["admin-password"]),
				HaveKeyWithValue("cqlshrc", []byte(fmt.Sprintf(`
[authentication]
username = %s
password = %s
[connection]
hostname = 127.0.0.1
port = 9042
`, string(authRoleSecret.Data["admin-role"]), string(authRoleSecret.Data["admin-password"])))),
			))

			roles, err := mockCQLClient.GetRoles()
			Expect(err).ToNot(HaveOccurred())
			Expect(roles).To(ContainElements(cql.Role{
				Role:     string(authRoleSecret.Data["admin-role"]),
				Login:    true,
				Super:    true,
				Password: string(authRoleSecret.Data["admin-password"]),
			}))

			By("changing the password")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: cc.Spec.AdminRoleSecretName}, authRoleSecret)).To(Succeed())
			authRoleSecret.Data["admin-password"] = []byte("new-password")
			Expect(k8sClient.Update(ctx, authRoleSecret)).To(Succeed())

			Eventually(func() map[string][]byte {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: names.AdminAuthConfigSecret(cc.Name)}, authConfigSecret)).To(Succeed())
				return authConfigSecret.Data
			}, mediumTimeout, mediumRetry).Should(And(
				HaveKeyWithValue("admin-role", authRoleSecret.Data["admin-role"]),
				HaveKeyWithValue("admin-password", []byte("new-password")),
				HaveKeyWithValue("cqlshrc", []byte(fmt.Sprintf(`
[authentication]
username = %s
password = %s
[connection]
hostname = 127.0.0.1
port = 9042
`, string(authRoleSecret.Data["admin-role"]), "new-password"))),
			))

			Eventually(func() map[string][]byte {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: names.ActiveAdminSecret(cc.Name)}, activeAdminSecret)).To(Succeed())
				return activeAdminSecret.Data
			}, mediumTimeout, mediumRetry).Should(And(
				HaveKeyWithValue("admin-role", authRoleSecret.Data["admin-role"]),
				HaveKeyWithValue("admin-password", []byte("new-password")),
			))

			roles, err = mockCQLClient.GetRoles()
			Expect(err).ToNot(HaveOccurred())
			Expect(roles).To(ContainElements(cql.Role{
				Role:     string(authRoleSecret.Data["admin-role"]),
				Login:    true,
				Super:    true,
				Password: string(authRoleSecret.Data["admin-password"]),
			}))

			By("create a new admin role")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: cc.Spec.AdminRoleSecretName}, authRoleSecret)).To(Succeed())
			authRoleSecret.Data["admin-role"] = []byte("new-role")
			authRoleSecret.Data["admin-password"] = []byte("new-password")
			Expect(k8sClient.Update(ctx, authRoleSecret)).To(Succeed())

			Eventually(func() map[string][]byte {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: names.AdminAuthConfigSecret(cc.Name)}, authConfigSecret)).To(Succeed())
				return authConfigSecret.Data
			}, mediumTimeout, mediumRetry).Should(And(
				HaveKeyWithValue("admin-role", []byte("new-role")),
				HaveKeyWithValue("admin-password", []byte("new-password")),
				HaveKeyWithValue("cqlshrc", []byte(fmt.Sprintf(`
[authentication]
username = %s
password = %s
[connection]
hostname = 127.0.0.1
port = 9042
`, "new-role", "new-password"))),
			))

			Eventually(func() map[string][]byte {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: names.ActiveAdminSecret(cc.Name)}, activeAdminSecret)).To(Succeed())
				return activeAdminSecret.Data
			}, mediumTimeout, mediumRetry).Should(And(
				HaveKeyWithValue("admin-role", []byte("new-role")),
				HaveKeyWithValue("admin-password", []byte("new-password")),
			))

			roles, err = mockCQLClient.GetRoles()
			Expect(err).ToNot(HaveOccurred())
			Expect(roles).To(ContainElements(cql.Role{
				Role:     string(authRoleSecret.Data["admin-role"]),
				Login:    true,
				Super:    true,
				Password: string(authRoleSecret.Data["admin-password"]),
			}))
		})
	})
})
