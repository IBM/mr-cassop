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

	caCrtEncoded = "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUZGakNDQXY2Z0F3SUJBZ0lCQVRBTkJna3Foa2lHOXcwQkFRc0ZBREFkTVJzd0dRWURWUVFLRXhKRFlYTnoKWVc1a2NtRWdUM0JsY21GMGIzSXdIaGNOTWpJd01qRTNNVE14TURJM1doY05Nekl3TWpFMU1UTXhNREkzV2pBZApNUnN3R1FZRFZRUUtFeEpEWVhOellXNWtjbUVnVDNCbGNtRjBiM0l3Z2dJaU1BMEdDU3FHU0liM0RRRUJBUVVBCkE0SUNEd0F3Z2dJS0FvSUNBUURJYnNFQjFNOVFIa0h6bld1S0VFUE40NHV4WEg4cG5vRDFsYWQyeFRWNUd3eHIKWnhtQmhIY1Y1Tk1ZZFJUbWEvYksyclZHbi9vQkhqRGY3VWVvOUlqUjVlTXFYc2tqYTZZSTJ3UGxuK3ZtR29hagpsQ0xOUmEwb01Xa1JFM0RJSStTbUthNk9LTDB6ek9oK3dnN2dBYVhqa0VxbXRVVStqcHBJY1VPbXgvcmM3Qlk2Cmxvc3dmWXVmN1MvTE9CY2N1WFBZd214SWt2MEkrSFhDMjdmcVQ0ZmlzV2xCYUtxV2VINVdBcWpsd2FHMVUvMW4KR1NmTHYrcW5OZERkWkwxeERlcDljWW40UitLcTFvQjE0b3VZaklXUVdSUk9UNHFuWHAxQ25kanNXZDVoUitWeApIL1Vac2RiOE9zTm9KOUVEYXNVMmtsYnljVGJjNE1UbSt0RUdOYzdNNVNMT0dHY3BpMzhzV2dEeTNUKzI0N1NUCkkycDl6WWdOdWYxcHZmNW8zbU1KWW0yVndiUC9qUHk2bmlRaXFJQ1FyUzU5d3hsaTBiTVFHN2E4ZWIxT2dPd0UKdHlGT1RVRm9kemdLVDQzbzVJanF4blE0TEZWZlZXYytHakgza3JQOEk0RGJSRW56bERUMDhXWE4xYVFMZmwvSgpBb2tzbFZzc2l1d1lza1h5Z0M1SjVONkdrUkhWa1pUZVNZY3Rna0x4MDFCeHNJSFJIV2E2M29CMnZFc2Nnanc0CnQ4TDZncHFBUGZlTlhIVFlORzJYeTdsZGlYNUJsR3QvT2FpU1hRZE1ZRHpuT3ZrOXAwRWt6VGJpdkRveHJkZlgKeWxnbHJESjBXcWJPcDUrdVhSVGFXaGt2TUc1cnJLMDk2NXU1c3BaanU5ODNZWk56eW9aTk1YK2FEYkswc3dJRApBUUFCbzJFd1h6QU9CZ05WSFE4QkFmOEVCQU1DQW9Rd0hRWURWUjBsQkJZd0ZBWUlLd1lCQlFVSEF3RUdDQ3NHCkFRVUZCd01DTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3SFFZRFZSME9CQllFRktzb2RIQW9lczFDVHVDUmRYTDEKa0FPdkZKdUVNQTBHQ1NxR1NJYjNEUUVCQ3dVQUE0SUNBUUF3a2RVdmZjTytOMWxIcFVXOGh3WE5FZkZRVDl5MQp2dnd4NGc1ODNNckRhNDVINmwvcU5waTVzbjBHbkF6aHNHL2swNHdleURwTEJUR1Fha3BQczVsRHA1b1NpL0l2CkVwWXE5bmhJcXZXbTlzYVpQUXQrVUpxTjcxK1VUUFdFOGZlQXFJSVZJQlMrdTVrNVZRQnVud2JIakJLU050cVAKVC9iYmdldjdxZG5ycEd2YWtpdlFxU2hmQUlyRGR2TGNJSlNNQnhIK0VvT2M1Y2xsUVpRRlR0Q29OYTR0THpPTwpVR2czWGVGRmYyMHIyTXE0U2pYNURzMHBIcTYwSC9DMGJZYmdxQjBFcHBNSXRwdTBZbndGUWlzdGY2OE5LV2xqClAxVG80a0d0bVBkdmo5NU9uVk5PUlFSYlF2NktwcU55R0hSdEM0MWpUbVV5WmVVK1k0Z1c4b2lXVHB4eDZEZHQKN2lTOU9FT0g2SXVocE1WYXdEdjFYdThYQll4NHFVRjRwNnVqVWRuRUZTVW9VQi9vWXVXVkQ3UFAzU3E5SHVVNQpXQTgybzF6UmY1OXF0Z2NsV3ZNTEJQWWtpR29CekhNVFQ0NENBcTRXRkZ0bTA5Y0JPdC81VHRYZ2xhSkhvRmRDCmpnc0tlWjhUYm15Q1VnWHZ4VDlpWkNnYTUzTnhxMGFudGRDUkNQbisrTE5lcFZBRkFuYW54eWdBT0V6cW5SdXgKQm1XNDdvbTNCdGdrMnFiS3FjMnI4anZtN3A3ajRoZFJPTW5GRHV6c1RPdVNnRlFBRWtWenMxYjEwdWFGbldWNApISURySXN2VzlQZjBBR0J3S1B1dVArUVNoME9QU25JUEZrNWM3U1gwSmlBelp6VHJGWXUxYUhaTzlXMHRPTFk5Ck1uaTBtYWVucDRyMXB3PT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	caKeyEncoded = "LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlKSndJQkFBS0NBZ0VBeUc3QkFkVFBVQjVCODUxcmloQkR6ZU9Mc1Z4L0taNkE5WlduZHNVMWVSc01hMmNaCmdZUjNGZVRUR0hVVTVtdjJ5dHExUnAvNkFSNHczKzFIcVBTSTBlWGpLbDdKSTJ1bUNOc0Q1Wi9yNWhxR281UWkKelVXdEtERnBFUk53eUNQa3BpbXVqaWk5TTh6b2ZzSU80QUdsNDVCS3ByVkZQbzZhU0hGRHBzZjYzT3dXT3BhTApNSDJMbiswdnl6Z1hITGx6Mk1Kc1NKTDlDUGgxd3R1MzZrK0g0ckZwUVdpcWxuaCtWZ0tvNWNHaHRWUDlaeGtuCnk3L3FwelhRM1dTOWNRM3FmWEdKK0VmaXF0YUFkZUtMbUl5RmtGa1VUaytLcDE2ZFFwM1k3Rm5lWVVmbGNSLzEKR2JIVy9EckRhQ2ZSQTJyRk5wSlc4bkUyM09ERTV2clJCalhPek9VaXpoaG5LWXQvTEZvQTh0MC90dU8wa3lOcQpmYzJJRGJuOWFiMythTjVqQ1dKdGxjR3ovNHo4dXA0a0lxaUFrSzB1ZmNNWll0R3pFQnUydkhtOVRvRHNCTGNoClRrMUJhSGM0Q2srTjZPU0k2c1owT0N4VlgxVm5QaG94OTVLei9DT0EyMFJKODVRMDlQRmx6ZFdrQzM1ZnlRS0oKTEpWYkxJcnNHTEpGOG9BdVNlVGVocEVSMVpHVTNrbUhMWUpDOGROUWNiQ0IwUjFtdXQ2QWRyeExISUk4T0xmQworb0thZ0QzM2pWeDAyRFJ0bDh1NVhZbCtRWlJyZnptb2tsMEhUR0E4NXpyNVBhZEJKTTAyNHJ3Nk1hM1gxOHBZCkphd3lkRnFtenFlZnJsMFUybG9aTHpCdWE2eXRQZXVidWJLV1k3dmZOMkdUYzhxR1RURi9tZzJ5dExNQ0F3RUEKQVFLQ0FnQmUwUjRYR3JCVUMyeFJyY2ZBMFg3eCtGSU9QbDZkdHJEMC9LM3pIc24wRjVxaGVHMTFlcy9IR0svUQpJeHNYQWo0R3FyNFV3ZnRINmh0ZTQyWUNCR1J4UDFwZW9lWnZEaTdHZzYxdFJHRVpRclVzenhoRG1WR1g4UC91Ckp3ODBidDVzeU0wZHpTSHNUbVF5Q3VWMGpQTUlHeXRsZjkxWkFhYjAzRGdQdnd6cTAvaVVFTUdaMTlwa2RwVWsKZ3MxVU5sc2FVS2RmRWNJSUsxbXlLN1R1Y0Y4dEc1WmFiZ1E5R0pWVFpRQzNhQWx5dVYvb0ZOdGhwTkdCOXBCdgpHNGdPNG5GcWxIcWJiSTVMR1J3K0tFUzNqc3BraGU5NE9HSzBXS3IyWjZ2RjY2R3F6Wk02Rjg4Z0w2UXZRREZPCnBYOVlLWHRRSEp4ckpxbTRoZEJBSUtrZlVmY1kyU1JDZmdrbU5JdU9Hamdob3JqeTNWandkSVBZNWRJemtNVEIKRUZIeUtSTnRKNEdjazRKRG9PTFZCWnJyWkQ2QlNXdDN6UERRSmVCd2htblBYamdZQVE0YjNjK2F2NFNWQlJCeQo2WjRkZEsyd1BqQ25OM1hJNzZWTjhCTGhkaE1zYmxaWmFSYjVkRE5oZXpmMkFYNW1YenNZV3pjOGpTQU1YM0h2Cm9GV1RlOEtkYmg0M1lNY0RDcWx3RzYrVWdUdFVjKzlMRm1EYUR2NVFiaWp0ZE5aQk5OUU9KZmd3eGtXZ0RpaEMKQzdScG1mZWFKVkQyS3VOdExBdENPRUxvNFNOOERWdk1yTHBESW9JWnRIZjFqdHdmZVBzRnN1Vk45M2dTZ1pDYwoxNFhtaFZlSkZyanNiVFdQVUUrbktFWWdqdGVFd2hGaWIvNmZSYmdFVnZUczc2OFMrUUtDQVFFQXltM0tCbVdTCjh2SG5VUG0yTUhHemVBL2FqVVAvN3dMRDlGU0NiZUE1NUFzREhDUmVGb0QvV3RJWnRDYktXaUZjZGpQU3dhUVcKVEpwbDV4clY1Tm0ybGJNdmQzYTFwcHRDd2lGYWhnbml5Rm40VTdPb2hib0crdnlRMkJaSHhHbzF4d1hrckRwTgpvbXR1VUlpMU9ZMXY0eGg4RFBIRlJlZ2x5T2tWRTFvM2tGamEwT1hvYkw1cUFzWjZ4ZU1ielp4eWNRWFZZZWxwCklDRW5CZFl2dFk5T0xORWhyaDBkNFpjSUpWcGVrajk1QUppbkhOQnRzOHdCc2RhK0xVVXBpTDgrQXllNWc1RG4KSEdldGlON2lEL0xLRUcweGtBWGxpMWRPak1xQ3V3aGt0dGpLN280ZFdFY3hGT3daaDFHaHVwL3RCNjBJbldRNwpMK0Rmck5QczhHaHN6UUtDQVFFQS9YbTVOejI5am1PZFcxbnRMTjYrK1R0OStRTVkvakhZWm0wVXdJaEFtMk9DCktzaHgyVE9tNXBGRVkwTzYydkhHT0dqaVY2OEtqaC84V1psSTh6Mng0djh3anVZemJKVEpQTGYxb29RMGRiYzUKd2syMUhQTXV3ZjV0YjBqL2lkQk9hWGlIaWRTbllUcUhlVkwvOFFjSENXQ0ZqM0d6NnhQS2p1NEdDYlhhWXRDNQp4b00rc0I0K3JxckNzZGZHU3lwSlFXRk5KYmJRYVhqUGM1NDNmUEp2aWlGck9vTDMvM3RMdldzTkhmZG1DVTRoCmtTSWhoUnZWcnp2Y3lkdkVKM2hzODY0UjRIaXlZOHlHWmRpa2NOaUQ0dzNsNnJKS3VXcTk0WllPa2xNZGgxNVYKSkc1N1dLbENqY25Jbk9RQWp6dzNVeFRHUGlUT2RsNmhLTTVzZHBlbmZ3S0NBUUJDVE8xRFpSZFpQUVBQVU1wcwpXWUUzakxHL1hRdEJaRDE4RkFYWUtQMnRCREpUa0ZIRXV5Rm54TEtvZjUvOUh6b2llTnpKa1kzQUx6MjdFTjRICm80c2F3dUtFRlR4dndpQitadUE0VUpxWGxtZ3dPZ0t6TWZmQlV1RzU5S295MmJxZFlmL0FyU1BxVTVlQkJ4V2MKTVFmNWNIYUk0dE1ERDRMNHArYkFQT2MvL3VwRVMxang3UGZaeXRwQllCNG1ITnlheWhkV2gxVm9NWk9QWk5TaAplYnRZRUhNZ2pPYlJrVjhZcE4yZXR1MVIxYTIrVVVIdEJwOXplT3MyOXBVZzljcEF6RTBGbTNzbW9ZcUQ3c1JLCkJ2SkpxUW4zcXdiQXVhcS9rRUI3TThlUTM3YXZwWnBVNUpSZHp1cVptSklKQndKaVpqa1JHOWdLMlhOSkx1eEcKM1Z6dEFvSUJBRCtTcU81KzhLem10USsxVlRQOHhkOFNtYnk3bHlnaDdrbDZNRXM5b1I2WDdZeTNhejV6b3ZlUApGWnpqM3RpTTdRODIxeFh3MCsvamU5SXBETS9jK0dHYmFWMWR4U1lGaHhkUWVDNERoSGpGdEpuVURZbXVRRnJ0CmFoc1FMdThzckkzdGFla2F5Y1FyL3RCaURja3czd1h1REhGMnJnNVdqMllic3EzNnkwUWZYNGkzWUNDaDVVeS8KalVjM2ZBZGNHclZvSndZL2ZMUUhWZGlFcFJ3VVhmOUI5SGZmWXozVGVhS1BWK0hkSzkxSG1FbWpTczdzdFVKVwovRUF3ZTFqKzdpeUx5dllHcjQ4eU83OE5mK2pCbFFwOGNON1ZTc0tJVUFsbExsQnF3aXd5YjU1TWkya29Rb1gzClJ2WjZoTjFuMStSaGdIc1RsaWpBQVNHUDdFb3VMUmNDZ2dFQWJFVWQ0a1lWSmlsOGlOeWp3eHJsQ3BaZ0hRcTYKcnNLSW9XZXhCNFFDY256c0ZZMzI0YjVVTUNqc3p1aXpnY3RJWEl3Z2I5R1hmNmVhVXNqNU9BYVV1Rlk0SVZNKwpvSGRDODZSbHdYeEs2c00xU0tCaEkrZnI4YU1OVU16RkF6QmN4cWhoTG4rckNjYU5RSWNHU09JTHBPNm8vMlEvClFSclVKQWpEZVFWbVdGdnZKa00rV3kwQVBLOEoxQnI4UG5vNnZqam0rc2Y5dWZhVEUzWGhWRnZUTm9DK055TDkKcElpN0RPOGFRTGJRcEVjNThtcmIrTDZFcGhzYkdRWjVxMGJ3WENRcHh0Uy92MjE4NklxVER0aSsxOEpoTndmaAp5WFViNFRGOGFjMzUwSGYxZ21vbHBBUFgwNDVkUjZzdUxlUlR5RHE1aHhyVWs3RjBLMHNrZ0h1Ti93PT0KLS0tLS1FTkQgUlNBIFBSSVZBVEUgS0VZLS0tLS0K"
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
				MaxParallelRepairs:  20,
				RepairRunThreads:    4,
				RepairThreadCount:   4,
				SegmentCountPerNode: 1, // speeds up repairs
			},
		},
	}
}
