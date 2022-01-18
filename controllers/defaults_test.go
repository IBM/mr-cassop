package controllers

import (
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/config"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
)

func TestDefaultingFunction(t *testing.T) {
	g := NewGomegaWithT(t)
	reconciler := &CassandraClusterReconciler{
		Cfg: config.Config{
			DefaultProberImage:    "prober/image",
			DefaultJolokiaImage:   "jolokia/image",
			DefaultCassandraImage: "cassandra/image",
			DefaultReaperImage:    "reaper/image",
		},
	}

	cc := &v1alpha1.CassandraCluster{}
	reconciler.defaultCassandraCluster(cc)
	g.Expect(cc.Spec.CQLConfigMapLabelKey).To(Equal(defaultCQLConfigMapLabelKey))
	g.Expect(cc.Spec.TopologySpreadByZone).ToNot(BeNil())
	g.Expect(*cc.Spec.TopologySpreadByZone).To(BeTrue())
	g.Expect(cc.Spec.Cassandra).ToNot(BeNil())
	g.Expect(cc.Spec.Cassandra.Image).To(Equal("cassandra/image"))
	g.Expect(cc.Spec.Cassandra.ImagePullPolicy).To(Equal(v1.PullIfNotPresent))
	g.Expect(cc.Spec.Cassandra.NumSeeds).To(Equal(int32(2)))
	g.Expect(cc.Spec.Prober.Image).To(Equal("prober/image"))
	g.Expect(cc.Spec.Prober.ImagePullPolicy).To(Equal(v1.PullIfNotPresent))
	g.Expect(cc.Spec.Prober.Jolokia.Image).To(Equal("jolokia/image"))
	g.Expect(cc.Spec.Prober.Jolokia.ImagePullPolicy).To(Equal(v1.PullIfNotPresent))
	g.Expect(cc.Spec.Reaper).ToNot(BeNil())
	g.Expect(cc.Spec.Reaper.Image).To(Equal("reaper/image"))
	g.Expect(cc.Spec.Reaper.ImagePullPolicy).To(Equal(v1.PullIfNotPresent))
	g.Expect(cc.Spec.Reaper.Keyspace).To(Equal("reaper"))
	g.Expect(cc.Spec.Reaper.Tolerations).To(BeNil())
	g.Expect(cc.Spec.Reaper.NodeSelector).To(BeNil())
	g.Expect(cc.Spec.Reaper.RepairIntensity).To(Equal("1.0"))
	g.Expect(cc.Spec.Reaper.ServiceMonitor.Enabled).To(BeFalse())
	g.Expect(cc.Spec.Reaper.ServiceMonitor.Namespace).To(BeEmpty())
	g.Expect(cc.Spec.Reaper.ServiceMonitor.Labels).To(BeEmpty())
	g.Expect(cc.Spec.Reaper.ServiceMonitor.ScrapeInterval).To(BeEmpty())
	g.Expect(cc.Spec.Maintenance).To(BeNil())
	g.Expect(cc.Status.MaintenanceState).To(BeNil())
	g.Expect(cc.Spec.Encryption.Server.InternodeEncryption).To(Equal(internodeEncryptionNone))
	g.Expect(cc.Spec.Encryption.Client.Enabled).To(BeFalse())
	g.Expect(cc.Spec.Monitoring.Enabled).To(BeFalse())
	g.Expect(cc.Spec.Monitoring.Agent).To(BeEmpty())
	g.Expect(cc.Spec.Monitoring.ServiceMonitor.Enabled).To(BeFalse())
	g.Expect(cc.Spec.Monitoring.ServiceMonitor.Namespace).To(BeEmpty())
	g.Expect(cc.Spec.Monitoring.ServiceMonitor.Labels).To(BeEmpty())
	g.Expect(cc.Spec.Monitoring.ServiceMonitor.ScrapeInterval).To(BeEmpty())
	g.Expect(cc.Spec.Cassandra.Sysctls).To(Equal(map[string]string{
		"net.ipv4.ip_local_port_range": "1025 65535",
		"net.ipv4.tcp_rmem":            "4096 87380 16777216",
		"net.ipv4.tcp_wmem":            "4096 65536 16777216",
		"net.core.somaxconn":           "65000",
		"net.ipv4.tcp_ecn":             "0",
		"net.ipv4.tcp_window_scaling":  "1",
		"vm.dirty_background_bytes":    "10485760",
		"vm.dirty_bytes":               "1073741824",
		"vm.zone_reclaim_mode":         "0",
		"fs.file-max":                  "1073741824",
		"vm.max_map_count":             "1073741824",
		"vm.swappiness":                "1",
	}))

	cc = &v1alpha1.CassandraCluster{
		Spec: v1alpha1.CassandraClusterSpec{
			DCs: []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(3),
				},
			},
			Cassandra: &v1alpha1.Cassandra{
				Sysctls: map[string]string{
					"fs.file-max":      "1000000000",
					"vm.max_map_count": "1000000000",
				},
			},
			Reaper: &v1alpha1.Reaper{
				RepairSchedules: v1alpha1.RepairSchedules{
					Repairs: []v1alpha1.RepairSchedule{
						{
							Keyspace: "system_auth",
						},
					},
				},
				ServiceMonitor: v1alpha1.ServiceMonitor{
					Enabled:   true,
					Namespace: "",
				},
			},
			Maintenance: []v1alpha1.Maintenance{
				{
					DC: "dc1",
				},
			},
			Encryption: v1alpha1.Encryption{
				Server: v1alpha1.ServerEncryption{
					InternodeEncryption: "dc",
					TLSSecret: v1alpha1.TLSSecret{
						Name:                  "server-tls-secret",
						KeystoreFileKey:       "test.jks",
						KeystorePasswordKey:   "test.txt",
						TruststoreFileKey:     "test.jks",
						TruststorePasswordKey: "test.txt",
					},
					RequireClientAuth:           proto.Bool(false),
					RequireEndpointVerification: true,
					Protocol:                    "TLS",
					Algorithm:                   "SunX509",
					StoreType:                   "PKCS12",
					CipherSuites:                []string{tlsEcdheRsaAes128GcmSha256},
				},
				Client: v1alpha1.ClientEncryption{
					Enabled:  true,
					Optional: true,
					TLSSecret: v1alpha1.ClientTLSSecret{
						TLSSecret: v1alpha1.TLSSecret{
							Name:                  "client-tls-secret",
							KeystoreFileKey:       "test.jks",
							KeystorePasswordKey:   "test.txt",
							TruststoreFileKey:     "test.jks",
							TruststorePasswordKey: "test.txt",
						},
						CAFileKey:     "ca.crt",
						TLSFileKey:    "tls.key",
						TLSCrtFileKey: "tls.crt",
					},
					RequireClientAuth: proto.Bool(false),
					Protocol:          "TLS",
					Algorithm:         "SunX509",
					StoreType:         "PKCS12",
					CipherSuites:      []string{tlsEcdheRsaAes128GcmSha256},
				},
			},
			Monitoring: v1alpha1.Monitoring{
				Enabled: true,
				ServiceMonitor: v1alpha1.ServiceMonitor{
					Enabled:   true,
					Namespace: "",
				},
			},
			TopologySpreadByZone: proto.Bool(false),
		},
	}
	reconciler.defaultCassandraCluster(cc)
	g.Expect(cc.Spec.DCs[0].Tolerations).To(BeNil())
	g.Expect(cc.Spec.DCs[0].Affinity).To(BeNil())
	g.Expect(cc.Spec.SystemKeyspaces.DCs).To(BeNil())
	g.Expect(cc.Spec.TopologySpreadByZone).ToNot(BeNil())
	g.Expect(*cc.Spec.TopologySpreadByZone).To(BeFalse())
	g.Expect(cc.Spec.Cassandra.Sysctls).To(Equal(map[string]string{
		"net.ipv4.ip_local_port_range": "1025 65535",
		"net.ipv4.tcp_rmem":            "4096 87380 16777216",
		"net.ipv4.tcp_wmem":            "4096 65536 16777216",
		"net.core.somaxconn":           "65000",
		"net.ipv4.tcp_ecn":             "0",
		"net.ipv4.tcp_window_scaling":  "1",
		"vm.dirty_background_bytes":    "10485760",
		"vm.dirty_bytes":               "1073741824",
		"vm.zone_reclaim_mode":         "0",
		"fs.file-max":                  "1000000000",
		"vm.max_map_count":             "1000000000",
		"vm.swappiness":                "1",
	}))

	// Reaper
	g.Expect(cc.Spec.Reaper.RepairSchedules.Repairs[0].Keyspace).To(Equal("system_auth"))
	g.Expect(cc.Spec.Reaper.RepairSchedules.Repairs[0].RepairParallelism).To(Equal("DATACENTER_AWARE"))
	g.Expect(cc.Spec.Reaper.RepairSchedules.Repairs[0].ScheduleDaysBetween).To(Equal(int32(7)))
	g.Expect(cc.Spec.Reaper.RepairSchedules.Repairs[0].Datacenters).To(Equal([]string{"dc1"}))
	g.Expect(cc.Spec.Reaper.RepairSchedules.Repairs[0].RepairThreadCount).To(Equal(int32(2)))
	g.Expect(cc.Spec.Reaper.ServiceMonitor.Enabled).To(BeTrue())
	g.Expect(cc.Spec.Reaper.ServiceMonitor.Labels).To(BeEmpty())
	g.Expect(cc.Spec.Reaper.ServiceMonitor.ScrapeInterval).To(BeEquivalentTo("60s"))

	// Maintenance mode
	g.Expect(cc.Spec.Maintenance[0].DC).To(Equal("dc1"))
	g.Expect(cc.Spec.Maintenance[0].Pods).ToNot(BeEmpty())

	// Server encryption
	g.Expect(cc.Spec.Encryption.Server.InternodeEncryption).To(Equal("dc"))
	g.Expect(cc.Spec.Encryption.Server.TLSSecret.Name).To(Equal("server-tls-secret"))
	g.Expect(cc.Spec.Encryption.Server.TLSSecret.KeystoreFileKey).To(Equal("test.jks"))
	g.Expect(cc.Spec.Encryption.Server.TLSSecret.KeystorePasswordKey).To(Equal("test.txt"))
	g.Expect(cc.Spec.Encryption.Server.TLSSecret.TruststoreFileKey).To(Equal("test.jks"))
	g.Expect(cc.Spec.Encryption.Server.TLSSecret.TruststorePasswordKey).To(BeEquivalentTo("test.txt"))
	g.Expect(cc.Spec.Encryption.Server.Protocol).To(BeEquivalentTo("TLS"))
	g.Expect(cc.Spec.Encryption.Server.Algorithm).To(BeEquivalentTo("SunX509"))
	g.Expect(cc.Spec.Encryption.Server.StoreType).To(BeEquivalentTo("PKCS12"))
	g.Expect(cc.Spec.Encryption.Server.CipherSuites).To(BeEquivalentTo([]string{tlsEcdheRsaAes128GcmSha256}))
	g.Expect(cc.Spec.Encryption.Server.RequireClientAuth).To(BeEquivalentTo(proto.Bool(false)))
	g.Expect(cc.Spec.Encryption.Server.RequireEndpointVerification).To(BeTrue())

	// Client encryption
	g.Expect(cc.Spec.Encryption.Client.Enabled).To(BeTrue())
	g.Expect(cc.Spec.Encryption.Client.TLSSecret.Name).To(Equal("client-tls-secret"))
	g.Expect(cc.Spec.Encryption.Client.TLSSecret.KeystoreFileKey).To(Equal("test.jks"))
	g.Expect(cc.Spec.Encryption.Client.TLSSecret.KeystorePasswordKey).To(Equal("test.txt"))
	g.Expect(cc.Spec.Encryption.Client.TLSSecret.TruststoreFileKey).To(Equal("test.jks"))
	g.Expect(cc.Spec.Encryption.Client.TLSSecret.TruststorePasswordKey).To(BeEquivalentTo("test.txt"))
	g.Expect(cc.Spec.Encryption.Client.Protocol).To(BeEquivalentTo("TLS"))
	g.Expect(cc.Spec.Encryption.Client.Algorithm).To(BeEquivalentTo("SunX509"))
	g.Expect(cc.Spec.Encryption.Client.StoreType).To(BeEquivalentTo("PKCS12"))
	g.Expect(cc.Spec.Encryption.Client.CipherSuites).To(BeEquivalentTo([]string{tlsEcdheRsaAes128GcmSha256}))
	g.Expect(cc.Spec.Encryption.Client.RequireClientAuth).To(BeEquivalentTo(proto.Bool(false)))

	// Monitoring
	g.Expect(cc.Spec.Monitoring.Enabled).To(BeTrue())
	g.Expect(cc.Spec.Monitoring.Agent).To(BeEquivalentTo("tlp"))
	g.Expect(cc.Spec.Monitoring.ServiceMonitor.Enabled).To(BeTrue())
	g.Expect(cc.Spec.Monitoring.ServiceMonitor.Labels).To(BeEmpty())
	g.Expect(cc.Spec.Monitoring.ServiceMonitor.ScrapeInterval).To(BeEquivalentTo("30s"))
}
