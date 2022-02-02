package controllers

import (
	"fmt"
	"time"

	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
	v1 "k8s.io/api/core/v1"
)

func (r *CassandraClusterReconciler) defaultCassandraCluster(cc *dbv1alpha1.CassandraCluster) {
	if cc.Spec.CQLConfigMapLabelKey == "" {
		cc.Spec.CQLConfigMapLabelKey = defaultCQLConfigMapLabelKey
	}

	r.defaultCassandra(cc)
	r.defaultProber(cc)
	r.defaultReaper(cc)

	if len(cc.Spec.Maintenance) > 0 {
		for i, entry := range cc.Spec.Maintenance {
			if len(entry.Pods) == 0 {
				for _, dc := range cc.Spec.DCs {
					if entry.DC == dc.Name {
						for j := 0; j < int(*dc.Replicas); j++ {
							cc.Spec.Maintenance[i].Pods = append(cc.Spec.Maintenance[i].Pods, dbv1alpha1.PodName(fmt.Sprintf("%s-%d", names.DC(cc.Name, dc.Name), j)))
						}
					}
				}
			}
		}
	}

	if cc.Spec.JMXAuth == "" {
		cc.Spec.JMXAuth = jmxAuthenticationInternal
	}

	if cc.Spec.TopologySpreadByZone == nil {
		cc.Spec.TopologySpreadByZone = proto.Bool(true)
	}

	r.defaultServerTLS(cc)
	r.defaultClientTLS(cc)
	r.defaultSysctls(cc)
}

func (r *CassandraClusterReconciler) defaultReaper(cc *dbv1alpha1.CassandraCluster) {
	if cc.Spec.Reaper == nil {
		cc.Spec.Reaper = &dbv1alpha1.Reaper{}
	}

	if cc.Spec.Reaper.ServiceMonitor.Enabled {
		if _, err := time.ParseDuration(cc.Spec.Reaper.ServiceMonitor.ScrapeInterval); err != nil {
			cc.Spec.Reaper.ServiceMonitor.ScrapeInterval = "60s"
		}
	}

	if cc.Spec.Reaper.Keyspace == "" {
		cc.Spec.Reaper.Keyspace = "reaper"
	}

	if cc.Spec.Reaper.Image == "" {
		cc.Spec.Reaper.Image = r.Cfg.DefaultReaperImage
	}

	if cc.Spec.Reaper.ImagePullPolicy == "" {
		cc.Spec.Reaper.ImagePullPolicy = v1.PullIfNotPresent
	}

	if cc.Spec.Reaper.RepairIntensity == "" {
		cc.Spec.Reaper.RepairIntensity = "1.0"
	}

	if cc.Spec.Reaper.RepairParallelism == "" {
		cc.Spec.Reaper.RepairParallelism = "DATACENTER_AWARE"
	}

	if cc.Spec.Reaper.RepairThreadCount == 0 {
		cc.Spec.Reaper.RepairThreadCount = 1
	}

	if len(cc.Spec.Reaper.Tolerations) == 0 {
		cc.Spec.Reaper.Tolerations = nil
	}

	if len(cc.Spec.Reaper.NodeSelector) == 0 {
		cc.Spec.Reaper.NodeSelector = nil
	}

	if len(cc.Spec.Reaper.RepairSchedules.Repairs) != 0 {
		for i, repair := range cc.Spec.Reaper.RepairSchedules.Repairs {
			if repair.RepairParallelism == "" {
				cc.Spec.Reaper.RepairSchedules.Repairs[i].RepairParallelism = "DATACENTER_AWARE"
			}

			// RepairParallelism must be 'parallel' if IncrementalRepair is 'true'
			if repair.IncrementalRepair {
				cc.Spec.Reaper.RepairSchedules.Repairs[i].RepairParallelism = "PARALLEL"
			}

			if repair.ScheduleDaysBetween == 0 {
				cc.Spec.Reaper.RepairSchedules.Repairs[i].ScheduleDaysBetween = 7
			}

			if repair.Intensity == "" {
				cc.Spec.Reaper.RepairSchedules.Repairs[i].Intensity = "1.0"
			}

			// Can't set both `nodes` and `datacenters` so set defaults for `datacenters` only if `nodes` not set
			if len(repair.Datacenters) == 0 && len(repair.Nodes) == 0 {
				dcNames := make([]string, 0, len(cc.Spec.DCs))
				for _, dc := range cc.Spec.DCs {
					dcNames = append(dcNames, dc.Name)
				}
				cc.Spec.Reaper.RepairSchedules.Repairs[i].Datacenters = dcNames
			}
			if repair.RepairThreadCount == 0 {
				cc.Spec.Reaper.RepairSchedules.Repairs[i].RepairThreadCount = 2
			}
		}
	}
}

func (r *CassandraClusterReconciler) defaultProber(cc *dbv1alpha1.CassandraCluster) {
	if cc.Spec.Prober.Image == "" {
		cc.Spec.Prober.Image = r.Cfg.DefaultProberImage
	}

	if cc.Spec.Prober.ImagePullPolicy == "" {
		cc.Spec.Prober.ImagePullPolicy = v1.PullIfNotPresent
	}

	if cc.Spec.Prober.LogLevel == "" {
		cc.Spec.Prober.LogLevel = "info"
	}

	if cc.Spec.Prober.LogFormat == "" {
		cc.Spec.Prober.LogFormat = "json"
	}

	if cc.Spec.Prober.Jolokia.Image == "" {
		cc.Spec.Prober.Jolokia.Image = r.Cfg.DefaultJolokiaImage
	}

	if cc.Spec.Prober.Jolokia.ImagePullPolicy == "" {
		cc.Spec.Prober.Jolokia.ImagePullPolicy = v1.PullIfNotPresent
	}

	if cc.Spec.Prober.ServiceMonitor.Enabled {
		if _, err := time.ParseDuration(cc.Spec.Prober.ServiceMonitor.ScrapeInterval); err != nil {
			cc.Spec.Prober.ServiceMonitor.ScrapeInterval = "30s"
		}
	}
}

func (r *CassandraClusterReconciler) defaultCassandra(cc *dbv1alpha1.CassandraCluster) {
	if cc.Spec.Cassandra == nil {
		cc.Spec.Cassandra = &dbv1alpha1.Cassandra{}
	}

	if cc.Spec.Cassandra.Image == "" {
		cc.Spec.Cassandra.Image = r.Cfg.DefaultCassandraImage
	}

	if cc.Spec.Cassandra.ImagePullPolicy == "" {
		cc.Spec.Cassandra.ImagePullPolicy = v1.PullIfNotPresent
	}

	if cc.Spec.Cassandra.NumSeeds == 0 {
		cc.Spec.Cassandra.NumSeeds = 2
	}

	if cc.Spec.Cassandra.LogLevel == "" {
		cc.Spec.Cassandra.LogLevel = "info"
	}

	if cc.Spec.Cassandra.TerminationGracePeriodSeconds == nil {
		cc.Spec.Cassandra.TerminationGracePeriodSeconds = proto.Int64(300)
	}

	if cc.Spec.Cassandra.Persistence.DataVolumeClaimSpec.VolumeMode == nil {
		volumeModeFile := v1.PersistentVolumeFilesystem
		cc.Spec.Cassandra.Persistence.DataVolumeClaimSpec.VolumeMode = &volumeModeFile
	}

	cc.Spec.Cassandra.Persistence.DataVolumeClaimSpec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce}

	if cc.Spec.Cassandra.Persistence.CommitLogVolumeClaimSpec.VolumeMode == nil {
		volumeModeFile := v1.PersistentVolumeFilesystem
		cc.Spec.Cassandra.Persistence.CommitLogVolumeClaimSpec.VolumeMode = &volumeModeFile
	}

	cc.Spec.Cassandra.Persistence.CommitLogVolumeClaimSpec.AccessModes = []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce}

	r.defaultMonitoring(cc)
}

func (r *CassandraClusterReconciler) defaultServerTLS(cc *dbv1alpha1.CassandraCluster) {
	if cc.Spec.Encryption.Server.InternodeEncryption == "" {
		cc.Spec.Encryption.Server.InternodeEncryption = dbv1alpha1.InternodeEncryptionNone
	}

	if cc.Spec.Encryption.Server.RequireClientAuth == nil {
		cc.Spec.Encryption.Server.RequireClientAuth = proto.Bool(true)
	}

	r.defaultTLSSecret(&cc.Spec.Encryption.Server.TLSSecret)

	if cc.Spec.Encryption.Server.Protocol == "" {
		cc.Spec.Encryption.Server.Protocol = "TLS"
	}

	if cc.Spec.Encryption.Server.Algorithm == "" {
		cc.Spec.Encryption.Server.Algorithm = "SunX509"
	}

	if cc.Spec.Encryption.Server.StoreType == "" {
		cc.Spec.Encryption.Server.StoreType = "JKS"
	}

	if len(cc.Spec.Encryption.Server.CipherSuites) == 0 {
		cc.Spec.Encryption.Server.CipherSuites = []string{tlsEcdheRsaAes256GcmSha384, tlsEcdheRsaAes128GcmSha256}
	}
}

func (r *CassandraClusterReconciler) defaultClientTLS(cc *dbv1alpha1.CassandraCluster) {
	if cc.Spec.Encryption.Client.RequireClientAuth == nil {
		cc.Spec.Encryption.Client.RequireClientAuth = proto.Bool(true)
	}

	r.defaultTLSSecret(&cc.Spec.Encryption.Client.TLSSecret.TLSSecret)

	if cc.Spec.Encryption.Client.Protocol == "" {
		cc.Spec.Encryption.Client.Protocol = "TLS"
	}

	if cc.Spec.Encryption.Client.Algorithm == "" {
		cc.Spec.Encryption.Client.Algorithm = "SunX509"
	}

	if cc.Spec.Encryption.Client.StoreType == "" {
		cc.Spec.Encryption.Client.StoreType = "JKS"
	}

	if len(cc.Spec.Encryption.Client.CipherSuites) == 0 {
		cc.Spec.Encryption.Client.CipherSuites = []string{tlsEcdheRsaAes256GcmSha384, tlsEcdheRsaAes128GcmSha256}
	}

	if cc.Spec.Encryption.Client.TLSSecret.CAFileKey == "" {
		cc.Spec.Encryption.Client.TLSSecret.CAFileKey = "ca.crt"
	}

	if cc.Spec.Encryption.Client.TLSSecret.TLSCrtFileKey == "" {
		cc.Spec.Encryption.Client.TLSSecret.TLSCrtFileKey = "tls.crt"
	}

	if cc.Spec.Encryption.Client.TLSSecret.TLSFileKey == "" {
		cc.Spec.Encryption.Client.TLSSecret.TLSFileKey = "tls.key"
	}
}

func (r *CassandraClusterReconciler) defaultTLSSecret(tlsSecret *dbv1alpha1.TLSSecret) {
	if tlsSecret.KeystoreFileKey == "" {
		tlsSecret.KeystoreFileKey = "keystore.jks"
	}

	if tlsSecret.KeystorePasswordKey == "" {
		tlsSecret.KeystorePasswordKey = "keystore.password"
	}

	if tlsSecret.TruststoreFileKey == "" {
		tlsSecret.TruststoreFileKey = "truststore.jks"
	}

	if tlsSecret.TruststorePasswordKey == "" {
		tlsSecret.TruststorePasswordKey = "truststore.password"
	}
}

func (r *CassandraClusterReconciler) defaultMonitoring(cc *dbv1alpha1.CassandraCluster) {
	if !cc.Spec.Cassandra.Monitoring.Enabled {
		return
	}
	if cc.Spec.Cassandra.Monitoring.Agent == "" {
		cc.Spec.Cassandra.Monitoring.Agent = "tlp"
	}
	if cc.Spec.Cassandra.Monitoring.ServiceMonitor.Enabled {
		if _, err := time.ParseDuration(cc.Spec.Cassandra.Monitoring.ServiceMonitor.ScrapeInterval); err != nil {
			cc.Spec.Cassandra.Monitoring.ServiceMonitor.ScrapeInterval = "30s"
		}
	}
}

func (r *CassandraClusterReconciler) defaultSysctls(cc *dbv1alpha1.CassandraCluster) {
	defaultSysctls := map[string]string{
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
	}

	if cc.Spec.Cassandra.Sysctls == nil {
		cc.Spec.Cassandra.Sysctls = defaultSysctls
		return
	}

	for key, value := range defaultSysctls {
		if _, exists := cc.Spec.Cassandra.Sysctls[key]; !exists {
			cc.Spec.Cassandra.Sysctls[key] = value
		}
	}
}
