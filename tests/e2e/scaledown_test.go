package e2e

import (
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("scaling down", func() {
	testAdminRole := "alice"
	testAdminPassword := "testpassword"
	const ccName = "scaledown"
	testAdminRoleSecretName := ccName + "-admin-role"

	AfterEach(func() {
		cleanupResources(ccName, cfg.operatorNamespace)
	})

	It("should decommission cassandra nodes correctly", func() {
		cc := newCassandraClusterTmpl(ccName, cfg.operatorNamespace)

		createSecret(cc.Namespace, testAdminRoleSecretName, map[string][]byte{
			dbv1alpha1.CassandraOperatorAdminRole:     []byte(testAdminRole),
			dbv1alpha1.CassandraOperatorAdminPassword: []byte(testAdminPassword),
		})

		cc.Spec.DCs = []dbv1alpha1.DC{
			{
				Name:     "dc1",
				Replicas: proto.Int32(5),
			},
		}
		cc.Spec.AdminRoleSecretName = testAdminRoleSecretName
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

		deployCassandraCluster(cc)

		podList := &v1.PodList{}
		Expect(kubeClient.List(ctx, podList, client.InNamespace(cc.Namespace), client.MatchingLabels(labels.Cassandra(cc)))).To(Succeed())
		Expect(podList.Items).To(HaveLen(5))
		expectNumberOfNodes(podList.Items[0].Name, podList.Items[0].Namespace, testAdminRole, testAdminPassword, numberOfNodes(cc))
		Expect(kubeClient.Get(ctx, types.NamespacedName{Name: cc.Name, Namespace: cc.Namespace}, cc)).To(Succeed())
		cc.Spec.DCs[0].Replicas = proto.Int32(3)
		Expect(kubeClient.Update(ctx, cc)).To(Succeed())
		waitForPodsReadiness(cc.Namespace, labels.Cassandra(cc), *cc.Spec.DCs[0].Replicas)
		expectNumberOfNodes(podList.Items[0].Name, podList.Items[0].Namespace, testAdminRole, testAdminPassword, numberOfNodes(cc))
	})
})
