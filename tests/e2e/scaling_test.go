package e2e

import (
	"fmt"
	"time"

	"github.com/gocql/gocql"
	"github.com/ibm/cassandra-operator/controllers/cql"

	appsv1 "k8s.io/api/apps/v1"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("scaling", func() {
	testAdminRole := "alice"
	testAdminPassword := "testpassword"
	const ccName = "scaling"
	testAdminRoleSecretName := ccName + "-admin-role"

	AfterEach(func() {
		cleanupResources(ccName, cfg.operatorNamespace)
	})

	It("should join and decommission cassandra nodes correctly", func() {
		cc := newCassandraClusterTmpl(ccName, cfg.operatorNamespace)

		createSecret(cc.Namespace, testAdminRoleSecretName, map[string][]byte{
			dbv1alpha1.CassandraOperatorAdminRole:     []byte(testAdminRole),
			dbv1alpha1.CassandraOperatorAdminPassword: []byte(testAdminPassword),
		})

		cc.Spec.DCs = []dbv1alpha1.DC{
			{
				Name:     "dc1",
				Replicas: proto.Int32(3),
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
		Expect(podList.Items).To(HaveLen(3))
		pod := podList.Items[0]
		expectNumberOfNodes(pod.Name, pod.Namespace, testAdminRole, testAdminPassword, numberOfNodes(cc))

		By("Add a new DC")
		ccName := types.NamespacedName{Name: cc.Name, Namespace: cc.Namespace}
		cassPodsLabels := labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra)
		Expect(kubeClient.Get(ctx, ccName, cc)).To(Succeed())

		newDCName := "dc2"
		cc.Spec.DCs = append(cc.Spec.DCs, dbv1alpha1.DC{Name: newDCName, Replicas: proto.Int32(5)})
		Expect(kubeClient.Update(ctx, cc)).To(Succeed())
		waitForPodsReadiness(cc.Namespace, cassPodsLabels, numberOfNodes(cc))
		expectNumberOfNodes(pod.Name, pod.Namespace, testAdminRole, testAdminPassword, numberOfNodes(cc))

		By("Scaling down DC `dc2` from 5 to 3 pods")
		Expect(kubeClient.Get(ctx, types.NamespacedName{Name: cc.Name, Namespace: cc.Namespace}, cc)).To(Succeed())
		cc.Spec.DCs[1].Replicas = proto.Int32(3)
		Expect(kubeClient.Update(ctx, cc)).To(Succeed())
		waitForPodsReadiness(cc.Namespace, labels.Cassandra(cc), numberOfNodes(cc))
		expectNumberOfNodes(pod.Name, pod.Namespace, testAdminRole, testAdminPassword, numberOfNodes(cc))

		By("Decommissioning DC `dc2`")
		Expect(kubeClient.Get(ctx, types.NamespacedName{Name: cc.Name, Namespace: cc.Namespace}, cc)).To(Succeed())
		cc.Spec.DCs = []dbv1alpha1.DC{cc.Spec.DCs[0]} //leave only the first DC
		Expect(kubeClient.Update(ctx, cc)).To(Succeed())
		waitForPodsReadiness(cc.Namespace, labels.Cassandra(cc), numberOfNodes(cc))
		expectNumberOfNodes(pod.Name, pod.Namespace, testAdminRole, testAdminPassword, numberOfNodes(cc))

		Eventually(func() bool {
			return errors.IsNotFound(kubeClient.Get(ctx, types.NamespacedName{Name: names.DCService(cc.Name, "dc2"), Namespace: cc.Namespace}, &v1.Service{}))
		}, time.Minute, 10*time.Second).Should(BeTrue())
		Eventually(func() bool {
			return errors.IsNotFound(kubeClient.Get(ctx, types.NamespacedName{Name: names.DC(cc.Name, "dc2"), Namespace: cc.Namespace}, &appsv1.StatefulSet{}))
		}, time.Minute, 10*time.Second).Should(BeTrue())
		Eventually(func() bool {
			return errors.IsNotFound(kubeClient.Get(ctx, types.NamespacedName{Name: names.ReaperDeployment(cc.Name, "dc2"), Namespace: cc.Namespace}, &appsv1.Deployment{}))
		}, time.Minute, 10*time.Second).Should(BeTrue())

		casPf := portForwardPod(cc.Namespace, labels.Cassandra(cc), []string{fmt.Sprintf("%d:%d", dbv1alpha1.CqlPort, dbv1alpha1.CqlPort)})
		defer casPf.Close()

		By("Connecting to Cassandra pod over cql...")
		clusterConfig := gocql.NewCluster("localhost")
		clusterConfig.Port = dbv1alpha1.CqlPort
		clusterConfig.Keyspace = "system_auth"
		clusterConfig.ConnectTimeout = time.Second * 10
		clusterConfig.Timeout = time.Second * 10
		clusterConfig.Consistency = gocql.LocalQuorum
		clusterConfig.Authenticator = gocql.PasswordAuthenticator{
			Username: testAdminRole,
			Password: testAdminPassword,
		}

		cqlSession, err := cql.NewCQLClient(clusterConfig)
		Expect(err).ToNot(HaveOccurred())
		allKeyspaces, err := cqlSession.GetKeyspacesInfo()
		Expect(err).ToNot(HaveOccurred())
		for _, keyspace := range allKeyspaces {
			Expect(keyspace.Replication).ToNot(HaveKey("dc2"), "keyspace should not reference decommissioned DC")
		}
	})
})
