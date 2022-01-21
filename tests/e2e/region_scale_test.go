package e2e

import (
	"context"

	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("one DC region", func() {
	testAdminRole := "alice"
	testAdminPassword := "testpassword"
	testAdminRoleSecretName := "test-admin-role"
	namespaceName1 := "e2e-region1"
	namespaceName2 := "e2e-region2"
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

	It("should be able to scale to a two DC region", func() {
		cc1 := cassandraCluster.DeepCopy()
		cc1.Namespace = namespaceName1
		cc1.Spec.AdminRoleSecretName = testAdminRoleSecretName
		cc1.Spec.HostPort = dbv1alpha1.HostPort{Enabled: true}
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
		cc1.Spec.Ingress = dbv1alpha1.Ingress{
			Domain:           ingressDomain,
			Secret:           ingressSecret,
			IngressClassName: proto.String("public-iks-k8s-nginx"),
		}

		cc2 := cc1.DeepCopy()
		cc2.Namespace = namespaceName2
		cc2.Spec.DCs = []dbv1alpha1.DC{
			{
				Name:     "dc1",
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

		cc1.Spec.DCs = []dbv1alpha1.DC{
			{
				Name:     "dc2",
				Replicas: proto.Int32(3),
			},
		}

		By("Deploy first region")
		deployCassandraCluster(cc1)
		waitForPodsReadiness(cc1.Namespace, labels.ComponentLabels(cc1, dbv1alpha1.CassandraClusterComponentReaper), int32(len(cc1.Spec.DCs)))

		cc1Name := types.NamespacedName{Name: cc1.Name, Namespace: namespaceName1}
		Expect(restClient.Get(context.Background(), cc1Name, cc1)).To(Succeed())
		cc1.Spec.ExternalRegions = dbv1alpha1.ExternalRegions{
			Managed: []dbv1alpha1.ManagedRegion{
				{
					Domain:    ingressDomain,
					Namespace: namespaceName2,
				},
			},
		}
		By("Setup first region to connect to the new second region")
		Expect(restClient.Update(context.Background(), cc1)).To(Succeed())

		By("Deploy second region")
		deployCassandraCluster(cc2)
		waitForPodsReadiness(cc2.Namespace, labels.ComponentLabels(cc1, dbv1alpha1.CassandraClusterComponentReaper), int32(len(cc2.Spec.DCs)))

		By("Check if cqlsh and nodetool work")
		newRegionPods := &v1.PodList{}
		cc2PodLabels := labels.ComponentLabels(cc2, dbv1alpha1.CassandraClusterComponentCassandra)
		Expect(restClient.List(context.Background(), newRegionPods, client.InNamespace(cc2.Namespace), client.MatchingLabels(cc2PodLabels)))
		Expect(newRegionPods.Items).ToNot(BeEmpty())

		testCQLLogin(newRegionPods.Items[0].Name, newRegionPods.Items[0].Namespace, testAdminRole, testAdminPassword)
		expectNumberOfNodes(newRegionPods.Items[0].Name, newRegionPods.Items[0].Namespace, testAdminRole, testAdminPassword, numberOfNodes(cc1)+numberOfNodes(cc2))
	})
})
