package e2e

import (
	"fmt"
	"time"

	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Network policies in multi-region cluster", Serial, func() {

	ccName := "netpol-multi"
	adminRoleName := ccName + "-admin-role"
	namespaceName1 := "netpol1"
	namespaceName2 := "netpol2"
	testAdminRole := "alice"
	testAdminPassword := "testpassword"
	adminRoleSecretData := map[string][]byte{
		dbv1alpha1.CassandraOperatorAdminRole:     []byte(testAdminRole),
		dbv1alpha1.CassandraOperatorAdminPassword: []byte(testAdminPassword),
	}
	testPodName := ccName + "-test-pod"

	podSelector := map[string]string{
		"app.kubernetes.io/instance": "test-pod",
	}

	BeforeEach(func() {
		prepareNamespace(namespaceName1)
		prepareNamespace(namespaceName2)

		createSecret(namespaceName1, adminRoleName, adminRoleSecretData)
		createSecret(namespaceName2, adminRoleName, adminRoleSecretData)
	})

	AfterEach(func() {
		removeNamespaces(namespaceName1)
		removeNamespaces(namespaceName2)
	})

	Context("When network policies are enabled", func() {
		It("Network Policies should work", func() {

			cc1 := newCassandraClusterTmpl(ccName, namespaceName1)

			cc1.Spec.NetworkPolicies.Enabled = true
			cc1.Spec.HostPort = dbv1alpha1.HostPort{Enabled: true, UseExternalHostIP: false}
			cc1.Spec.Cassandra.Monitoring.Agent = "tlp"
			cc1.Spec.Cassandra.Monitoring.Enabled = true

			cc1.Spec.Ingress = dbv1alpha1.Ingress{
				Domain:           cfg.ingressDomain,
				Secret:           cfg.ingressSecret,
				IngressClassName: proto.String("public-iks-k8s-nginx"),
			}

			cc1.Spec.NetworkPolicies.ExtraIngressRules = []dbv1alpha1.NetworkPolicyRule{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"alb-image-type": "community"},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"kubernetes.io/metadata.name": "kube-system"},
					},
				},
			}

			cc2 := cc1.DeepCopy()

			cc1.Spec.DCs = []dbv1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(3),
				},
			}

			cc1.Spec.ExternalRegions = dbv1alpha1.ExternalRegions{
				Managed: []dbv1alpha1.ManagedRegion{
					{
						Domain:    cfg.ingressDomain,
						Namespace: namespaceName2,
					},
				},
			}

			cc2.Namespace = namespaceName2
			cc2.Spec.DCs = []dbv1alpha1.DC{
				{
					Name:     "dc2",
					Replicas: proto.Int32(3),
				},
			}

			cc2.Spec.ExternalRegions = dbv1alpha1.ExternalRegions{
				Managed: []dbv1alpha1.ManagedRegion{
					{
						Domain:    cfg.ingressDomain,
						Namespace: namespaceName1,
					},
				},
			}

			By("Deploying clusters")
			Expect(kubeClient.Create(ctx, cc1)).To(Succeed())
			Expect(kubeClient.Create(ctx, cc2)).To(Succeed())

			waitForPodsReadiness(cc1.Namespace, labels.Prober(cc1), 1)
			waitForPodsReadiness(cc2.Namespace, labels.Prober(cc2), 1)
			By("Waiting for the first region to become ready")
			waitForPodsReadiness(cc1.Namespace, labels.Cassandra(cc1), numberOfNodes(cc1))

			By("Waiting for the second region to become ready")
			waitForPodsReadiness(cc2.Namespace, labels.Cassandra(cc1), numberOfNodes(cc2))

			By("Waiting for reaper to become ready in both regions")
			waitForPodsReadiness(cc1.Namespace, labels.Reaper(cc1), int32(len(cc1.Spec.DCs)))
			waitForPodsReadiness(cc2.Namespace, labels.Reaper(cc2), int32(len(cc1.Spec.DCs)))

			testPod := &v1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      testPodName,
					Namespace: namespaceName1,
					Labels:    nil,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:      "test-container",
							Image:     "cassandra:3.11.13",
							Command:   []string{"sleep", "3600"},
							Stdin:     false,
							StdinOnce: false,
							TTY:       false,
						},
					},
				},
			}

			Expect(kubeClient.Create(ctx, testPod)).To(Succeed())

			By("Cassandra shouldn't accept connections from external clients")

			Eventually(func() error {
				if _, err := execPod(testPodName, namespaceName1, []string{"bash", "-c", "ls"}, "test-container"); err != nil {
					return err
				}
				return nil
			}, time.Minute*1, time.Second*10).Should(Succeed())

			cmd := []string{
				"bash",
				"-c",
				fmt.Sprintf("cqlsh --connect-timeout 1 -u %s -p \"%s\" %s-cassandra-dc1.%s.svc.cluster.local", testAdminRole, testAdminPassword, ccName, namespaceName1),
			}

			Eventually(func() string {
				execResult, _ := execPod(testPodName, namespaceName1, cmd, "test-container")
				return execResult.stderr
			}, time.Minute*1, time.Second*10).Should(ContainSubstring("Connection error"))

			By("Prober shouldn't accept connections from external clients")

			cmd = []string{
				"bash",
				"-c",
				fmt.Sprintf("curl --show-error --connect-timeout 3 %s-cassandra-prober.%s.svc.cluster.local/ping", ccName, namespaceName1),
			}

			Eventually(func() string {
				execResult, _ := execPod(testPodName, namespaceName1, cmd, "test-container")
				return execResult.stderr
			}, time.Minute*1, time.Second*10).Should(ContainSubstring("Connection timed out"))

			By("Reaper shouldn't accept connections from external clients")

			cmd = []string{
				"bash",
				"-c",
				fmt.Sprintf("curl --show-error --connect-timeout 3 %s-reaper.%s.svc.cluster.local", ccName, namespaceName1),
			}

			Eventually(func() string {
				execResult, _ := execPod(testPodName, namespaceName1, cmd, "test-container")
				return execResult.stderr
			}, time.Minute*1, time.Second*10).Should(ContainSubstring("Connection timed out"))

			By("Icarus shouldn't accept connections from external clients")

			cmd = []string{
				"bash",
				"-c",
				fmt.Sprintf("curl --show-error --connect-timeout 3 %s-cassandra-dc1.%s.svc.cluster.local:%d/operations", ccName, namespaceName1, dbv1alpha1.IcarusPort),
			}

			Eventually(func() string {
				execResult, _ := execPod(testPodName, namespaceName1, cmd, "test-container")
				return execResult.stderr
			}, time.Minute*1, time.Second*10).Should(ContainSubstring("Connection timed out"))

			By("Cassandra should accept connection from external clients via cql")
			Expect(kubeClient.Get(ctx, types.NamespacedName{Name: testPodName, Namespace: namespaceName1}, testPod)).To(Succeed())
			testPod.Labels = podSelector
			Expect(kubeClient.Update(ctx, testPod)).To(Succeed())

			Eventually(func() error {
				if _, err := execPod(testPodName, namespaceName1, []string{"bash", "-c", "ls"}, "test-container"); err != nil {
					return err
				}
				return nil
			}, time.Minute*1, time.Second*10).Should(Succeed())

			Expect(kubeClient.Get(ctx, types.NamespacedName{Name: cc1.Name, Namespace: namespaceName1}, cc1)).To(Succeed())

			cc1.Spec.NetworkPolicies.ExtraCassandraRules = []dbv1alpha1.NetworkPolicyRule{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: podSelector,
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"kubernetes.io/metadata.name": namespaceName1},
					},
					Ports: []int32{dbv1alpha1.IcarusPort, dbv1alpha1.CqlPort},
				},
			}

			Expect(kubeClient.Update(ctx, cc1)).To(Succeed())

			cmd = []string{
				"bash",
				"-c",
				fmt.Sprintf("cqlsh --connect-timeout 3 -u %s -p \"%s\" -e \"DESCRIBE KEYSPACES\" %s-cassandra-dc1.%s.svc.cluster.local", testAdminRole, testAdminPassword, ccName, namespaceName1),
			}

			Eventually(func() (string, error) {
				execResult, err := execPod(testPodName, namespaceName1, cmd, "test-container")
				if err != nil {
					return "", err
				}
				return execResult.stdout, nil
			}, time.Minute*1, time.Second*10).Should(ContainSubstring("system"))

			By("Icarus should accept connections from external clients")
			cmd = []string{
				"bash",
				"-c",
				fmt.Sprintf("curl --show-error --connect-timeout 3 %s-cassandra-dc1.%s.svc.cluster.local:%d/operations", ccName, namespaceName1, dbv1alpha1.IcarusPort),
			}

			Eventually(func() string {
				execResult, _ := execPod(testPodName, namespaceName1, cmd, "test-container")
				return execResult.stdout
			}, time.Minute*1, time.Second*10).Should(ContainSubstring("[ ]"))

			By("Cassandra should accept connection for prometheus agent")
			Expect(kubeClient.Get(ctx, types.NamespacedName{Name: testPodName, Namespace: namespaceName1}, testPod)).To(Succeed())
			testPod.Labels = podSelector
			Expect(kubeClient.Update(ctx, testPod)).To(Succeed())

			Expect(kubeClient.Get(ctx, types.NamespacedName{Name: cc1.Name, Namespace: namespaceName1}, cc1)).To(Succeed())

			cc1.Spec.NetworkPolicies.ExtraPrometheusRules = []dbv1alpha1.NetworkPolicyRule{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: podSelector,
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"kubernetes.io/metadata.name": namespaceName1},
					},
				},
			}

			Expect(kubeClient.Update(ctx, cc1)).To(Succeed())

			cmd = []string{
				"bash",
				"-c",
				fmt.Sprintf("curl --connect-timeout 3 %s-cassandra-dc1.%s.svc.cluster.local:8090/", ccName, namespaceName1),
			}

			Eventually(func() (string, error) {
				execResult, err := execPod(testPodName, namespaceName1, cmd, "test-container")
				if err != nil {
					return "", err
				}
				return execResult.stdout, nil
			}, time.Minute*2, time.Second*10).Should(ContainSubstring("jvm_memory"))

			By("Check if nodes are healthy")
			ccPods := &v1.PodList{}
			Expect(kubeClient.List(ctx, ccPods, client.InNamespace(cc1.Namespace), client.MatchingLabels(labels.Cassandra(cc1))))
			Expect(ccPods.Items).ToNot(BeEmpty())
			expectNumberOfNodes(ccPods.Items[0].Name, ccPods.Items[0].Namespace, testAdminRole, testAdminPassword, numberOfNodes(cc1)+numberOfNodes(cc2))

			Expect(kubeClient.List(ctx, ccPods, client.InNamespace(cc2.Namespace), client.MatchingLabels(labels.Cassandra(cc2))))
			Expect(ccPods.Items).ToNot(BeEmpty())
			expectNumberOfNodes(ccPods.Items[0].Name, ccPods.Items[0].Namespace, testAdminRole, testAdminPassword, numberOfNodes(cc1)+numberOfNodes(cc2))
		})
	})
})
