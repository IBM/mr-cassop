package integration

import (
	"fmt"

	"github.com/ibm/cassandra-operator/controllers/util"

	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("user roles", func() {
	rolesSecretName := "cassandra-roles"
	baseCC := &v1alpha1.CassandraCluster{
		ObjectMeta: cassandraObjectMeta,
		Spec: v1alpha1.CassandraClusterSpec{
			DCs: []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(3),
				},
			},
			AdminRoleSecretName: "admin-role",
			ImagePullSecretName: "imagePullSecret",
			RolesSecretName:     rolesSecretName,
		},
	}

	baseRolesSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rolesSecretName,
			Namespace: cassandraObjectMeta.Namespace,
		},
	}

	Context("with existing secret with valid users", func() {
		It("should create users in cassandra", func() {
			rolesSecretData := map[string][]byte{
				"alice": []byte(`{"password": "foo", "login": true, "super": true}`),
				"bob":   []byte(`{"password": "bar"}`),
				"eve":   []byte(`{"super": false}`), //should ignore but not fail (no password)
				"sam":   []byte(`bad json`),         //should ignore but not fail
			}
			createReadyCluster(baseCC.DeepCopy())
			rolesSecret := baseRolesSecret.DeepCopy()
			rolesSecret.Data = rolesSecretData
			Expect(k8sClient.Create(ctx, rolesSecret)).To(Succeed())

			Eventually(func() []cql.Role {
				roles, err := mockCQLClient.GetRoles()
				Expect(err).ToNot(HaveOccurred())
				return roles
			}, mediumTimeout, mediumRetry).Should(ContainElements(
				cql.Role{
					Role:     "alice",
					Super:    true,
					Login:    true,
					Password: "foo",
				},
				cql.Role{
					Role:     "bob",
					Super:    false,
					Login:    true,
					Password: "bar",
				},
			))
			rolesSecretDataChecksum := util.Sha1(fmt.Sprintf("%v", rolesSecretData))
			updatedSecret := &v1.Secret{}
			Eventually(func() []string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: rolesSecret.Name, Namespace: rolesSecret.Namespace}, updatedSecret)
				Expect(err).ToNot(HaveOccurred())
				return []string{updatedSecret.Annotations[v1alpha1.CassandraClusterChecksum], updatedSecret.Annotations[v1alpha1.CassandraClusterInstance]}
			}).Should(ContainElements(rolesSecretDataChecksum, baseCC.Name))

			rolesSecretData["alice"] = []byte(`{"password": "newpassword", "login": false, "super": false}`)
			updatedSecret.Data = rolesSecretData
			Expect(k8sClient.Update(ctx, updatedSecret)).To(Succeed())
			Eventually(func() []cql.Role {
				roles, err := mockCQLClient.GetRoles()
				Expect(err).ToNot(HaveOccurred())
				return roles
			}, mediumTimeout, mediumRetry).Should(ContainElements(
				cql.Role{
					Role:     "alice",
					Super:    false,
					Login:    false,
					Password: "newpassword",
				},
				cql.Role{
					Role:     "bob",
					Super:    false,
					Login:    true,
					Password: "bar",
				},
			))

			By("role marked for removal should be deleted")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: rolesSecret.Name, Namespace: rolesSecret.Namespace}, updatedSecret)).To(Succeed())
			updatedSecret.Data["alice"] = []byte(`{"password": "newpassword", "login": false, "super": false, "delete": true}`)
			Expect(k8sClient.Update(ctx, updatedSecret)).To(Succeed())
			Eventually(func() []cql.Role {
				roles, err := mockCQLClient.GetRoles()
				Expect(err).ToNot(HaveOccurred())
				return roles
			}, mediumTimeout, mediumRetry).Should(ContainElements(
				cql.Role{
					Role:     "bob",
					Super:    false,
					Login:    true,
					Password: "bar",
				},
			))
		})
	})

	var _ = AfterEach(func() {
		Expect(k8sClient.Delete(ctx, baseRolesSecret)).To(Succeed())
	})
})
