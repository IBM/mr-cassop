package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/gogo/protobuf/proto"
	"k8s.io/apimachinery/pkg/api/resource"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("auth logic", func() {
	testAdminRole := "alice"
	testAdminPassword := "testpassword"
	testAdminRoleSecretName := "test-admin-role"
	changedAdminRoleName := "newadmin"
	changedAdminRolePassword := "newpassword"
	testAdminRoleSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAdminRoleSecretName,
			Namespace: cassandraNamespace,
		},
		Data: map[string][]byte{
			dbv1alpha1.CassandraOperatorAdminRole:     []byte(testAdminRole),
			dbv1alpha1.CassandraOperatorAdminPassword: []byte(testAdminPassword),
		},
	}
	BeforeEach(func() {
		Expect(restClient.Create(context.Background(), testAdminRoleSecret)).To(Succeed())
	})

	AfterEach(func() {
		Expect(restClient.Delete(context.Background(), testAdminRoleSecret)).To(Succeed())
		Expect(restClient.DeleteAllOf(
			context.Background(),
			&v1.PersistentVolumeClaim{},
			client.InNamespace(cassandraNamespace),
			client.MatchingLabels(map[string]string{
				dbv1alpha1.CassandraClusterInstance:  cassandraRelease,
				dbv1alpha1.CassandraClusterComponent: dbv1alpha1.CassandraClusterComponentCassandra,
			})),
		).To(Succeed())
	})
	Context("with persistence enabled, internal auth", func() {
		It("should be up to date with the secret", func() {
			newCassandraCluster := cassandraCluster.DeepCopy()
			newCassandraCluster.Spec.AdminRoleSecretName = testAdminRoleSecretName
			newCassandraCluster.Spec.JMX.Authentication = "internal"
			newCassandraCluster.Spec.Cassandra.Persistence = dbv1alpha1.Persistence{
				Enabled: true,
				DataVolumeClaimSpec: v1.PersistentVolumeClaimSpec{
					StorageClassName: proto.String(storageClassName),
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("20Gi"),
						},
					},
				},
			}

			deployCassandraCluster(newCassandraCluster)
			waitForPodsReadiness(newCassandraCluster.Namespace, reaperPodLabels, int32(len(newCassandraCluster.Spec.DCs)))

			podList := &v1.PodList{}
			err = restClient.List(context.Background(), podList, client.InNamespace(cassandraNamespace), client.MatchingLabels(cassandraClusterPodLabels))
			Expect(err).ToNot(HaveOccurred())

			Expect(len(podList.Items)).Should(BeNumerically(">", 0))
			pod := podList.Items[0]

			By("Secure admin role should be created")
			testCQLLogin(pod.Name, pod.Namespace, testAdminRole, testAdminPassword)

			Expect(restClient.Get(context.Background(), types.NamespacedName{
				Name: testAdminRoleSecret.Name, Namespace: testAdminRoleSecret.Namespace,
			}, testAdminRoleSecret)).To(Succeed())

			testAdminRoleSecret.Data = map[string][]byte{
				dbv1alpha1.CassandraOperatorAdminRole:     []byte(changedAdminRoleName),
				dbv1alpha1.CassandraOperatorAdminPassword: []byte(changedAdminRolePassword),
			}

			By("Changing admin role name and password")
			Expect(restClient.Update(context.Background(), testAdminRoleSecret)).To(Succeed())

			By("New admin role should work")
			testCQLLogin(pod.Name, pod.Namespace, changedAdminRoleName, changedAdminRolePassword)

			Expect(restClient.Get(context.Background(), types.NamespacedName{
				Name: testAdminRoleSecret.Name, Namespace: testAdminRoleSecret.Namespace,
			}, testAdminRoleSecret)).To(Succeed())

			changedPassword := "new-password-old-role"
			testAdminRoleSecret.Data = map[string][]byte{
				dbv1alpha1.CassandraOperatorAdminRole:     []byte(changedAdminRoleName),
				dbv1alpha1.CassandraOperatorAdminPassword: []byte(changedPassword),
			}

			By("Changing admin role password")
			Expect(restClient.Update(context.Background(), testAdminRoleSecret)).To(Succeed())

			By("Admin role password should be changed")
			testCQLLogin(pod.Name, pod.Namespace, changedAdminRoleName, changedPassword)

			Expect(restClient.Get(context.Background(), types.NamespacedName{
				Name: testAdminRoleSecret.Name, Namespace: testAdminRoleSecret.Namespace,
			}, testAdminRoleSecret)).To(Succeed())

			cassandraRole := "cassandra"
			cassandraRolePassword := "cassandra"
			testAdminRoleSecret.Data = map[string][]byte{
				dbv1alpha1.CassandraOperatorAdminRole:     []byte(cassandraRole),
				dbv1alpha1.CassandraOperatorAdminPassword: []byte(cassandraRolePassword),
			}

			By("Using cassandra/cassandra as admin role")
			Expect(restClient.Update(context.Background(), testAdminRoleSecret)).To(Succeed())

			By("Using cassandra/cassandra should work")
			testCQLLogin(pod.Name, pod.Namespace, cassandraRole, cassandraRolePassword)

			Expect(restClient.Get(context.Background(), types.NamespacedName{
				Name: testAdminRoleSecret.Name, Namespace: testAdminRoleSecret.Namespace,
			}, testAdminRoleSecret)).To(Succeed())

			testAdminRoleSecret.Data = map[string][]byte{
				dbv1alpha1.CassandraOperatorAdminRole:     []byte(testAdminRole),
				dbv1alpha1.CassandraOperatorAdminPassword: []byte(testAdminPassword),
			}

			By("Change the role back to original")
			Expect(restClient.Update(context.Background(), testAdminRoleSecret)).To(Succeed())

			By("Test if changes applied")
			testCQLLogin(pod.Name, pod.Namespace, testAdminRole, testAdminPassword)

			By("Wait until cassandra/cassandra role is removed")
			cmd := []string{
				"sh",
				"-c",
				fmt.Sprintf("cqlsh -u %s -p \"%s\" -e \"SHOW HOST\"", cassandraRole, cassandraRolePassword),
			}

			Eventually(func() error {
				_, err := execPod(pod.Name, pod.Namespace, cmd)
				return err
			}, 3*time.Minute, 15*time.Second).ShouldNot(Succeed())

			By("Recreate cluster with created PVCs to check if the new password is picked up")
			Expect(restClient.Delete(context.Background(), newCassandraCluster)).To(Succeed())

			By("Removing cassandra cluster...")
			Expect(restClient.DeleteAllOf(context.Background(), &dbv1alpha1.CassandraCluster{}, client.InNamespace(cassandraNamespace))).To(Succeed())

			By("Wait until all pods are terminated...")
			waitForPodsTermination(cassandraNamespace, cassandraDeploymentLabel)

			By("Wait until CassandraCluster resource is deleted")
			waitForCassandraClusterSchemaDeletion(cassandraNamespace, cassandraRelease)

			newCassandraCluster = newCassandraCluster.DeepCopy()
			newCassandraCluster.ResourceVersion = ""
			deployCassandraCluster(newCassandraCluster)
			waitForPodsReadiness(newCassandraCluster.Namespace, reaperPodLabels, int32(len(newCassandraCluster.Spec.DCs)))

			testCQLLogin(pod.Name, pod.Namespace, testAdminRole, testAdminPassword)
		})

	})
})

func testCQLLogin(podName, podNamespace, roleName, password string) {
	cmd := []string{
		"sh",
		"-c",
		fmt.Sprintf("cqlsh -u %s -p \"%s\" -e \"SHOW HOST\"", roleName, password),
	}
	stdout := ""
	Eventually(func() error {
		execResult, err := execPod(podName, podNamespace, cmd)
		stdout = execResult.stdout
		return err
	}, 3*time.Minute, 15*time.Second).Should(Succeed())
	Expect(stdout).To(ContainSubstring(fmt.Sprintf("Connected to %s at 127.0.0.1:%d.", cassandraRelease, dbv1alpha1.CqlPort)))
}
