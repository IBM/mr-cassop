package e2e

import (
	"fmt"
	"net/http"
	"time"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/expfmt"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

var _ = Describe("Cassandra cluster", func() {
	Context("when C* monitoring is enabled with tlp exporter", func() {
		It("should be able to get C* metrics on tlp port", func() {
			newCassandraCluster := cassandraCluster.DeepCopy()
			newCassandraCluster.Spec.Cassandra.Monitoring.Enabled = true

			deployCassandraCluster(newCassandraCluster)

			By("checking reaper pods readiness...")
			for _, dc := range newCassandraCluster.Spec.DCs {
				waitForPodsReadiness(cassandraNamespace, labels.WithDCLabel(reaperPodLabels, dc.Name), 1)
			}

			By("check tlp exporter port")
			pf := portForwardPod(cassandraNamespace, cassandraClusterPodLabels, []string{fmt.Sprintf("%d:%d", dbv1alpha1.TlpPort, dbv1alpha1.TlpPort)})
			var resp *http.Response
			Eventually(func() (int, error) {
				var err error
				resp, err = http.Get(fmt.Sprintf("http://localhost:%d", dbv1alpha1.TlpPort))
				if err != nil {
					return 0, err
				}
				return resp.StatusCode, nil
			}, time.Second*20, time.Second*2).Should(Equal(200))

			By("read metrics from C* pod metrics exporter")
			var parser expfmt.TextParser
			metricFamilies, err := parser.TextToMetricFamilies(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			By("ClientRequest TotalLatency Read metric")
			metric, found := getMetricByLabel(metricFamilies, "org_apache_cassandra_metrics_ClientRequest_TotalLatency", "scope", "Read")
			Expect(found).To(BeTrue())
			Expect(*metric.Untyped.Value).To(BeNumerically(">=", 0))

			By("ClientRequest TotalLatency Write metric")
			metric, found = getMetricByLabel(metricFamilies, "org_apache_cassandra_metrics_ClientRequest_TotalLatency", "scope", "Write")
			Expect(found).To(BeTrue())
			Expect(*metric.Untyped.Value).To(BeNumerically(">=", 0))

			By("Client connectedNativeClients metric")
			metric, found = getMetric(metricFamilies, "org_apache_cassandra_metrics_Client_connectedNativeClients")
			Expect(found).To(BeTrue())
			Expect(*metric.Untyped.Value).To(BeNumerically(">=", 0))

			By("HeapMemoryUsage committed metric")
			metric, found = getMetric(metricFamilies, "java_lang_Memory_HeapMemoryUsage_committed")
			Expect(found).To(BeTrue())
			Expect(*metric.Untyped.Value).To(BeNumerically(">=", 1))

			By("Runtime StartTime metric")
			metric, found = getMetric(metricFamilies, "java_lang_Runtime_StartTime")
			Expect(found).To(BeTrue())
			Expect(*metric.Untyped.Value).To(BeNumerically(">=", 1))

			pf.Close()

			By("check reaper admin port")
			pf = portForwardPod(cassandraNamespace, reaperPodLabels, []string{fmt.Sprintf("%d:%d", dbv1alpha1.ReaperAdminPort, dbv1alpha1.ReaperAdminPort)})
			defer pf.Close()
			resp = &http.Response{}
			Eventually(func() (bool, error) {
				var err error
				resp, err = http.Get(fmt.Sprintf("http://localhost:%d/prometheusMetrics", dbv1alpha1.ReaperAdminPort))
				if err != nil {
					return false, err
				}

				By("read metrics from reaper pod metrics exporter")
				parser = expfmt.TextParser{}
				metricFamilies, err = parser.TextToMetricFamilies(resp.Body)
				if err != nil {
					return false, err
				}

				// Get the first metrics with retries to not fail if the metrics returned no error but not yet appeared
				By("SegmentRunner Renew Lead metric")
				metric, found = getMetric(metricFamilies, "io_cassandrareaper_service_SegmentRunner_renewLead")
				if !found {
					return false, nil
				}

				return true, nil
			}, time.Second*20, time.Second*2).Should(BeTrue(), "should have returned non empty metrics")

			By("SegmentRunner Renew Lead metric")
			metric, found = getMetric(metricFamilies, "io_cassandrareaper_service_SegmentRunner_renewLead")
			Expect(found).To(BeTrue())
			Expect(*metric.Summary.SampleCount).To(BeNumerically(">=", 0))

			By("SegmentRunner Take Lead metric")
			metric, found = getMetric(metricFamilies, "io_cassandrareaper_service_SegmentRunner_takeLead")
			Expect(found).To(BeTrue())
			Expect(*metric.Summary.SampleCount).To(BeNumerically(">=", 0))

			By("Jmx Connections metric")
			metric, found = getMetric(metricFamilies, "io_cassandrareaper_jmx_JmxConnectionFactory_jmxConnectionsIntializer")
			Expect(found).To(BeTrue())
			Expect(*metric.Summary.SampleCount).To(BeNumerically(">=", 1))

			By("Memory Heap Committed metric")
			metric, found = getMetric(metricFamilies, "jvm_memory_heap_committed")
			Expect(found).To(BeTrue())
			Expect(*metric.Gauge.Value).To(BeNumerically(">=", 1))
		})
	})

	Context("when C* monitoring is enabled with datastax exporter", func() {
		It("should be able to get C* metrics on datastax port", func() {
			newCassandraCluster := cassandraCluster.DeepCopy()
			newCassandraCluster.Spec.Cassandra.Monitoring.Enabled = true
			newCassandraCluster.Spec.Cassandra.Monitoring.Agent = dbv1alpha1.CassandraAgentDatastax

			deployCassandraCluster(newCassandraCluster)

			By("check datastax exporter port")
			pf := portForwardPod(cassandraNamespace, cassandraClusterPodLabels, []string{fmt.Sprintf("%d:%d", dbv1alpha1.DatastaxPort, dbv1alpha1.DatastaxPort)})
			defer pf.Close()
			var resp *http.Response
			Eventually(func() (int, error) {
				var err error
				resp, err = http.Get(fmt.Sprintf("http://localhost:%d", dbv1alpha1.DatastaxPort))
				if err != nil {
					return 0, err
				}
				return resp.StatusCode, nil
			}, time.Second*20, time.Second*2).Should(Equal(200))

			By("read metrics from C* pod metrics exporter")
			var parser expfmt.TextParser
			metricFamilies, err := parser.TextToMetricFamilies(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			By("ClientRequest TotalLatency Read metric")
			metric, found := getMetricByLabel(metricFamilies, "collectd_mcac_counter_total", "mcac", "org.apache.cassandra.metrics.client_request.total_latency.read")
			Expect(found).To(BeTrue())
			Expect(*metric.Counter.Value).To(BeNumerically(">=", 0))

			By("ClientRequest TotalLatency Write metric")
			metric, found = getMetricByLabel(metricFamilies, "collectd_mcac_counter_total", "mcac", "org.apache.cassandra.metrics.client_request.total_latency.write")
			Expect(found).To(BeTrue())
			Expect(*metric.Counter.Value).To(BeNumerically(">=", 0))

			By("Client connectedNativeClients metric")
			metric, found = getMetricByLabel(metricFamilies, "collectd_mcac_gauge", "mcac", "org.apache.cassandra.metrics.client.connected_native_clients")
			Expect(found).To(BeTrue())
			Expect(*metric.Gauge.Value).To(BeNumerically(">=", 0))

			By("Jvm Buffer Capacity metric")
			metric, found = getMetricByLabel(metricFamilies, "collectd_mcac_gauge", "mcac", "jvm.buffers.direct.capacity")
			Expect(found).To(BeTrue())
			Expect(*metric.Gauge.Value).To(BeNumerically(">=", 1))

			By("Jvm Memory Heap metric")
			metric, found = getMetricByLabel(metricFamilies, "collectd_mcac_gauge", "mcac", "jvm.memory.heap.committed")
			Expect(found).To(BeTrue())
			Expect(*metric.Gauge.Value).To(BeNumerically(">=", 1))
		})
	})

	Context("when C* monitoring is enabled with instaclustr exporter", func() {
		It("should be able to get C* metrics on instaclustr port", func() {
			newCassandraCluster := cassandraCluster.DeepCopy()
			newCassandraCluster.Spec.Cassandra.Monitoring.Enabled = true
			newCassandraCluster.Spec.Cassandra.Monitoring.Agent = dbv1alpha1.CassandraAgentInstaclustr

			deployCassandraCluster(newCassandraCluster)

			By("check instaclustr exporter port")
			pf := portForwardPod(cassandraNamespace, cassandraClusterPodLabels, []string{fmt.Sprintf("%d:%d", dbv1alpha1.InstaclustrPort, dbv1alpha1.InstaclustrPort)})
			defer pf.Close()
			var resp *http.Response
			Eventually(func() (int, error) {
				var err error
				resp, err = http.Get(fmt.Sprintf("http://localhost:%d/metrics", dbv1alpha1.InstaclustrPort))
				if err != nil {
					return 0, err
				}
				return resp.StatusCode, nil
			}, time.Second*20, time.Second*2).Should(Equal(200))

			By("read metrics from C* pod metrics exporter")
			var parser expfmt.TextParser
			metricFamilies, err := parser.TextToMetricFamilies(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			By("ClientRequest TotalLatency Read metric")
			metric, found := getMetricByLabel(metricFamilies, "cassandra_client_request_latency_seconds", "operation", "read")
			Expect(found).To(BeTrue())
			Expect(*metric.Summary.SampleCount).To(BeNumerically(">=", 0))

			By("ClientRequest TotalLatency Write metric")
			metric, found = getMetricByLabel(metricFamilies, "cassandra_client_request_latency_seconds", "operation", "write")
			Expect(found).To(BeTrue())
			Expect(*metric.Summary.SampleCount).To(BeNumerically(">=", 0))

			By("Active Endpoints metric")
			metrics, found := getMetrics(metricFamilies, "cassandra_endpoint_active")
			Expect(found).To(BeTrue())
			totalClients := 0
			for _, m := range metrics {
				totalClients += int(*m.Gauge.Value)
			}
			Expect(totalClients).To(BeNumerically("==", 3))

			By("Storage Load Bytes metric")
			metric, found = getMetric(metricFamilies, "cassandra_storage_load_bytes")
			Expect(found).To(BeTrue())
			Expect(*metric.Gauge.Value).To(BeNumerically(">=", 1))

			By("Storage Hints Total metric")
			metric, found = getMetric(metricFamilies, "cassandra_storage_hints_total")
			Expect(found).To(BeTrue())
			Expect(*metric.Counter.Value).To(BeNumerically(">=", 0))
		})
	})
})
