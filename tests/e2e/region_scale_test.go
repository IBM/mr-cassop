package e2e

import (
	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("single DC region", Serial, func() {
	testAdminRole := "alice"
	testAdminPassword := "testpassword"
	testAdminRoleSecretName := "test-admin-role"
	ccName := "region-scale"
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

	It("should be able to scale to a two single DC regions", func() {
		cc1 := newCassandraClusterTmpl(ccName, namespaceName1)
		cc1.Spec.AdminRoleSecretName = testAdminRoleSecretName
		cc1.Spec.HostPort = dbv1alpha1.HostPort{Enabled: true}
		cc1.Spec.Cassandra.Persistence = dbv1alpha1.Persistence{
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
		cc1.Spec.Ingress = dbv1alpha1.Ingress{
			Domain:           cfg.ingressDomain,
			Secret:           cfg.ingressSecret,
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
					Domain:    cfg.ingressDomain,
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

		cc1Name := types.NamespacedName{Name: cc1.Name, Namespace: namespaceName1}
		Expect(kubeClient.Get(ctx, cc1Name, cc1)).To(Succeed())
		cc1.Spec.ExternalRegions = dbv1alpha1.ExternalRegions{
			Managed: []dbv1alpha1.ManagedRegion{
				{
					Domain:    cfg.ingressDomain,
					Namespace: namespaceName2,
				},
			},
		}
		By("Setup first region to connect to the new second region")
		Expect(kubeClient.Update(ctx, cc1)).To(Succeed())

		By("Deploy second region")
		deployCassandraCluster(cc2)

		By("Check if cqlsh and nodetool work")
		newRegionPods := &v1.PodList{}
		cc2PodLabels := labels.Cassandra(cc2)
		Expect(kubeClient.List(ctx, newRegionPods, client.InNamespace(cc2.Namespace), client.MatchingLabels(cc2PodLabels)))
		Expect(newRegionPods.Items).ToNot(BeEmpty())

		testCQLLogin(cc1, newRegionPods.Items[0].Name, newRegionPods.Items[0].Namespace, testAdminRole, testAdminPassword)
		expectNumberOfNodes(newRegionPods.Items[0].Name, newRegionPods.Items[0].Namespace, testAdminRole, testAdminPassword, numberOfNodes(cc1)+numberOfNodes(cc2))
	})
})
