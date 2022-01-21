package e2e

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/ibm/cassandra-operator/controllers/labels"

	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("managed multi region cluster", func() {
	testAdminRole := "alice"
	testAdminPassword := "testpassword"
	testAdminRoleSecretName := "test-admin-role"
	namespaceName1 := "e2e-region1-cc"
	namespaceName2 := "e2e-region2-cc"
	adminRoleSecretData := map[string][]byte{
		dbv1alpha1.CassandraOperatorAdminRole:     []byte(testAdminRole),
		dbv1alpha1.CassandraOperatorAdminPassword: []byte(testAdminPassword),
	}

	BeforeEach(func() {
		prepareNamespace(namespaceName1)
		prepareNamespace(namespaceName2)

		createSecret(namespaceName1, testAdminRoleSecretName, adminRoleSecretData)
		createSecret(namespaceName2, testAdminRoleSecretName, adminRoleSecretData)
	})

	AfterEach(func() {
		removeNamespaces(namespaceName1, namespaceName2)
	})

	It("should pass", func() {
		cc1 := cassandraCluster.DeepCopy()
		cc1.Namespace = namespaceName1
		cc1.Spec.AdminRoleSecretName = testAdminRoleSecretName
		cc1.Spec.HostPort = dbv1alpha1.HostPort{Enabled: true}
		cc1.Spec.Ingress = dbv1alpha1.Ingress{
			Domain:           ingressDomain,
			Secret:           ingressSecret,
			IngressClassName: proto.String("public-iks-k8s-nginx"),
		}
		cc1.Spec.Cassandra.Persistence = dbv1alpha1.Persistence{
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
					Domain:    ingressDomain,
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
					Domain:    ingressDomain,
					Namespace: namespaceName1,
				},
			},
		}

		By("Deploying clusters")
		Expect(restClient.Create(context.Background(), cc1)).To(Succeed())
		Expect(restClient.Create(context.Background(), cc2)).To(Succeed())

		waitForPodsReadiness(cc1.Namespace, labels.ComponentLabels(cc1, dbv1alpha1.CassandraClusterComponentProber), 1)
		waitForPodsReadiness(cc2.Namespace, labels.ComponentLabels(cc2, dbv1alpha1.CassandraClusterComponentProber), 1)
		By("Waiting for the first region to become ready")
		waitForPodsReadiness(cc1.Namespace, labels.ComponentLabels(cc1, dbv1alpha1.CassandraClusterComponentCassandra), numberOfNodes(cc1))

		By("Waiting for the second region to become ready")
		waitForPodsReadiness(cc2.Namespace, labels.ComponentLabels(cc1, dbv1alpha1.CassandraClusterComponentCassandra), numberOfNodes(cc2))

		By("Waiting for reaper to become ready in both regions")
		waitForPodsReadiness(cc1.Namespace, labels.ComponentLabels(cc1, dbv1alpha1.CassandraClusterComponentReaper), int32(len(cc1.Spec.DCs)))
		waitForPodsReadiness(cc2.Namespace, labels.ComponentLabels(cc2, dbv1alpha1.CassandraClusterComponentReaper), int32(len(cc1.Spec.DCs)))

		cc1Pods := &v1.PodList{}
		Expect(restClient.List(context.Background(), cc1Pods, client.InNamespace(cc1.Namespace), client.MatchingLabels(labels.ComponentLabels(cc1, dbv1alpha1.CassandraClusterComponentCassandra))))
		Expect(cc1Pods.Items).ToNot(BeEmpty())

		expectNumberOfNodes(cc1Pods.Items[0].Name, cc1Pods.Items[0].Namespace, testAdminRole, testAdminPassword, numberOfNodes(cc1)+numberOfNodes(cc2))
	})
})

func expectNumberOfNodes(podName, podNamespace, roleName, rolePassword string, expectedNodes int32) {
	cmd := []string{
		"sh",
		"-c",
		fmt.Sprintf("nodetool -u %s -pw %s status | awk '/^(U|D)(N|L|J|M)/{print $2}'", roleName, rolePassword),
	}
	var stdout, stderr string
	Eventually(func() error {
		execResult, err := execPod(podName, podNamespace, cmd)
		stdout = execResult.stdout
		stderr = execResult.stderr
		return err
	}, 3*time.Minute, 15*time.Second).Should(Succeed())
	scanner := bufio.NewScanner(strings.NewReader(stdout))

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	Expect(scanner.Err()).To(BeNil())
	Expect(lines).To(HaveLen(int(expectedNodes)), fmt.Sprintf("cluster should have %d nodes. stdout: %s, stderr: %s", expectedNodes, stdout, stderr))
}

func numberOfNodes(cc *dbv1alpha1.CassandraCluster) int32 {
	var nodesNum int32
	for _, dc := range cc.Spec.DCs {
		nodesNum += *dc.Replicas
	}

	return nodesNum
}
