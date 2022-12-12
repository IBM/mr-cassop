package e2e

import (
	"strings"

	"github.com/ibm/cassandra-operator/controllers/util"
	"k8s.io/apimachinery/pkg/types"

	"github.com/ibm/cassandra-operator/controllers/labels"

	"github.com/gogo/protobuf/proto"
	"k8s.io/apimachinery/pkg/api/resource"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("unmanaged region", Serial, func() {
	testAdminRole := "alice"
	testAdminPassword := "testpassword"
	testAdminRoleSecretName := "test-admin-role"
	namespaceName1 := "e2e-unmanaged-cc"
	namespaceName2 := "e2e-managed-cc"
	ccName := "unmanaged-region"
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

	It("should be able to connect to a managed one", func() {
		unmanagedCC := newCassandraClusterTmpl(ccName, namespaceName1)
		unmanagedCC.Spec.AdminRoleSecretName = testAdminRoleSecretName
		unmanagedCC.Spec.HostPort = dbv1alpha1.HostPort{Enabled: true, UseExternalHostIP: true}
		unmanagedCC.Spec.JMXAuth = "local_files"
		unmanagedCC.Spec.Cassandra.Persistence = dbv1alpha1.Persistence{
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

		managedCC := unmanagedCC.DeepCopy()
		managedCC.Namespace = namespaceName2
		managedCC.Spec.DCs = []dbv1alpha1.DC{
			{
				Name:     "dc1",
				Replicas: proto.Int32(3),
			},
		}

		unmanagedCC.Spec.DCs = []dbv1alpha1.DC{
			{
				Name:     "dc2",
				Replicas: proto.Int32(3),
			},
		}

		By("Deploy an unmanaged cluster")
		deployCassandraCluster(unmanagedCC)

		Expect(kubeClient.Get(ctx, types.NamespacedName{Name: unmanagedCC.Name, Namespace: unmanagedCC.Namespace}, unmanagedCC))
		By("Prepare the keyspaces on the unmanaged cluster")
		unmanagedCC.Spec.SystemKeyspaces = dbv1alpha1.SystemKeyspaces{
			Keyspaces: []dbv1alpha1.KeyspaceName{"reaper", "system_auth"},
			DCs: []dbv1alpha1.SystemKeyspaceDC{
				{
					Name: "dc1",
					RF:   3,
				},
				{
					Name: "dc2",
					RF:   3,
				},
			},
		}
		Expect(kubeClient.Update(ctx, unmanagedCC))

		By("Get unmanaged cluster seed nodes IPs")
		unmanagedCCPods := &v1.PodList{}
		Expect(kubeClient.List(ctx, unmanagedCCPods, client.InNamespace(unmanagedCC.Namespace), client.MatchingLabels(labels.ComponentLabels(unmanagedCC, dbv1alpha1.CassandraClusterComponentCassandra)), client.HasLabels{dbv1alpha1.CassandraClusterSeed}))
		Expect(unmanagedCCPods.Items).ToNot(BeEmpty())

		var seedIPs []string
		for _, pod := range unmanagedCCPods.Items {
			seedIPs = append(seedIPs, pod.Spec.NodeName)
		}

		managedCC.Spec.ExternalRegions = dbv1alpha1.ExternalRegions{
			Unmanaged: []dbv1alpha1.UnmanagedRegion{
				{
					Seeds: seedIPs,
					DCs: []dbv1alpha1.SystemKeyspaceDC{
						{
							Name: "dc2",
							RF:   3,
						},
					},
				},
			},
		}

		By("Deploy a CassandraCluster with connected unmanaged Cassandra cluster")
		deployCassandraCluster(managedCC)

		By("Waiting for all Cassandra nodes to become ready")
		waitForPodsReadiness(unmanagedCC.Namespace, labels.Reaper(unmanagedCC), int32(len(unmanagedCC.Spec.DCs)))
		waitForPodsReadiness(unmanagedCC.Namespace, labels.Cassandra(unmanagedCC), *unmanagedCC.Spec.DCs[0].Replicas)

		expectNumberOfNodes(unmanagedCCPods.Items[0].Name, unmanagedCCPods.Items[0].Namespace, testAdminRole, testAdminPassword, numberOfNodes(managedCC)+numberOfNodes(unmanagedCC))

		By("Check if node's externalIP is used")
		cmd := []string{
			"sh",
			"-c",
			"cat /etc/cassandra/cassandra.yaml | grep 'broadcast_address:' | cut -f2 -d':'",
		}

		nodes := &v1.NodeList{}
		Expect(kubeClient.List(ctx, nodes))
		checkBroadcastAddressOnAllPods(unmanagedCCPods, nodes, v1.NodeExternalIP, cmd)
	})
})

func checkBroadcastAddressOnAllPods(podList *v1.PodList, nodeList *v1.NodeList, addressType v1.NodeAddressType, cmd []string) {
	for _, pod := range podList.Items {
		nodeName := pod.Spec.NodeName

		var nodeFound = false
		for _, node := range nodeList.Items {
			if node.Name == nodeName {
				cassandraIP := util.GetNodeIP(addressType, node.Status.Addresses)
				execResult, err := execPod(pod.Name, pod.Namespace, cmd, "cassandra")
				execResult.stdout = strings.TrimSuffix(execResult.stdout, "\n")
				execResult.stdout = strings.TrimSpace(execResult.stdout)
				Expect(err).ToNot(HaveOccurred())
				Expect(execResult.stderr).To(BeEmpty())
				Expect(execResult.stdout).To(Equal(cassandraIP))
				nodeFound = true
			}
		}
		Expect(nodeFound).To(BeTrue())
	}
}
