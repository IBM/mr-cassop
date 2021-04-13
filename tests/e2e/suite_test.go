package e2e

import (
	"context"
	"flag"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var err error
var restClient client.Client
var restClientConfig *rest.Config
var kubeClient *kubernetes.Clientset

var cassandraImage string
var cassandraNamespace string
var cassandraRelease string
var imagePullSecret string
var ingressDomain string
var ingressSecret string

var tailLines int64 = 30
var statusCode int

var operatorPodLabel map[string]string
var cassandraDeploymentLabel map[string]string
var proberPodLabels map[string]string
var cassandraClusterPodLabels map[string]string
var reaperPodLabels map[string]string

var cassandraResources = v1.ResourceRequirements{
	Requests: v1.ResourceList{
		v1.ResourceMemory: resource.MustParse("1.5Gi"),
		v1.ResourceCPU:    resource.MustParse("1"),
	},
}

var proberResources = v1.ResourceRequirements{
	Limits: v1.ResourceList{
		v1.ResourceMemory: resource.MustParse("256Mi"),
		v1.ResourceCPU:    resource.MustParse("200m"),
	},
	Requests: v1.ResourceList{
		v1.ResourceMemory: resource.MustParse("256Mi"),
		v1.ResourceCPU:    resource.MustParse("200m"),
	},
}

var cassandraDCs = []v1alpha1.DC{
	{
		Name:     "dc1",
		Replicas: proto.Int32(3),
	},
	{
		Name:     "dc2",
		Replicas: proto.Int32(3),
	},
}

var cassandraObjectMeta metav1.ObjectMeta
var cassandraCluster *v1alpha1.CassandraCluster

const valuesFile = "../../cassandra-operator/values.yaml"

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

var _ = BeforeSuite(func(done Done) {
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

	By("Getting C* image name...")
	content, err := readFile(valuesFile)
	Expect(err).ToNot(HaveOccurred())

	valuesYaml := make(map[string]interface{})

	err = yaml.Unmarshal(content, &valuesYaml)
	Expect(err).ToNot(HaveOccurred())

	cassandraImage = fmt.Sprint(findFirstElemByKey(valuesYaml, "cassandraImage"))

	By("Configuring API Clients for REST...")
	restClientConfig, err = ctrl.GetConfig()
	Expect(err).ToNot(HaveOccurred())

	restClient, err = client.New(restClientConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())

	// Create client test. We use kubernetes package bc currently only it has GetLogs method.
	kubeClient, err = kubernetes.NewForConfig(restClientConfig)
	Expect(err).ToNot(HaveOccurred())

	cassandraCluster = &v1alpha1.CassandraCluster{
		ObjectMeta: cassandraObjectMeta,
		Spec: v1alpha1.CassandraClusterSpec{
			DCs:                 cassandraDCs,
			ImagePullSecretName: imagePullSecret,
			Cassandra: &v1alpha1.Cassandra{
				ImagePullPolicy: "IfNotPresent",
				Resources:       cassandraResources,
				NumSeeds:        2,
			},
			Prober: v1alpha1.Prober{
				ImagePullPolicy: "IfNotPresent",
				Resources:       proberResources,
				Debug:           false,
				Jolokia: v1alpha1.Jolokia{
					ImagePullPolicy: "IfNotPresent",
				},
			},
		},
	}

	close(done)
}, 60) // Set a timeout for function execution

var _ = AfterEach(func() {
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

	By("Removing cassandra cluster...")
	Expect(restClient.DeleteAllOf(context.Background(), &v1alpha1.CassandraCluster{}, client.InNamespace(cassandraNamespace))).To(Succeed())

	By("Wait until all pods are terminated...")
	waitPodsTermination(cassandraNamespace, cassandraDeploymentLabel)

	By("Wait until CassandraCluster schema is deleted")
	waitForCassandraClusterSchemaDeletion(cassandraNamespace, cassandraRelease)
})
