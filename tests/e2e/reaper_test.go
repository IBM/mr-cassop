package e2e

import (
	"context"
	"fmt"
	"github.com/gocql/gocql"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"encoding/json"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"strings"
	"time"
)

// We call init function from assigned `Describe` function, in such way we can avoid using init() {} in this file
var _ = Describe("Cassandra cluster", func() {

	Context("When reaper repair schedules are enabled and set", func() {
		It("should be enabled and all set", func() {
			var (
				currentTime                      = time.Now().UTC()
				testScheduleReaperKeyspace       = "reaper_db"
				reaperRequestTimeLayout          = "2006-01-02T15:04:05"
				reaperResponseTimeLayout         = "2006-01-02T15:04:05Z"
				testRepairReaperKeyspace         = "cycling"
				testRepairReaperTable            = "cyclist_alt_stats"
				intensity                        = "1.0"
				repairParallelism                = "sequential"
				segmentCount                     = 10
				repairThreadCount          int32 = 4
				reaperRepairs              []v1alpha1.Repair
				respBody                   []byte
				responseData               map[string]interface{}
				responsesData              []map[string]interface{}
				releaseVersion             string
				dcs                        []string
			)

			for _, dc := range cassandraDCs {
				dcs = append(dcs, dc.Name)
			}

			// Generate repair schedules
			reaperRepairs = append(reaperRepairs,
				v1alpha1.Repair{
					Keyspace:            testScheduleReaperKeyspace,
					Owner:               "cassandra",
					Tables:              []string{"snapshot"},
					ScheduleDaysBetween: 7,
					ScheduleTriggerTime: currentTime.AddDate(0, 0, 5).Format(reaperRequestTimeLayout),
					Datacenters:         dcs,
					IncrementalRepair:   false,
					RepairThreadCount:   repairThreadCount,
					Intensity:           intensity,
					RepairParallelism:   repairParallelism,
				},
				v1alpha1.Repair{
					Keyspace:            testScheduleReaperKeyspace,
					Owner:               "cassandra",
					Tables:              []string{"schema_migration"},
					ScheduleDaysBetween: 7,
					ScheduleTriggerTime: currentTime.AddDate(0, 0, -5).Format(reaperRequestTimeLayout),
					Datacenters:         dcs,
					IncrementalRepair:   false,
					RepairThreadCount:   repairThreadCount,
					Intensity:           intensity,
					RepairParallelism:   repairParallelism,
				},
				v1alpha1.Repair{
					Keyspace:            testScheduleReaperKeyspace,
					Owner:               "cassandra",
					Tables:              []string{"cluster"},
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
				Keyspace:                               testScheduleReaperKeyspace,
				DCs:                                    cassandraDCs,
				DatacenterAvailability:                 "each",
				IncrementalRepair:                      false,
				RepairIntensity:                        "1.0",
				RepairManagerSchedulingIntervalSeconds: 0,
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceMemory: resource.MustParse("512Mi"),
						v1.ResourceCPU:    resource.MustParse("1"),
					},
					Requests: v1.ResourceList{
						v1.ResourceMemory: resource.MustParse("512Mi"),
						v1.ResourceCPU:    resource.MustParse("1"),
					},
				},
				ScheduleRepairs: v1alpha1.ScheduleRepairs{
					Enabled:        true,
					StartRepairsIn: "60 minutes",
					Repairs:        reaperRepairs,
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

			By("Connecting to Cassandra pod over cql...")
			cluster := gocql.NewCluster("localhost")
			cluster.Port = v1alpha1.CqlPort
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
			err = session.Query(`SELECT release_version FROM system.local`).Consistency(gocql.LocalQuorum).Scan(&releaseVersion)
			Expect(err).ToNot(HaveOccurred())

			cassandraVersion := fmt.Sprintf(strings.Split(cassandraImage, ":")[1])
			cassandraVersion = fmt.Sprintf(strings.Split(cassandraVersion, "-")[0])
			Expect(releaseVersion).To(Equal(cassandraVersion))

			By("Running cql query: creating test keyspace...")
			cqlQuery := `CREATE KEYSPACE IF NOT EXISTS %s WITH REPLICATION = { 'class' : 'NetworkTopologyStrategy', '%s' : 3 }`
			err = session.Query(fmt.Sprintf(cqlQuery, testRepairReaperKeyspace, cassandraDCs[0].Name)).Exec()
			Expect(err).ToNot(HaveOccurred())

			By("Running cql query: creating test table...")
			cqlQuery = `CREATE TABLE %s.%s ( id UUID PRIMARY KEY, lastname text, birthday timestamp, nationality text, weight text, height text )`
			err = session.Query(fmt.Sprintf(cqlQuery, testRepairReaperKeyspace, testRepairReaperTable)).Exec()
			Expect(err).ToNot(HaveOccurred())

			By("Running default rack name check...")
			podList := &v1.PodList{}
			err = restClient.List(context.Background(), podList, client.InNamespace(cassandraNamespace), client.MatchingLabels(cassandraClusterPodLabels))
			Expect(err).ToNot(HaveOccurred())

			cmd := []string{
				"sh",
				"-c",
				"cat /etc/cassandra/cassandra-rackdc.properties | grep rack= | cut -f2 -d'='",
			}

			for _, p := range podList.Items {
				podName := p.Name
				r := execPod(podName, cassandraNamespace, cmd)

				r.stdout = strings.TrimSuffix(r.stdout, "\n")
				r.stdout = strings.TrimSpace(r.stdout)
				Expect(len(r.stderr)).To(Equal(0))
				Expect(r.stdout).To(Equal("rack1"))
			}

			By("Port forwarding reaper API pod port...")
			reaperPf := portForwardPod(cassandraNamespace, reaperPodLabels, []string{"9999:8080"})
			defer reaperPf.Close()

			By("Checking cassandra cluster name in reaper...")
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
						"&keyspace="+testRepairReaperKeyspace+"&owner=cassandra&tables="+testRepairReaperTable+
						"&incrementalRepair=false&repairThreadCount="+fmt.Sprintf("%d", repairThreadCount)+
						"&repairParallelism="+repairParallelism+"&incrementalRepair=false&intensity="+intensity+
						"&datacenters="+strings.Join(dcs[:], ",")+"&segmentCount="+fmt.Sprintf("%d", segmentCount))
				if statusCode != 201 || err != nil {
					return false
				}

				err = json.Unmarshal(respBody, &responseData)
				return err == nil

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
				respBody, _, err = doHTTPRequest("GET", "http://localhost:9999/repair_run/"+fmt.Sprintf("%s", responseData["id"]))
				if err != nil {
					return false
				}

				err = json.Unmarshal(respBody, &responseData)
				if err != nil {
					return false
				}

				if responseData["state"] != "DONE" {
					return false
				}

				return true
			}, time.Minute*15, time.Second*15).Should(BeTrue(), "Reaper job should be finished")

			By("Checking reaper job rescheduling logic...")
			// Get response with array of maps with reaper repairs
			Eventually(func() bool {
				respBody, statusCode, err = doHTTPRequest("GET", "http://localhost:9999/repair_schedule?clusterName="+cassandraRelease+"&keyspace="+testScheduleReaperKeyspace)
				if statusCode != 200 || err != nil {
					return false
				}

				err = json.Unmarshal(respBody, &responsesData)
				Expect(err).ToNot(HaveOccurred())

				return len(responsesData) != 0

			}, time.Minute*2, time.Second*5).Should(BeTrue(), "Reaper repairs should be received.")

			By("Checking rescheduled times...")
			for _, reaperRepair := range reaperRepairs {
				parsedReaperReqTime, err := time.Parse(reaperRequestTimeLayout, reaperRepair.ScheduleTriggerTime)
				Expect(err).ToNot(HaveOccurred())

				repairResponse := findFirstMapByKV(responsesData, "column_families", reaperRepair.Tables[0])
				parsedReaperRespTime, err := time.Parse(reaperResponseTimeLayout, repairResponse["next_activation"].(string))
				Expect(err).ToNot(HaveOccurred())

				fmt.Println("Processing schedule for tables: ", repairResponse["column_families"], "...")
				testReaperRescheduleTime(parsedReaperReqTime, parsedReaperRespTime, currentTime)
			}
		})
	})
})
