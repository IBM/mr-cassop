package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
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
			sysctls := map[string]string{
				"net.core.somaxconn": "65000",
				"vm.swappiness":      "1",
				"fs.file-max":        "1073741824",
			}
			cc := cassandraCluster.DeepCopy()
			cc.Spec.AdminRoleSecretName = testAdminRoleSecretName
			cc.Spec.JMXAuth = "internal"
			cc.Spec.Cassandra.Persistence = dbv1alpha1.Persistence{
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
			cc.Spec.Reaper = &dbv1alpha1.Reaper{
				MaxParallelRepairs:  20,
				RepairRunThreads:    4,
				RepairThreadCount:   4,
				SegmentCountPerNode: 1, // speeds up repairs
			}
			cc.Spec.Cassandra.Sysctls = sysctls

			deployCassandraCluster(cc)
			waitForPodsReadiness(cc.Namespace, reaperPodLabels, int32(len(cc.Spec.DCs)))

			podList := &v1.PodList{}
			err = restClient.List(context.Background(), podList, client.InNamespace(cassandraNamespace), client.MatchingLabels(cassandraClusterPodLabels))
			Expect(err).ToNot(HaveOccurred())

			Expect(len(podList.Items)).Should(BeNumerically(">", 0))
			pod := podList.Items[0]

			By("Secure admin role should be created")
			testCQLLogin(pod.Name, pod.Namespace, testAdminRole, testAdminPassword)

			By("Add a new DC")
			ccName := types.NamespacedName{Name: cc.Name, Namespace: cc.Namespace}
			cassPodsLabels := labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra)
			Expect(restClient.Get(context.Background(), ccName, cc)).To(Succeed())

			newDCName := "dc2"
			cc.Spec.DCs = append(cc.Spec.DCs, dbv1alpha1.DC{Name: newDCName, Replicas: proto.Int32(3)})
			Expect(restClient.Update(context.Background(), cc)).To(Succeed())
			waitForPodsReadiness(cc.Namespace, cassPodsLabels, numberOfNodes(cc))

			newDCPods := &v1.PodList{}
			newDCPodLabels := labels.WithDCLabel(cassPodsLabels, newDCName)
			Expect(restClient.List(context.Background(), newDCPods, client.InNamespace(cc.Namespace), client.MatchingLabels(newDCPodLabels)))
			Expect(newDCPods.Items).ToNot(BeEmpty())

			testCQLLogin(newDCPods.Items[0].Name, newDCPods.Items[0].Namespace, testAdminRole, testAdminPassword)
			expectNumberOfNodes(newDCPods.Items[0].Name, newDCPods.Items[0].Namespace, testAdminRole, testAdminPassword, numberOfNodes(cc))

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
			Expect(restClient.Delete(context.Background(), cc)).To(Succeed())

			By("Removing cassandra cluster...")
			Expect(restClient.DeleteAllOf(context.Background(), &dbv1alpha1.CassandraCluster{}, client.InNamespace(cassandraNamespace))).To(Succeed())

			By("Wait until all pods are terminated...")
			waitForPodsTermination(cassandraNamespace, cassandraDeploymentLabel)

			By("Wait until CassandraCluster resource is deleted")
			waitForCassandraClusterSchemaDeletion(cassandraNamespace, cassandraRelease)

			cc = cc.DeepCopy()
			cc.ResourceVersion = ""
			deployCassandraCluster(cc)
			waitForPodsReadiness(cc.Namespace, reaperPodLabels, int32(len(cc.Spec.DCs)))
			testCQLLogin(pod.Name, pod.Namespace, testAdminRole, testAdminPassword)

			By("Check overridden sysctl parameters")
			for key, value := range sysctls {
				cmd := []string{
					"sh",
					"-c",
					fmt.Sprintf("sysctl -n %s", key),
				}
				execResult, err := execPod(pod.Name, pod.Namespace, cmd)
				Expect(err).ToNot(HaveOccurred())
				Expect(execResult.stderr).To(BeEmpty())
				Expect(strings.TrimSpace(execResult.stdout)).To(Equal(value))
			}

			By("Check if reaper works")
			Eventually(func() (string, error) {
				reaperPf := portForwardPod(cassandraNamespace, reaperPodLabels, []string{"8080"})
				defer reaperPf.Close()
				forwardedPorts, err := reaperPf.GetPorts()
				Expect(err).ToNot(HaveOccurred())

				respBody, _, err := doHTTPRequest("GET", fmt.Sprintf("http://localhost:%d/cluster/%s", forwardedPorts[0].Local, cc.Name))
				if err != nil {
					return "", err
				}

				responseData := make(map[string]interface{})
				err = json.Unmarshal(respBody, &responseData)
				if err != nil {
					return "", errors.Errorf("json unmarshal error: %s", string(respBody))
				}

				name, ok := responseData["name"].(string)
				if !ok {
					return "", errors.Errorf("name is not string. Resp data: %#v", responseData)
				}
				return name, nil
			}, 15*time.Minute, 30*time.Second).Should(Equal(cc.Name))
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
	}, 5*time.Minute, 15*time.Second).Should(Succeed())
	Expect(stdout).To(ContainSubstring(fmt.Sprintf("Connected to %s at 127.0.0.1:%d.", cassandraRelease, dbv1alpha1.CqlPort)))
}
