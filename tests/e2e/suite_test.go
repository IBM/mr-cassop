package e2e

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/onsi/ginkgo/v2/types"

	apixv1Client "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/gogo/protobuf/proto"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	debugLogsDir = "/tmp/debug-logs/"
)

var (
	kubeClient       client.Client
	restClientConfig *rest.Config
	k8sClientset     *kubernetes.Clientset

	operatorPodLabel = map[string]string{"operator": "cassandra-operator"}
	ctx              = context.Background()
	cfg              = testConfig{}
)

type testConfig struct {
	operatorNamespace string
	imagePullSecret   string
	ingressDomain     string
	ingressSecret     string
	storageClassName  string
	tailLines         int64
}

func init() {
	flag.StringVar(&cfg.operatorNamespace, "operatorNamespace", "default", "Set the namespace for e2e tests run.")
	flag.StringVar(&cfg.imagePullSecret, "imagePullSecret", "all-icr-io", "Set the imagePullSecret.")
	flag.StringVar(&cfg.ingressDomain, "ingressDomain", "", "Set the ingress domain.")
	flag.StringVar(&cfg.ingressSecret, "ingressSecret", "", "Set the ingress secret name.")
	flag.StringVar(&cfg.storageClassName, "storageClassName", "", "Set the storage class name.")
	flag.Int64Var(&cfg.tailLines, "tailLines", 60, "Set the storage class name.")
}

func TestCassandraCluster(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cassandra Cluster Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	// this will run only once on the first process
	return []byte("")
}, func(address []byte) {
	// this will run on every process
	By("Configuring environment...")

	Expect(v1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	By("Configuring API Clients for REST...")
	var err error
	restClientConfig, err = ctrl.GetConfig()
	Expect(err).ToNot(HaveOccurred())

	kubeClient, err = client.New(restClientConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())

	By("Checking if CRD is deployed...")
	apixClient, err := apixv1Client.NewForConfig(restClientConfig)
	Expect(err).ToNot(HaveOccurred())
	cassandraCRD := apixClient.CustomResourceDefinitions()
	crd, err := cassandraCRD.Get(ctx, "cassandraclusters.db.ibm.com", metav1.GetOptions{TypeMeta: metav1.TypeMeta{}})
	if err != nil || crd == nil {
		Fail(fmt.Sprintf("Cassandra operator is not deployed in the cluster. Error: %s", err))
	}

	// Create clientset to be able to pull logs and exec into pods
	k8sClientset, err = kubernetes.NewForConfig(restClientConfig)
	Expect(err).ToNot(HaveOccurred())
})

var _ = JustAfterEach(func() {
	if CurrentSpecReport().Failed() && CurrentSpecReport().State != types.SpecStateInterrupted {
		fmt.Printf("Test failed! Collecting diags just after failed test in %s:%d\n", CurrentSpecReport().FileName(), CurrentSpecReport().LineNumber())
		Expect(os.MkdirAll(debugLogsDir, 0777)).To(Succeed())
		ccList := &v1alpha1.CassandraClusterList{}
		Expect(kubeClient.List(ctx, ccList)).To(Succeed())

		GinkgoWriter.Println("Gathering log info for Cassandra Operator")
		showPodLogs(operatorPodLabel, cfg.operatorNamespace)

		for _, cc := range ccList.Items {
			GinkgoWriter.Printf("Gathering log info for CassandraCluster %s/%s\n", cc.Namespace, cc.Name)
			podList := &v1.PodList{}
			Expect(kubeClient.List(ctx, podList, client.InNamespace(cc.Namespace))).To(Succeed())

			for _, pod := range podList.Items {
				fmt.Println("Pod: ", pod.Name, " Status: ", pod.Status.Phase)
				for _, container := range pod.Status.ContainerStatuses {
					fmt.Println("Container: ", container.Name, " Ready: ", container.State)
				}
			}

			showPodLogs(map[string]string{v1alpha1.CassandraClusterInstance: cc.Name}, cc.Namespace)
		}

		showClusterEvents()
	}
})

func newCassandraClusterTmpl(name, namespace string) *v1alpha1.CassandraCluster {
	adminRoleName := name + "-admin-role"
	return &v1alpha1.CassandraCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.CassandraClusterSpec{
			DCs: []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(3),
				},
			},
			ImagePullSecretName: cfg.imagePullSecret,
			AdminRoleSecretName: adminRoleName,
			Cassandra: &v1alpha1.Cassandra{
				ImagePullPolicy: v1.PullAlways,
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceMemory: resource.MustParse("1.5Gi"),
					},
					Requests: v1.ResourceList{
						v1.ResourceMemory: resource.MustParse("1.5Gi"),
					},
				},
				NumSeeds: 2,
				JVMOptions: []string{
					"-Xmx1024M", //Max Heap Size
					"-Xms1024M", //Min Heap Size
				},
			},
			Prober: v1alpha1.Prober{
				ImagePullPolicy: v1.PullAlways,
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
				Jolokia: v1alpha1.Jolokia{
					ImagePullPolicy: v1.PullAlways,
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
			Reaper: &v1alpha1.Reaper{
				ImagePullPolicy: v1.PullIfNotPresent,
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
			},
		},
	}
}
