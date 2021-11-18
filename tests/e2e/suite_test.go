package e2e

import (
	"context"
	"flag"
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apixv1Client "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	restClient          client.Client
	restClientConfig    *rest.Config
	kubeClient          *kubernetes.Clientset
	cassandraObjectMeta metav1.ObjectMeta
	cassandraCluster    *v1alpha1.CassandraCluster
	adminRoleSecret     *v1.Secret

	err                 error
	cassandraNamespace  string
	cassandraRelease    string
	imagePullSecret     string
	adminRoleSecretName string
	ingressDomain       string
	ingressSecret       string
	tailLines           int64 = 30
	statusCode          int

	operatorPodLabel          map[string]string
	cassandraDeploymentLabel  map[string]string
	proberPodLabels           map[string]string
	cassandraClusterPodLabels map[string]string
	reaperPodLabels           map[string]string

	cassandraResources = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceMemory: resource.MustParse("1.5Gi"),
			v1.ResourceCPU:    resource.MustParse("1"),
		},
		Requests: v1.ResourceList{
			v1.ResourceMemory: resource.MustParse("1.5Gi"),
			v1.ResourceCPU:    resource.MustParse("1"),
		},
	}

	proberResources = v1.ResourceRequirements{
		Requests: v1.ResourceList{
			v1.ResourceMemory: resource.MustParse("256Mi"),
			v1.ResourceCPU:    resource.MustParse("200m"),
		},
	}

	cassandraDCs = []v1alpha1.DC{
		{
			Name:     "dc1",
			Replicas: proto.Int32(3),
		},
	}
)

func init() {
	flag.StringVar(&cassandraNamespace, "cassandraNamespace", "default", "Set the namespace for e2e tests run.")
	flag.StringVar(&cassandraRelease, "cassandraRelease", "e2e-tests", "Set the cassandra cluster release name for e2e tests run.")
	flag.StringVar(&imagePullSecret, "imagePullSecret", "all-icr-io", "Set the imagePullSecret.")
	flag.StringVar(&ingressDomain, "ingressDomain", "", "Set the ingress domain.")
	flag.StringVar(&ingressSecret, "ingressSecret", "", "Set the ingress secret name.")
}

func TestCassandraCluster(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cassandra Cluster Suite")
}

var _ = BeforeSuite(func() {
	operatorPodLabel = map[string]string{"operator": "cassandra-operator"}
	cassandraDeploymentLabel = map[string]string{v1alpha1.CassandraClusterInstance: cassandraRelease}
	proberPodLabels = map[string]string{v1alpha1.CassandraClusterInstance: cassandraRelease, v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentProber}
	cassandraClusterPodLabels = map[string]string{v1alpha1.CassandraClusterInstance: cassandraRelease, v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra}
	reaperPodLabels = map[string]string{v1alpha1.CassandraClusterInstance: cassandraRelease, v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentReaper}

	cassandraObjectMeta = metav1.ObjectMeta{
		Namespace: cassandraNamespace,
		Name:      cassandraRelease,
	}

	By("Configuring environment...")

	err = v1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).ToNot(HaveOccurred())

	By("Configuring API Clients for REST...")
	restClientConfig, err = ctrl.GetConfig()
	Expect(err).ToNot(HaveOccurred())

	restClient, err = client.New(restClientConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())

	By("Checking if CRD is deployed...")
	apixClient, err := apixv1Client.NewForConfig(restClientConfig)
	Expect(err).ToNot(HaveOccurred())
	cassandraCRD := apixClient.CustomResourceDefinitions()
	crd, err := cassandraCRD.Get(context.Background(), "cassandraclusters.db.ibm.com", metav1.GetOptions{TypeMeta: metav1.TypeMeta{}})
	if err != nil || crd == nil {
		Fail(fmt.Sprintf("Cassandra operator is not deployed in the cluster. Error: %s", err))
	}

	// Create client test. We use kubernetes package bc currently only it has GetLogs method.
	kubeClient, err = kubernetes.NewForConfig(restClientConfig)
	Expect(err).ToNot(HaveOccurred())

	adminRoleSecretName = "admin-role"

	cassandraCluster = &v1alpha1.CassandraCluster{
		ObjectMeta: cassandraObjectMeta,
		Spec: v1alpha1.CassandraClusterSpec{
			DCs:                 cassandraDCs,
			ImagePullSecretName: imagePullSecret,
			AdminRoleSecretName: adminRoleSecretName,
			Cassandra: &v1alpha1.Cassandra{
				ImagePullPolicy: v1.PullAlways,
				Resources:       cassandraResources,
				NumSeeds:        2,
			},
			Prober: v1alpha1.Prober{
				ImagePullPolicy: v1.PullAlways,
				Resources:       proberResources,
				Debug:           false,
				Jolokia: v1alpha1.Jolokia{
					ImagePullPolicy: v1.PullAlways,
					Resources:       proberResources,
				},
			},
			JVM: v1alpha1.JVM{
				MaxHeapSize: "1024M",
			},
		},
	}

	err = restClient.Get(context.Background(), types.NamespacedName{Namespace: cassandraCluster.Namespace, Name: adminRoleSecretName}, adminRoleSecret)

	if err != nil {
		adminRoleSecret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      adminRoleSecretName,
				Namespace: cassandraNamespace,
			},
			Data: map[string][]byte{
				v1alpha1.CassandraOperatorAdminRole:     []byte("cassandra-operator"),
				v1alpha1.CassandraOperatorAdminPassword: []byte("password"),
			},
		}

		Expect(restClient.Create(context.Background(), adminRoleSecret)).To(Succeed())
	}

})

var _ = AfterSuite(func() {
	Expect(restClient.Delete(context.Background(), adminRoleSecret)).To(Succeed())
})

var _ = JustAfterEach(func() {
	if CurrentGinkgoTestDescription().Failed {
		fmt.Printf("Test failed! Collecting diags just after failed test in %s\n", CurrentGinkgoTestDescription().TestText)

		podList := &v1.PodList{}
		err = restClient.List(context.Background(), podList, client.InNamespace(cassandraNamespace))
		Expect(err).ToNot(HaveOccurred())

		for _, pod := range podList.Items {
			fmt.Println("Pod: ", pod.Name, " Status: ", pod.Status.Phase)
			for _, container := range pod.Status.ContainerStatuses {
				fmt.Println("Container: ", container.Name, " Ready: ", container.State)
			}
		}

		showPodLogs(operatorPodLabel)
		showPodLogs(cassandraDeploymentLabel)
	}
})

var _ = AfterEach(func() {
	By("Removing cassandra cluster...")
	Expect(restClient.DeleteAllOf(context.Background(), &v1alpha1.CassandraCluster{}, client.InNamespace(cassandraNamespace))).To(Succeed())

	By("Wait until all pods are terminated...")
	waitPodsTermination(cassandraNamespace, cassandraDeploymentLabel)

	By("Wait until CassandraCluster schema is deleted")
	waitForCassandraClusterSchemaDeletion(cassandraNamespace, cassandraRelease)
})
