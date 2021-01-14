package e2e

import (
	"context"
	"fmt"
	"github.com/gocql/gocql"
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"encoding/json"
)

var (
	casNamespace = os.Getenv("e2e_namespace")
	casRelease   = os.Getenv("e2e_release") // C* cluster will get this name as well
	log          = logf.Log.WithName("e2e-test")
	err          error
	restClient   client.Client // client to talk to k8s rest API like kubectl
)

func TestCassandraCluster(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cassandra Cluster Suite")
}

var _ = BeforeSuite(func(done Done) {
	By("Configuring environment...")

	err = v1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).ToNot(HaveOccurred())

	close(done)
}, 60) // Set a timeout for function execution

// Call init function from assigned function, in this way we avoid using init() {} function in this file
var _ = Describe("Cassandra cluster", func() {
	var (
		currentTime    time.Time
		casCl          *v1alpha1.CassandraCluster
		reaperRepairs  []v1alpha1.Repair
		statusCode     int
		currentTest    string
		labels         map[string]string
		respBody       []byte
		responseData   map[string]interface{}
		responsesData  []map[string]interface{}
		releaseVersion string

		tailLines int64 = 10

		dcs                      = []string{"dc1", "dc2"}
		testReaperKeyspace       = "reaper_db"
		reaperRequestTimeLayout  = "2006-01-02T15:04:05"
		reaperResponseTimeLayout = "2006-01-02T15:04:05Z"

		casDeploymentLabel = map[string]string{v1alpha1.CassandraClusterInstance: casRelease}
		proberPodLabels    = map[string]string{v1alpha1.CassandraClusterInstance: casRelease, v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentProber}
		casClPodLabels     = map[string]string{v1alpha1.CassandraClusterInstance: casRelease, v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentCassandra}
		reaperPodLabels    = map[string]string{v1alpha1.CassandraClusterInstance: casRelease, v1alpha1.CassandraClusterComponent: v1alpha1.CassandraClusterComponentReaper}
	)

	BeforeEach(func() { // Always initialize your variables in BeforeEach blocks
		if casNamespace == "" {
			Fail("You must define the e2e_namespace environment variable!\nIt's the namespace where your C* cluster will be deployed.\nFor example: `export e2e_namespace=\"my-namespace\"`")
		}

		if casRelease == "" {
			Fail("You must define the e2e_release environment variable!\nIt's the name of your C* cluster.\nFor example: `export e2e_release=\"test\"`")
		}

		currentTime = time.Now().UTC()
		// Generate slice of maps with repairs
		reaperRepairs = append(reaperRepairs,
			v1alpha1.Repair{
				Keyspace:            testReaperKeyspace,
				Owner:               "cassandra",
				Tables:              []string{"snapshot"},
				ScheduleDaysBetween: 7,
				ScheduleTriggerTime: currentTime.AddDate(0, 0, 5).Format(reaperRequestTimeLayout),
				Datacenters:         dcs,
				IncrementalRepair:   false,
				RepairThreadCount:   4,
				Intensity:           "1.0",
				RepairParallelism:   "sequential",
			},
			v1alpha1.Repair{
				Keyspace:            testReaperKeyspace,
				Owner:               "cassandra",
				Tables:              []string{"schema_migration"},
				ScheduleDaysBetween: 7,
				ScheduleTriggerTime: currentTime.AddDate(0, 0, -5).Format(reaperRequestTimeLayout),
				Datacenters:         dcs,
				IncrementalRepair:   false,
				RepairThreadCount:   4,
				Intensity:           "1.0",
				RepairParallelism:   "sequential",
			},
			v1alpha1.Repair{
				Keyspace:            testReaperKeyspace,
				Owner:               "cassandra",
				Tables:              []string{"cluster"},
				ScheduleDaysBetween: 7,
				ScheduleTriggerTime: currentTime.Add(time.Hour * 2).Format(reaperRequestTimeLayout),
				Datacenters:         dcs,
				IncrementalRepair:   false,
				RepairThreadCount:   4,
				Intensity:           "1.0",
				RepairParallelism:   "sequential",
			})

		for _, reaperRepair := range reaperRepairs {
			if len(reaperRepair.Tables) > 1 {
				Fail("You can't specify more than 1 table per reaper repair in e2e tests.")
			}
		}

		casObjectMeta := metav1.ObjectMeta{
			Namespace: casNamespace,
			Name:      casRelease,
		}

		casCl = &v1alpha1.CassandraCluster{
			ObjectMeta: casObjectMeta,
			Spec: v1alpha1.CassandraClusterSpec{
				DCs: []v1alpha1.DC{
					{
						Name:     "dc1",
						Replicas: proto.Int32(3),
					},
					{
						Name:     "dc2",
						Replicas: proto.Int32(3),
					},
				},
				ImagePullSecretName:  "icm-coreeng-pull-secret",
				CQLConfigMapLabelKey: "cql-cm",
				Cassandra: &v1alpha1.Cassandra{
					Image:           "us.icr.io/icm-cassandra/cassandra:3.11.8-0.18.3",
					ImagePullPolicy: "Always",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("1.5Gi"),
							corev1.ResourceCPU:    resource.MustParse("1"),
						},
					},
					NumSeeds: 2,
				},
				SystemKeyspaces: v1alpha1.SystemKeyspaces{
					Names: []v1alpha1.KeyspaceName{"system_auth", "system_distributed", "system_traces"},
					DCs: []v1alpha1.SystemKeyspaceDC{ // it's slice of structs with one element in it
						{
							Name: "dc1",
							RF:   2,
						},
						{
							Name: "dc2",
							RF:   2,
						},
					},
				},
				Prober: v1alpha1.Prober{
					Image:           "us.icr.io/icm-cassandra/cassandra-prober:0.18.3",
					ImagePullPolicy: "IfNotPresent",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("256Mi"),
							corev1.ResourceCPU:    resource.MustParse("200m"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("256Mi"),
							corev1.ResourceCPU:    resource.MustParse("200m"),
						},
					},
					Debug: false,
					Jolokia: v1alpha1.Jolokia{
						Image:           "us.icr.io/icm-docker-images/jolokia-proxy:1.6.2",
						ImagePullPolicy: "IfNotPresent",
					},
				},
				Reaper: &v1alpha1.Reaper{
					Image:           "thelastpickle/cassandra-reaper:2.1.2",
					ImagePullPolicy: "IfNotPresent",
					Keyspace:        testReaperKeyspace,
					DCs: []v1alpha1.DC{
						{
							Name:     "dc1",
							Replicas: proto.Int32(2),
						},
						{
							Name:     "dc2",
							Replicas: proto.Int32(2),
						},
					},
					DatacenterAvailability:                 "each",
					Tolerations:                            nil,
					NodeSelector:                           nil,
					IncrementalRepair:                      false,
					RepairIntensity:                        "1.0",
					RepairManagerSchedulingIntervalSeconds: 0,
					BlacklistTWCS:                          false,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("512Mi"),
							corev1.ResourceCPU:    resource.MustParse("1"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("512Mi"),
							corev1.ResourceCPU:    resource.MustParse("1"),
						},
					},
					ScheduleRepairs: v1alpha1.ScheduleRepairs{
						Enabled:        true,
						StartRepairsIn: "60 minutes",
						Repairs:        reaperRepairs,
					},
					AutoScheduling: v1alpha1.AutoScheduling{
						Enabled:                 false,
						InitialDelayPeriod:      "",
						PeriodBetweenPolls:      "",
						TimeBeforeFirstSchedule: "",
						ScheduleSpreadPeriod:    "",
						ExcludedKeyspaces:       nil,
					},
				},
			},
		}
	})

	JustAfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			fmt.Printf("Test failed! Collecting diags just after failed test in %s\n", CurrentGinkgoTestDescription().TestText)

			podList := &corev1.PodList{}

			// Show all pods statuses
			err := restClient.List(context.Background(), podList, client.InNamespace(casNamespace))
			if err != nil {
				log.Error(err, "Something went wrong.")
			}

			for _, pod := range podList.Items {
				fmt.Println("Pod: ", pod.Name, " Status: ", pod.Status.Phase)
				for _, container := range pod.Status.EphemeralContainerStatuses {
					fmt.Println("Container: ", container.Name, " Ready: ", container.State)
				}
			}

			// Show component's pod logs or from all pods
			switch test := currentTest; test {
			case "reaper":
				labels = reaperPodLabels
			default:
				labels = map[string]string{}
			}

			err = restClient.List(context.Background(), podList, client.InNamespace(casNamespace), client.MatchingLabels(labels))
			if err != nil {
				log.Error(err, "Something went wrong.")
			}

			podLogOpts := corev1.PodLogOptions{TailLines: &tailLines}

			restClientConfig, err := ctrl.GetConfig()
			if err != nil {
				log.Error(err, "error in getting config")
			}

			// Create client test. We use kubernetes package bc currently only it has GetLogs method.
			kubeClient, err := kubernetes.NewForConfig(restClientConfig)
			if err != nil {
				log.Error(err, "error in getting access to K8S")
			}

			for _, pod := range podList.Items {
				fmt.Println("Logs from pod: ", pod.Name)
				err, str := getPodLogs(pod, kubeClient, podLogOpts)
				if err != nil {
					log.Error(err, "Something went wrong.")
				}
				fmt.Println(str)
			}
		}
	})

	AfterEach(func() {
		By("Removing cassandra cluster...")
		Expect(restClient.DeleteAllOf(context.Background(), &v1alpha1.CassandraCluster{}, client.InNamespace(casNamespace))).To(Succeed())

		By("Wait until all pods are terminated...")
		waitPodsTermination(casNamespace, casDeploymentLabel)

		By("Wait until CassandraCluster schema is deleted")
		waitForCassandraClusterSchemaDeletion(casNamespace, casRelease)
	})

	It("Deploy cassandra cluster and all of its components", func() {
		restClientConfig, err := ctrl.GetConfig()
		if err != nil {
			log.Error(err, "Unable to setup rest client config!")
			Fail("Unable to setup rest client config.")
		}

		restClient, err = client.New(restClientConfig, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())
		Expect(restClient).ToNot(BeNil())

		By("Cassandra cluster is starting...")
		Expect(restClient.Create(context.Background(), casCl)).To(Succeed())

		By("Checking prober pods readiness...")
		waitForPodsReadiness(casNamespace, proberPodLabels)

		By("Checking cassandra cluster pods readiness...")
		for _, dc := range dcs {
			casClPodLabels[v1alpha1.CassandraClusterDC] = dc
			waitForPodsReadiness(casNamespace, casClPodLabels)
		}

		By("Checking reaper pods readiness...")
		currentTest = "reaper"
		for _, dc := range dcs {
			casClPodLabels[v1alpha1.CassandraClusterDC] = dc
			waitForPodsReadiness(casNamespace, reaperPodLabels)
		}

		By("Port forwarding cql and jmx ports of cassandra pod...")
		currentTest = ""
		casPf := portForwardPod(casNamespace, casClPodLabels, []string{"9042:9042", "7199:7199"}, restClientConfig)
		defer casPf.Close()

		By("Connecting to Cassandra pod over cql...")
		cluster := gocql.NewCluster("localhost")
		cluster.Port = 9042
		cluster.Keyspace = "system_auth"
		cluster.ConnectTimeout = time.Second * 10
		cluster.Timeout = time.Second * 10
		cluster.Consistency = gocql.LocalQuorum
		cluster.ProtoVersion = 4
		cluster.Authenticator = gocql.PasswordAuthenticator{
			Username: "cassandra",
			Password: "cassandra",
		}

		session, err := cluster.CreateSession()
		Expect(err).ToNot(HaveOccurred())
		defer session.Close()

		By("Running cql query: checking Cassandra version...")
		if err := session.Query(`SELECT release_version FROM system.local`).Consistency(gocql.LocalQuorum).Scan(&releaseVersion); err != nil {
			log.Error(err, "CQL query doesn't work!")
			Fail("Errors shouldn't occur.")
		}
		Expect(releaseVersion).To(Equal("3.11.8"))

		By("Port forwarding reaper API pod port...")
		reaperPf := portForwardPod(casNamespace, reaperPodLabels, []string{"8080:8080"}, restClientConfig)
		defer reaperPf.Close()

		By("Checking cassandra cluster name in reaper...")
		// Check C* cluster name from reaper API
		err, respBody, _ = doHTTPRequest("GET", "http://localhost:8080/cluster/"+casRelease)
		if err != nil {
			log.Error(err, "Something went wrong.")
			Fail("Errors shouldn't occur.")
		}

		err = json.Unmarshal(respBody, &responseData)
		if err != nil {
			log.Error(err, "Something went wrong.")
			Fail("Errors shouldn't occur.")
		}

		Expect(responseData["name"]).To(Equal(casRelease))

		By("Triggering reaper job and checking it's completion status...")
		// Create reaper job
		Eventually(func() bool {
			err, respBody, statusCode = doHTTPRequest("POST", "http://localhost:8080/repair_run?clusterName="+casRelease+"&keyspace="+testReaperKeyspace+"&tables=leader&owner=cassandra&segmentCount=10&repairParallelism=sequential&incrementalRepair=false&intensity=1.0&repairThreadCount=4&datacenters="+strings.Join(dcs[:], ","))
			if statusCode != 201 || err != nil {
				log.Error(err, "Something went wrong.")
				return false
			}

			err = json.Unmarshal(respBody, &responseData)
			if err != nil {
				log.Error(err, "Something went wrong.")
				return false
			}

			return true
		}, time.Minute*2, time.Second*5).Should(BeTrue(), "Reaper job should be created")

		// Start reaper job
		Eventually(func() bool {
			err, _, statusCode = doHTTPRequest("PUT", "http://localhost:8080/repair_run/"+responseData["id"].(string)+"/state/RUNNING")
			if statusCode != 200 || err != nil {
				log.Error(err, "Something went wrong.")
				return false
			}

			return true
		}, time.Minute*2, time.Second*5).Should(BeTrue(), "Reaper job should be started")

		// Get reaper job status
		Eventually(func() bool {
			err, respBody, _ = doHTTPRequest("GET", "http://localhost:8080/repair_run/"+responseData["id"].(string))
			if err != nil {
				log.Error(err, "Something went wrong.")
				return false
			}

			err = json.Unmarshal(respBody, &responseData)
			if err != nil {
				log.Error(err, "Something went wrong.")
				return false
			}

			if responseData["state"] != "DONE" {
				return false
			}

			return true
		}, time.Minute*5, time.Second*5).Should(BeTrue(), "Reaper job should be finished")

		By("Checking reaper job rescheduling logic...")
		// Get response with array of maps with reaper repairs
		Eventually(func() bool {
			err, respBody, statusCode = doHTTPRequest("GET", "http://localhost:8080/repair_schedule?clusterName="+casRelease+"&keyspace="+testReaperKeyspace)
			if statusCode != 200 || err != nil {
				log.Error(err, "Something went wrong.")
				return false
			}

			err = json.Unmarshal(respBody, &responsesData)
			if err != nil {
				log.Error(err, "Something went wrong.")
			}

			if len(responsesData) == 0 {
				log.Error(err, "Response body is empty.")
				return false
			}

			return true
		}, time.Minute*2, time.Second*5).Should(BeTrue(), "Reaper repairs should be received.")

		for _, reaperRepair := range reaperRepairs { // Loop through slice of Repair struct
			parsedReaperReqTime, err := time.Parse(reaperRequestTimeLayout, reaperRepair.ScheduleTriggerTime)
			if err != nil {
				log.Error(err, "Something went wrong.")
				Fail("Errors shouldn't occur.")
			}

			repairResponse := findFirstRepairMapByKV(responsesData, "column_families", reaperRepair.Tables[0])
			parsedReaperRespTime, err := time.Parse(reaperResponseTimeLayout, repairResponse["next_activation"].(string))
			if err != nil {
				log.Error(err, "Something went wrong.")
				Fail("Errors shouldn't occur.")
			}
			// Check rescheduled times
			fmt.Println("Processing schedule for tables: ", repairResponse["column_families"], "...")
			testReaperRescheduleTime(parsedReaperReqTime, parsedReaperRespTime, currentTime)
		}
	})
})

// Todo: find approach to consume restClientConfig from global vars
// Todo: add support of custom flags to specify namespace and release name from cli like: go test ./tests/e2e/ -- --ns=<namespace> --rl=<release name>
