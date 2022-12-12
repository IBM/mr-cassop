package e2e

import (
	"fmt"
	"time"

	"github.com/ibm/cassandra-operator/controllers/labels"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("user provided roles", func() {
	ccName := "roles"
	AfterEach(func() {
		cleanupResources(ccName, cfg.operatorNamespace)
	})

	Context("if a valid secret provided", func() {
		It("should be created", func() {
			cc := newCassandraClusterTmpl(ccName, cfg.operatorNamespace)
			rolesSecretName := cc.Name + "-cassandra-roles"
			cc.Spec.RolesSecretName = rolesSecretName
			testUserName := "alice"
			testUserPassword := "testpassword"

			createSecret(cc.Namespace, rolesSecretName, map[string][]byte{
				testUserName: []byte(fmt.Sprintf(`{"password": "%s", "login": true, "super": true}`, testUserPassword)),
			})
			deployCassandraCluster(cc)
			podList := &v1.PodList{}
			Expect(kubeClient.List(ctx, podList, client.InNamespace(cc.Namespace), client.MatchingLabels(labels.Cassandra(cc)))).To(Succeed())

			Expect(len(podList.Items)).Should(BeNumerically(">", 0))
			pod := podList.Items[0]

			cmd := []string{
				"sh",
				"-c",
				fmt.Sprintf("cqlsh -u %s -p \"%s\" -e \"SHOW HOST\"", testUserName, testUserPassword),
			}

			stdout := ""
			Eventually(func() error {
				execResult, err := execPod(pod.Name, pod.Namespace, cmd, "cassandra")
				stdout = execResult.stdout
				return err
			}, 30*time.Second, 5*time.Second).Should(Succeed())
			Expect(stdout).To(ContainSubstring(fmt.Sprintf("Connected to %s at %s:%d.", cc.Name, "127.0.0.1", dbv1alpha1.CqlPort)))

			rolesSecret := &v1.Secret{}
			Expect(kubeClient.Get(ctx, types.NamespacedName{
				Name: rolesSecretName, Namespace: cc.Namespace,
			}, rolesSecret)).To(Succeed())

			rolesSecret.Data = map[string][]byte{
				testUserName: []byte(fmt.Sprintf(`{"password": "%s", "login": true, "super": true, "delete": true}`, testUserPassword)),
			}

			Expect(kubeClient.Update(ctx, rolesSecret)).To(Succeed())

			Eventually(func() error {
				_, err := execPod(pod.Name, pod.Namespace, cmd, "cassandra")
				return err
			}, 30*time.Second, 5*time.Second).ShouldNot(Succeed())
		})
	})
})
