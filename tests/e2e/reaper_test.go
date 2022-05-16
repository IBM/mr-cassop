package e2e

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ibm/cassandra-operator/controllers/names"

	"github.com/gocql/gocql"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
)

// We call init function from assigned `Describe` function, in such way we can avoid using init() {} in this file
var _ = Describe("Reaper test", func() {
	ccName := "reaper"
	AfterEach(func() {
		cleanupResources(ccName, cfg.operatorNamespace)
	})
	Context("When reaper repair schedules are enabled and set", func() {
		It("should be enabled and all set", func() {
			var (
				currentTime                    = time.Now().UTC()
				reaperRequestTimeLayout        = "2006-01-02T15:04:05"
				reaperResponseTimeLayout       = "2006-01-02T15:04:05Z"
				testRepairReaperKeyspace       = "test_keyspace"
				testRepairReaperTables         = []string{"test_table1", "test_table2", "test_table3", "test_table4", "test_table5", "test_table6"}
				intensity                      = "1.0"
				repairParallelism              = "PARALLEL"
				segmentCount                   = 10
				repairThreadCount        int32 = 4

				reaperRepairSchedules []v1alpha1.RepairSchedule
				respBody              []byte
				responseData          map[string]interface{}
				responsesData         []map[string]interface{}
				releaseVersion        string
				dcs                   []string
			)

			cc := newCassandraClusterTmpl(ccName, cfg.operatorNamespace)

			for _, dc := range cc.Spec.DCs {
				dcs = append(dcs, dc.Name)
			}

			deployCassandraCluster(cc)

			By("Port forwarding cql ports of cassandra pod...")
			casPf := portForwardPod(cc.Namespace, labels.Cassandra(cc), []string{fmt.Sprintf("%d:%d", v1alpha1.CqlPort, v1alpha1.CqlPort)})
			defer casPf.Close()

			By("Obtaining auth credentials from Secret")
			activeAdminSecret := &v1.Secret{}
			Expect(kubeClient.Get(ctx, types.NamespacedName{Name: names.ActiveAdminSecret(cc.Name), Namespace: cc.Namespace}, activeAdminSecret))

			By("Connecting to Cassandra pod over cql...")
			cluster := gocql.NewCluster("localhost")
			cluster.Port = v1alpha1.CqlPort
			cluster.Keyspace = "system_auth"
			cluster.ConnectTimeout = time.Second * 10
			cluster.Timeout = time.Second * 10
			cluster.Consistency = gocql.LocalQuorum
			cluster.Authenticator = gocql.PasswordAuthenticator{
				Username: string(activeAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole]),
				Password: string(activeAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminPassword]),
			}

			session, err := cluster.CreateSession()
			Expect(err).ToNot(HaveOccurred())
			defer session.Close()

			By("Running cql query: checking Cassandra version...")
			err = session.Query(`SELECT release_version FROM system.local`).Consistency(gocql.LocalQuorum).Scan(&releaseVersion)
			Expect(err).ToNot(HaveOccurred())
			Expect(releaseVersion).To(Equal("3.11.12"))

			By("Running cql query: creating test keyspace...")
			cqlQuery := `CREATE KEYSPACE IF NOT EXISTS %s WITH REPLICATION = { 'class' : 'NetworkTopologyStrategy', '%s' : %v }`
			err = session.Query(fmt.Sprintf(cqlQuery, testRepairReaperKeyspace, cc.Spec.DCs[0].Name, *cc.Spec.DCs[0].Replicas)).Exec()
			Expect(err).ToNot(HaveOccurred())

			By("Running cql query: creating test tables...")
			cqlQuery = `CREATE TABLE %s.%s ( id UUID PRIMARY KEY, lastname text, birthday timestamp, nationality text, weight text, height text )`
			for _, table := range testRepairReaperTables {
				err = session.Query(fmt.Sprintf(cqlQuery, testRepairReaperKeyspace, table)).Exec()
				Expect(err).ToNot(HaveOccurred())
			}

			By("Updating CR: adding reaper schedules...")
			err = kubeClient.Get(ctx, types.NamespacedName{
				Namespace: cc.Namespace,
				Name:      cc.Name,
			}, cc)
			Expect(err).ToNot(HaveOccurred())

			// Generate invalid repair schedule
			reaperRepairSchedules = []v1alpha1.RepairSchedule{
				{
					Keyspace:          testRepairReaperKeyspace,
					Tables:            []string{"test_table1"},
					Datacenters:       dcs,
					IncrementalRepair: true,
					RepairThreadCount: repairThreadCount,
					Intensity:         intensity,
					RepairParallelism: "DATACENTER_AWARE"},
			}

			cc.Spec.Reaper.RepairSchedules = v1alpha1.RepairSchedules{
				Enabled: true,
				Repairs: reaperRepairSchedules,
			}

			By("Validating Webhooks should fail on CR update")
			Expect(kubeClient.Update(ctx, cc)).ToNot(Succeed())

			// Generate valid repair schedules
			reaperRepairSchedules = []v1alpha1.RepairSchedule{
				{
					Keyspace:            testRepairReaperKeyspace,
					Tables:              []string{"test_table1", "test_table2"},
					ScheduleDaysBetween: 7,
					ScheduleTriggerTime: currentTime.AddDate(0, 0, 5).Format(reaperRequestTimeLayout),
					Datacenters:         dcs,
					IncrementalRepair:   false,
					RepairThreadCount:   repairThreadCount,
					Intensity:           intensity,
					RepairParallelism:   repairParallelism,
				},
				{
					Keyspace:            testRepairReaperKeyspace,
					Tables:              []string{"test_table3"},
					ScheduleDaysBetween: 7,
					ScheduleTriggerTime: currentTime.AddDate(0, 0, -5).Format(reaperRequestTimeLayout),
					Datacenters:         dcs,
					IncrementalRepair:   false,
					RepairThreadCount:   repairThreadCount,
					Intensity:           intensity,
					RepairParallelism:   repairParallelism,
				},
				{
					Keyspace:            testRepairReaperKeyspace,
					Tables:              []string{"test_table4", "test_table5"},
					ScheduleDaysBetween: 7,
					ScheduleTriggerTime: currentTime.Add(time.Hour * 2).Format(reaperRequestTimeLayout),
					Datacenters:         dcs,
					IncrementalRepair:   false,
					RepairThreadCount:   repairThreadCount,
					Intensity:           intensity,
					RepairParallelism:   repairParallelism,
				},
			}

			err = kubeClient.Get(ctx, types.NamespacedName{
				Namespace: cc.Namespace,
				Name:      cc.Name,
			}, cc)
			Expect(err).ToNot(HaveOccurred())

			cc.Spec.Reaper.RepairSchedules = v1alpha1.RepairSchedules{
				Enabled: true,
				Repairs: reaperRepairSchedules,
			}

			By("Validating Webhooks should succeed on CR update")
			Expect(kubeClient.Update(ctx, cc)).To(Succeed())

			By("Port forwarding reaper API pod port...")
			reaperPf := portForwardPod(cc.Namespace, labels.Reaper(cc), []string{"9999:8080"})
			defer reaperPf.Close()

			By("Checking cassandra cluster name via reaper API...")
			// Check C* cluster name from reaper API
			respBody, _, err = doHTTPRequest("GET", "http://localhost:9999/cluster/"+cc.Name)
			Expect(err).ToNot(HaveOccurred())
			err = json.Unmarshal(respBody, &responseData)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to parse body %s", string(respBody)))
			Expect(responseData["name"]).To(Equal(cc.Name))

			By("Checking reaper repair job rescheduling logic...")
			// Get response with array of maps with reaper repairs
			Eventually(func() bool {
				respBody, statusCode, err := doHTTPRequest("GET", "http://localhost:9999/repair_schedule?clusterName="+cc.Name+"&keyspace="+testRepairReaperKeyspace)

				if statusCode != 200 || err != nil {
					GinkgoWriter.Printf("/repair_schedule request failed (%d): %s", statusCode, string(respBody))
					return false
				}

				err = json.Unmarshal(respBody, &responsesData)
				Expect(err).ToNot(HaveOccurred())

				if len(responsesData) == 0 {
					GinkgoWriter.Printf("empty response data, raw body: %s", string(respBody))
					return false
				}

				for _, repairSchedule := range reaperRepairSchedules {
					parsedRepairScheduleTime, err := time.Parse(reaperRequestTimeLayout, repairSchedule.ScheduleTriggerTime)
					Expect(err).ToNot(HaveOccurred())

					scheduledRepairsResponse := findFirstMapByKV(responsesData, "column_families", repairSchedule.Tables)
					if scheduledRepairsResponse == nil {
						GinkgoWriter.Printf("couldn't find scheduled repair with column_families=%v. Raw body: %s", repairSchedule.Tables, string(respBody))
						return false
					}

					nextActivation, ok := scheduledRepairsResponse["next_activation"].(string)
					if !ok {
						GinkgoWriter.Printf("next activation is not string: %#v", scheduledRepairsResponse["next_activation"])
						return false
					}

					parsedReaperRespTime, err := time.Parse(reaperResponseTimeLayout, nextActivation)
					Expect(err).ToNot(HaveOccurred())

					GinkgoWriter.Printf("Processing schedule for tables: %v", scheduledRepairsResponse["column_families"])
					testReaperRescheduleTime(parsedRepairScheduleTime, parsedReaperRespTime, currentTime)
				}

				return true
			}, time.Minute*2, time.Second*5).Should(BeTrue(), "All repair schedules from spec should be present in Reaper")

			By("Creating reaper repair job...")
			// Do retry bc tables are not become accessible immediately, error occurs: "Request failed with status code 404. Response body: keyspace doesn't contain a table named test_table1".
			Eventually(func() bool {
				respBody, statusCode, err := doHTTPRequest(
					"POST", "http://localhost:9999/repair_run?clusterName="+cc.Name+
						"&keyspace="+testRepairReaperKeyspace+"&owner=cassandra&tables=test_table6"+
						"&incrementalRepair=false&repairThreadCount="+fmt.Sprintf("%d", repairThreadCount)+
						"&repairParallelism="+repairParallelism+"&incrementalRepair=false&intensity="+intensity+
						"&datacenters="+strings.Join(dcs[:], ",")+"&segmentCount="+fmt.Sprintf("%d", segmentCount))

				if statusCode != 201 || err != nil {
					return false
				}

				return json.Unmarshal(respBody, &responseData) == nil

			}, time.Minute*2, time.Second*5).Should(BeTrue(), "Reaper repair job should be created")

			By("Starting reaper repair job...")
			Eventually(func() bool {
				_, statusCode, err := doHTTPRequest("PUT", "http://localhost:9999/repair_run/"+fmt.Sprintf("%s", responseData["id"])+"/state/RUNNING")
				if statusCode != 200 || err != nil {
					return false
				}

				return true
			}, time.Minute*2, time.Second*5).Should(BeTrue(), "Reaper repair job should be started")

			By("Getting reaper repair job status...")
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

			}, time.Minute*2, time.Second*5).Should(BeTrue(), "Reaper repair job should be running")

		})
	})
})

func testReaperRescheduleTime(repairScheduleTime time.Time, reaperRespTime time.Time, nowTime time.Time) {
	Expect(reaperRespTime.Weekday()).To(BeEquivalentTo(repairScheduleTime.Weekday()), "Week day should match.")

	if reaperRespTime.Year() == repairScheduleTime.Year() {
		Expect(reaperRespTime.YearDay()).To(BeNumerically(">=", repairScheduleTime.YearDay()), "Year day should be equal or greater than scheduled.")
	}

	if reaperRespTime.Year() == nowTime.Year() {
		Expect(reaperRespTime.YearDay()).To(BeNumerically(">=", nowTime.YearDay()), "Year day should be equal or greater than current.")
	}

	Expect(reaperRespTime.Unix()).To(BeNumerically(">=", reaperRespTime.Unix()), "Unix timestamp should be greater or equal to scheduled.")
	Expect(reaperRespTime.Unix()).To(BeNumerically(">=", nowTime.Unix()), "Unix timestamp should be greater or equal to current.")
	Expect(reaperRespTime.YearDay()-repairScheduleTime.YearDay()).To(BeNumerically("<=", 7), "Year day difference should be less or equal 7.")
}
