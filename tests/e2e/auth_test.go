package e2e

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("auth logic", func() {
	testAdminRole := "alice"
	testAdminPassword := "testpassword"
	testAdminRoleSecretName := "auth-test-admin-role"
	changedAdminRoleName := "newadmin"
	changedAdminRolePassword := "newpassword"
	const ccName = "auth-test"

	AfterEach(func() {
		cleanupResources(ccName, cfg.operatorNamespace)
	})

	Context("with persistence enabled, internal auth", func() {
		It("should be up to date with the secret", func() {
			cc := newCassandraClusterTmpl(ccName, cfg.operatorNamespace)
			cqlConfigMapName := cc.Name + "-cql-script"

			cqlConfigMap := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cqlConfigMapName,
					Namespace: cc.Namespace,
					Labels: map[string]string{
						cc.Name: "",
					},
				},
				// ordering the keys and scripts in a way to test our lexicographical order guarantee. Fails if the order is not lexicographical.
				Data: map[string]string{
					"3-second-script": `ALTER TABLE e2e_tests.e2e_tests_table 
ADD another_field text;`,
					"a-last-script": `DROP TABLE e2e_tests.e2e_tests_table;`,
					"1-first-script": `CREATE KEYSPACE e2e_tests WITH replication = {'class': 'NetworkTopologyStrategy', 'dc1': 3, 'dc2': 3};
CREATE TABLE e2e_tests.e2e_tests_table (
  name text,
  test_field text,
  PRIMARY KEY (name, test_field)
) WITH
  compaction={'class': 'LeveledCompactionStrategy'} AND
  compression={'sstable_compression': 'LZ4Compressor'};`,
				},
			}

			Expect(kubeClient.Create(ctx, cqlConfigMap)).To(Succeed())
			DeferCleanup(func() {
				cmNamespacedName := types.NamespacedName{Namespace: cqlConfigMap.Namespace, Name: cqlConfigMap.Name}
				Expect(kubeClient.Get(ctx, cmNamespacedName, cqlConfigMap)).To(Succeed())
				Expect(kubeClient.Delete(ctx, cqlConfigMap)).To(Succeed())
			})

			createSecret(cc.Namespace, testAdminRoleSecretName, map[string][]byte{
				dbv1alpha1.CassandraOperatorAdminRole:     []byte(testAdminRole),
				dbv1alpha1.CassandraOperatorAdminPassword: []byte(testAdminPassword),
			})

			sysctls := map[string]string{
				"net.core.somaxconn": "65000",
				"vm.swappiness":      "1",
				"fs.file-max":        "1073741824",
			}
			cc.Spec.AdminRoleSecretName = testAdminRoleSecretName
			cc.Spec.CQLConfigMapLabelKey = cc.Name
			cc.Spec.JMXAuth = "internal"
			cc.Spec.Cassandra.Persistence = dbv1alpha1.Persistence{
				Enabled: true,
				DataVolumeClaimSpec: v1.PersistentVolumeClaimSpec{
					StorageClassName: proto.String(cfg.storageClassName),
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

			podList := &v1.PodList{}
			Expect(kubeClient.List(ctx, podList, client.InNamespace(cc.Namespace), client.MatchingLabels(labels.Cassandra(cc)))).To(Succeed())

			Expect(len(podList.Items)).Should(BeNumerically(">", 0))
			pod := podList.Items[0]

			By("Secure admin role should be created")
			testCQLLogin(cc, pod.Name, pod.Namespace, testAdminRole, testAdminPassword)

			By("Check if CQL ConfigMap has been executed")
			selectQueryCmd := []string{
				"bash",
				"-c",
				fmt.Sprintf("cqlsh -u %s -p \"%s\" -e \"DESCRIBE KEYSPACES\"", testAdminRole, testAdminPassword),
			}
			Eventually(func() (string, error) {
				execResult, err := execPod(pod.Name, pod.Namespace, selectQueryCmd)
				if err != nil {
					return "", err
				}

				return execResult.stdout, nil
			}, 2*time.Minute, 10*time.Second).Should(ContainSubstring("e2e_tests"))

			By("Add a new DC")
			ccName := types.NamespacedName{Name: cc.Name, Namespace: cc.Namespace}
			cassPodsLabels := labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra)
			Expect(kubeClient.Get(ctx, ccName, cc)).To(Succeed())

			newDCName := "dc2"
			cc.Spec.DCs = append(cc.Spec.DCs, dbv1alpha1.DC{Name: newDCName, Replicas: proto.Int32(3)})
			Expect(kubeClient.Update(ctx, cc)).To(Succeed())
			waitForPodsReadiness(cc.Namespace, cassPodsLabels, numberOfNodes(cc))

			newDCPods := &v1.PodList{}
			newDCPodLabels := labels.WithDCLabel(cassPodsLabels, newDCName)
			Expect(kubeClient.List(ctx, newDCPods, client.InNamespace(cc.Namespace), client.MatchingLabels(newDCPodLabels)))
			Expect(newDCPods.Items).ToNot(BeEmpty())

			testCQLLogin(cc, newDCPods.Items[0].Name, newDCPods.Items[0].Namespace, testAdminRole, testAdminPassword)
			expectNumberOfNodes(newDCPods.Items[0].Name, newDCPods.Items[0].Namespace, testAdminRole, testAdminPassword, numberOfNodes(cc))

			testAdminRoleSecret := &v1.Secret{}
			Expect(kubeClient.Get(ctx, types.NamespacedName{
				Name: cc.Spec.AdminRoleSecretName, Namespace: cc.Namespace,
			}, testAdminRoleSecret)).To(Succeed())

			testAdminRoleSecret.Data = map[string][]byte{
				dbv1alpha1.CassandraOperatorAdminRole:     []byte(changedAdminRoleName),
				dbv1alpha1.CassandraOperatorAdminPassword: []byte(changedAdminRolePassword),
			}

			By("Changing admin role name and password")
			Expect(kubeClient.Update(ctx, testAdminRoleSecret)).To(Succeed())

			By("New admin role should work")
			testCQLLogin(cc, pod.Name, pod.Namespace, changedAdminRoleName, changedAdminRolePassword)

			Expect(kubeClient.Get(ctx, types.NamespacedName{
				Name: testAdminRoleSecret.Name, Namespace: testAdminRoleSecret.Namespace,
			}, testAdminRoleSecret)).To(Succeed())

			changedPassword := "new-password-old-role"
			testAdminRoleSecret.Data = map[string][]byte{
				dbv1alpha1.CassandraOperatorAdminRole:     []byte(changedAdminRoleName),
				dbv1alpha1.CassandraOperatorAdminPassword: []byte(changedPassword),
			}

			By("Changing admin role password")
			Expect(kubeClient.Update(ctx, testAdminRoleSecret)).To(Succeed())

			By("Admin role password should be changed")
			testCQLLogin(cc, pod.Name, pod.Namespace, changedAdminRoleName, changedPassword)

			Expect(kubeClient.Get(ctx, types.NamespacedName{
				Name: testAdminRoleSecret.Name, Namespace: testAdminRoleSecret.Namespace,
			}, testAdminRoleSecret)).To(Succeed())

			cassandraRole := "cassandra"
			cassandraRolePassword := "cassandra"
			testAdminRoleSecret.Data = map[string][]byte{
				dbv1alpha1.CassandraOperatorAdminRole:     []byte(cassandraRole),
				dbv1alpha1.CassandraOperatorAdminPassword: []byte(cassandraRolePassword),
			}

			By("Using cassandra/cassandra as admin role")
			Expect(kubeClient.Update(ctx, testAdminRoleSecret)).To(Succeed())

			By("Using cassandra/cassandra should work")
			testCQLLogin(cc, pod.Name, pod.Namespace, cassandraRole, cassandraRolePassword)

			Expect(kubeClient.Get(ctx, types.NamespacedName{
				Name: testAdminRoleSecret.Name, Namespace: testAdminRoleSecret.Namespace,
			}, testAdminRoleSecret)).To(Succeed())

			testAdminRoleSecret.Data = map[string][]byte{
				dbv1alpha1.CassandraOperatorAdminRole:     []byte(testAdminRole),
				dbv1alpha1.CassandraOperatorAdminPassword: []byte(testAdminPassword),
			}

			By("Change the role back to original")
			Expect(kubeClient.Update(ctx, testAdminRoleSecret)).To(Succeed())

			By("Test if changes applied")
			testCQLLogin(cc, pod.Name, pod.Namespace, testAdminRole, testAdminPassword)

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
			Expect(kubeClient.Delete(ctx, cc)).To(Succeed())

			By("Wait until all pods are terminated...")
			waitForPodsTermination(cc.Namespace, map[string]string{dbv1alpha1.CassandraClusterInstance: cc.Name})

			By("Wait until CassandraCluster resource is deleted")
			waitForCassandraClusterSchemaDeletion(cc.Namespace, cc.Name)

			cc = cc.DeepCopy()
			cc.ResourceVersion = ""
			deployCassandraCluster(cc)
			testCQLLogin(cc, pod.Name, pod.Namespace, testAdminRole, testAdminPassword)

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
				reaperPf := portForwardPod(cc.Namespace, labels.Reaper(cc), []string{"8080"})
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

func testCQLLogin(cc *dbv1alpha1.CassandraCluster, podName, podNamespace, roleName, password string) {
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
	Expect(stdout).To(ContainSubstring(fmt.Sprintf("Connected to %s at 127.0.0.1:%d.", cc.Name, dbv1alpha1.CqlPort)))
}
