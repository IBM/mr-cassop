package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ibm/cassandra-operator/controllers/names"

	"github.com/gocql/gocql"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// We call init function from assigned `Describe` function, in such way we can avoid using init() {} in this file
var _ = Describe("Cassandra cluster", func() {

	Context("When reaper repair schedules are enabled and set", func() {
		It("should be enabled and all set", func() {
			var (
				currentTime                    = time.Now().UTC()
				reaperRequestTimeLayout        = "2006-01-02T15:04:05"
				reaperResponseTimeLayout       = "2006-01-02T15:04:05Z"
				testRepairReaperKeyspace       = "test_keyspace"
				testRepairReaperTables         = []string{"test_table1", "test_table2", "test_table3", "test_table4", "test_table5", "test_table6"}
				intensity                      = "1.0"
				repairParallelism              = "parallel"
				segmentCount                   = 10
				repairThreadCount        int32 = 4
				reaperRepairSchedules    []v1alpha1.Repair
				respBody                 []byte
				responseData             map[string]interface{}
				responsesData            []map[string]interface{}
				releaseVersion           string
				dcs                      []string
			)

			for _, dc := range cassandraDCs {
				dcs = append(dcs, dc.Name)
			}

			// Generate repair schedules
			reaperRepairSchedules = append(reaperRepairSchedules,
				v1alpha1.Repair{
					Keyspace:            testRepairReaperKeyspace,
					Owner:               "cassandra",
					Tables:              []string{"test_table1", "test_table2"},
					ScheduleDaysBetween: 7,
					ScheduleTriggerTime: currentTime.AddDate(0, 0, 5).Format(reaperRequestTimeLayout),
					Datacenters:         dcs,
					IncrementalRepair:   false,
					RepairThreadCount:   repairThreadCount,
					Intensity:           intensity,
					RepairParallelism:   repairParallelism,
				},
				v1alpha1.Repair{
					Keyspace:            testRepairReaperKeyspace,
					Owner:               "cassandra",
					Tables:              []string{"test_table3"},
					ScheduleDaysBetween: 7,
					ScheduleTriggerTime: currentTime.AddDate(0, 0, -5).Format(reaperRequestTimeLayout),
					Datacenters:         dcs,
					IncrementalRepair:   false,
					RepairThreadCount:   repairThreadCount,
					Intensity:           intensity,
					RepairParallelism:   repairParallelism,
				},
				v1alpha1.Repair{
					Keyspace:            testRepairReaperKeyspace,
					Owner:               "cassandra",
					Tables:              []string{"test_table4", "test_table5"},
					ScheduleDaysBetween: 7,
					ScheduleTriggerTime: currentTime.Add(time.Hour * 2).Format(reaperRequestTimeLayout),
					Datacenters:         dcs,
					IncrementalRepair:   false,
					RepairThreadCount:   repairThreadCount,
					Intensity:           intensity,
					RepairParallelism:   repairParallelism,
				})

			newCassandraCluster := cassandraCluster.DeepCopy()
			newCassandraCluster.Spec.Reaper = &v1alpha1.Reaper{
				ImagePullPolicy:                        "IfNotPresent",
				Keyspace:                               testRepairReaperKeyspace,
				DCs:                                    cassandraDCs,
				DatacenterAvailability:                 "each",
				IncrementalRepair:                      false,
				RepairIntensity:                        "1.0",
				RepairManagerSchedulingIntervalSeconds: 0,
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceMemory: resource.MustParse("512Mi"),
						v1.ResourceCPU:    resource.MustParse("0.5"),
					},
				},
			}

			deployCassandraCluster(newCassandraCluster)

			By("Checking reaper pods readiness...")
			for _, dc := range newCassandraCluster.Spec.DCs {
				waitForPodsReadiness(cassandraNamespace, labels.WithDCLabel(reaperPodLabels, dc.Name))
			}

			By("Port forwarding cql and jmx ports of cassandra pod...")
			casPf := portForwardPod(cassandraNamespace, cassandraClusterPodLabels, []string{fmt.Sprintf("%d:%d", v1alpha1.CqlPort, v1alpha1.CqlPort), fmt.Sprintf("%d:%d", v1alpha1.JmxPort, v1alpha1.JmxPort)})
			defer casPf.Close()

			By("Obtaining auth credentials from Secret")
			activeAdminSecret, err := kubeClient.CoreV1().Secrets(cassandraNamespace).Get(context.Background(), names.ActiveAdminSecret(newCassandraCluster.Name), metav1.GetOptions{})
			if err != nil {
				Fail(fmt.Sprintf("Cannot get Secret : %s", names.ActiveAdminSecret(newCassandraCluster.Name)))
			}

			By("Connecting to Cassandra pod over cql...")
			cluster := gocql.NewCluster("localhost")
			cluster.Port = v1alpha1.CqlPort
			cluster.Keyspace = "system_auth"
			cluster.ConnectTimeout = time.Second * 10
			cluster.Timeout = time.Second * 10
			cluster.Consistency = gocql.LocalQuorum
			cluster.ProtoVersion = 4
			cluster.Authenticator = gocql.PasswordAuthenticator{
				Username: dbv1alpha1.CassandraOperatorAdminRole,
				Password: string(activeAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole]),
			}

			session, err := cluster.CreateSession()
			Expect(err).ToNot(HaveOccurred())
			defer session.Close()

			By("Running cql query: checking Cassandra version...")
			err = session.Query(`SELECT release_version FROM system.local`).Consistency(gocql.LocalQuorum).Scan(&releaseVersion)
			Expect(err).ToNot(HaveOccurred())
			Expect(releaseVersion).To(Equal("3.11.9"))

			By("Running cql query: creating test keyspace...")
			cqlQuery := `CREATE KEYSPACE IF NOT EXISTS %s WITH REPLICATION = { 'class' : 'NetworkTopologyStrategy', '%s' : 3 }`
			err = session.Query(fmt.Sprintf(cqlQuery, testRepairReaperKeyspace, cassandraDCs[0].Name)).Exec()
			Expect(err).ToNot(HaveOccurred())

			By("Running cql query: creating test tables...")
			cqlQuery = `CREATE TABLE %s.%s ( id UUID PRIMARY KEY, lastname text, birthday timestamp, nationality text, weight text, height text )`
			for _, table := range testRepairReaperTables {
				err = session.Query(fmt.Sprintf(cqlQuery, testRepairReaperKeyspace, table)).Exec()
				Expect(err).ToNot(HaveOccurred())
			}

			By("Updating CR: adding reaper schedules...")
			deployedCassandraCluster := &v1alpha1.CassandraCluster{}
			err = restClient.Get(context.Background(), types.NamespacedName{
				Namespace: cassandraNamespace,
				Name:      cassandraRelease,
			}, deployedCassandraCluster)
			Expect(err).ToNot(HaveOccurred())

			deployedCassandraCluster.Spec.Reaper.ScheduleRepairs = v1alpha1.ScheduleRepairs{
				Enabled:        true,
				StartRepairsIn: "60 minutes",
				Repairs:        reaperRepairSchedules,
			}
			Expect(restClient.Update(context.Background(), deployedCassandraCluster)).To(Succeed())

			By("Port forwarding reaper API pod port...")
			reaperPf := portForwardPod(cassandraNamespace, reaperPodLabels, []string{"9999:8080"})
			defer reaperPf.Close()

			By("Checking cassandra cluster name via reaper API...")
			// Check C* cluster name from reaper API
			respBody, _, err = doHTTPRequest("GET", "http://localhost:9999/cluster/"+cassandraRelease)
			Expect(err).ToNot(HaveOccurred())
			err = json.Unmarshal(respBody, &responseData)
			Expect(err).ToNot(HaveOccurred())
			Expect(responseData["name"]).To(Equal(cassandraRelease))

			By("Creating reaper job...")
			Eventually(func() bool {
				respBody, statusCode, err = doHTTPRequest(
					"POST", "http://localhost:9999/repair_run?clusterName="+cassandraRelease+
						"&keyspace="+testRepairReaperKeyspace+"&owner=cassandra&tables=test_table6"+
						"&incrementalRepair=false&repairThreadCount="+fmt.Sprintf("%d", repairThreadCount)+
						"&repairParallelism="+repairParallelism+"&incrementalRepair=false&intensity="+intensity+
						"&datacenters="+strings.Join(dcs[:], ",")+"&segmentCount="+fmt.Sprintf("%d", segmentCount))

				if statusCode != 201 || err != nil {
					return false
				}

				return json.Unmarshal(respBody, &responseData) == nil

			}, time.Minute*2, time.Second*5).Should(BeTrue(), "Reaper job should be created")

			By("Starting reaper job...")
			Eventually(func() bool {
				_, statusCode, err = doHTTPRequest("PUT", "http://localhost:9999/repair_run/"+fmt.Sprintf("%s", responseData["id"])+"/state/RUNNING")
				if statusCode != 200 || err != nil {
					return false
				}

				return true
			}, time.Minute*2, time.Second*5).Should(BeTrue(), "Reaper job should be started")

			By("Getting reaper job status...")
			Eventually(func() bool {
				respBody, _, err = doHTTPRequest("GET", "http://localhost:9999/repair_run/"+fmt.Sprint(responseData["id"]))
				if err != nil {
					return false
				}

				if json.Unmarshal(respBody, &responseData) != nil {
					return false
				}

				// Fail immediately if error received
				if responseData["state"] == "ERROR" {
					Fail(fmt.Sprintf("Reaper repair job failed. Response: %s", responseData))
				}

				return responseData["state"] == "RUNNING"

			}, time.Minute*2, time.Second*5).Should(BeTrue(), "Reaper job should be running")

			By("Checking reaper job rescheduling logic...")
			// Get response with array of maps with reaper repairs
			Eventually(func() bool {
				respBody, statusCode, err = doHTTPRequest("GET", "http://localhost:9999/repair_schedule?clusterName="+cassandraRelease+"&keyspace="+testRepairReaperKeyspace)
				if statusCode != 200 || err != nil {
					return false
				}

				err = json.Unmarshal(respBody, &responsesData)
				Expect(err).ToNot(HaveOccurred())

				return len(responsesData) != 0

			}, time.Minute*2, time.Second*5).Should(BeTrue(), "Reaper repairs should be received.")

			By("Checking rescheduled times...")
			for _, repairSchedule := range reaperRepairSchedules {
				parsedRepairScheduleTime, err := time.Parse(reaperRequestTimeLayout, repairSchedule.ScheduleTriggerTime)
				Expect(err).ToNot(HaveOccurred())

				scheduledRepairsResponse := findFirstMapByKV(responsesData, "column_families", repairSchedule.Tables)
				parsedReaperRespTime, err := time.Parse(reaperResponseTimeLayout, scheduledRepairsResponse["next_activation"].(string))
				Expect(err).ToNot(HaveOccurred())

				fmt.Println("Processing schedule for tables: ", scheduledRepairsResponse["column_families"], "...")
				testReaperRescheduleTime(parsedRepairScheduleTime, parsedReaperRespTime, currentTime)
			}
		})
	})
})

func testReaperRescheduleTime(reqTime time.Time, respTime time.Time, nowTime time.Time) {
	Expect(respTime.Weekday()).To(BeEquivalentTo(reqTime.Weekday()), "Week day should match.")

	if respTime.Year() == reqTime.Year() { // There is a case when schedules set with previous year
		Expect(respTime.YearDay()).To(BeNumerically(">=", reqTime.YearDay()), "Year day should be equal or greater than scheduled.")
		Expect(respTime.YearDay()).To(BeNumerically(">=", nowTime.YearDay()), "Year day should be equal or greater than current.")
	}

	Expect(respTime.Unix()).To(BeNumerically(">=", respTime.Unix()), "Unix timestamp should be greater or equal scheduled.")
	Expect(respTime.Unix()).To(BeNumerically(">=", nowTime.Unix()), "Unix timestamp should be greater or equal current.")
	Expect(respTime.YearDay()-reqTime.YearDay()).To(BeNumerically("<=", 7), "Year day difference should be less or equal 7.")
}
