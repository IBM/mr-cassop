package e2e

import (
	"context"
	"fmt"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("user provided roles", func() {
	Context("if a valid secret provided", func() {
		It("should be created", func() {
			rolesSecretName := cassandraCluster.Name + "-cassandra-roles"
			testUserName := "alice"
			testUserPassword := "testpassword"
			newCassandraCluster := cassandraCluster.DeepCopy()
			newCassandraCluster.Spec.RolesSecretName = rolesSecretName
			rolesSecret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rolesSecretName,
					Namespace: cassandraObjectMeta.Namespace,
				},
				Data: map[string][]byte{
					testUserName: []byte(fmt.Sprintf(`{"password": "%s", "login": true, "super": true}`, testUserPassword)),
				},
			}

			Expect(restClient.Create(context.Background(), rolesSecret)).To(Succeed())
			defer func() { Expect(restClient.Delete(context.Background(), rolesSecret)).To(Succeed()) }()

			deployCassandraCluster(newCassandraCluster)

			podList := &v1.PodList{}
			err = restClient.List(context.Background(), podList, client.InNamespace(cassandraNamespace), client.MatchingLabels(cassandraClusterPodLabels))
			Expect(err).ToNot(HaveOccurred())

			Expect(len(podList.Items)).Should(BeNumerically(">", 0))
			pod := podList.Items[0]

			cmd := []string{
				"sh",
				"-c",
				fmt.Sprintf("cqlsh -u %s -p \"%s\" -e \"SHOW HOST\"", testUserName, testUserPassword),
			}

			stdout := ""
			Eventually(func() error {
				execResult, err := execPod(pod.Name, pod.Namespace, cmd)
				stdout = execResult.stdout
				return err
			}, 30*time.Second, 5*time.Second).Should(Succeed())
			Expect(stdout).To(ContainSubstring(fmt.Sprintf("Connected to %s at %s:%d.", cassandraRelease, "127.0.0.1", dbv1alpha1.CqlPort)))

			Expect(restClient.Get(context.Background(), types.NamespacedName{
				Name: rolesSecret.Name, Namespace: rolesSecret.Namespace,
			}, rolesSecret)).To(Succeed())

			rolesSecret.Data = map[string][]byte{
				testUserName: []byte(fmt.Sprintf(`{"password": "%s", "login": true, "super": true, "delete": true}`, testUserPassword)),
			}

			Expect(restClient.Update(context.Background(), rolesSecret)).To(Succeed())

			Eventually(func() error {
				_, err := execPod(pod.Name, pod.Namespace, cmd)
				return err
			}, 30*time.Second, 5*time.Second).ShouldNot(Succeed())
		})
	})
})
