package controllers

import (
	"fmt"
	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

func (r *CassandraClusterReconciler) defaultCassandraCluster(cc *dbv1alpha1.CassandraCluster) {
	if cc.Spec.CQLConfigMapLabelKey == "" {
		cc.Spec.CQLConfigMapLabelKey = defaultCQLConfigMapLabelKey
	}

	if cc.Spec.PodManagementPolicy == "" {
		cc.Spec.PodManagementPolicy = appsv1.ParallelPodManagement
	}

	if len(cc.Spec.SystemKeyspaces.DCs) == 0 {
		for _, dc := range cc.Spec.DCs {
			cc.Spec.SystemKeyspaces.DCs = append(cc.Spec.SystemKeyspaces.DCs, dbv1alpha1.SystemKeyspaceDC{Name: dc.Name, RF: 3})
		}
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

	if cc.Spec.JMX.Authentication == "" {
		cc.Spec.JMX.Authentication = jmxAuthenticationInternal
	}

	if cc.Spec.Encryption.Server.InternodeEncryption != InternodeEncryptionNone {
		r.defaultServerTLS(cc)
	}
}

func (r *CassandraClusterReconciler) defaultReaper(cc *dbv1alpha1.CassandraCluster) {
	if cc.Spec.Reaper == nil {
		cc.Spec.Reaper = &dbv1alpha1.Reaper{}
	}

	if cc.Spec.Reaper.Keyspace == "" {
		cc.Spec.Reaper.Keyspace = "reaper_db"
	}

	if cc.Spec.Reaper.Image == "" {
		cc.Spec.Reaper.Image = r.Cfg.DefaultReaperImage
	}

	if cc.Spec.Reaper.ImagePullPolicy == "" {
		cc.Spec.Reaper.ImagePullPolicy = v1.PullIfNotPresent
	}

	if len(cc.Spec.Reaper.DCs) == 0 {
		cc.Spec.Reaper.DCs = cc.Spec.DCs
	}

	if cc.Spec.Reaper.RepairIntensity == "" {
		cc.Spec.Reaper.RepairIntensity = "1.0"
	}

	if cc.Spec.Reaper.DatacenterAvailability == "" {
		cc.Spec.Reaper.DatacenterAvailability = "each"
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

	if cc.Spec.Prober.Jolokia.Image == "" {
		cc.Spec.Prober.Jolokia.Image = r.Cfg.DefaultJolokiaImage
	}

	if cc.Spec.Prober.Jolokia.ImagePullPolicy == "" {
		cc.Spec.Prober.Jolokia.ImagePullPolicy = v1.PullIfNotPresent
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
}

func (r *CassandraClusterReconciler) defaultServerTLS(cc *dbv1alpha1.CassandraCluster) {
	if cc.Spec.Encryption.Server.InternodeEncryption == "" {
		cc.Spec.Encryption.Server.InternodeEncryption = InternodeEncryptionNone
	}

	if cc.Spec.Encryption.Server.TLSSecret.KeystoreFileKey == "" {
		cc.Spec.Encryption.Server.TLSSecret.KeystoreFileKey = "keystore.jks"
	}

	if cc.Spec.Encryption.Server.TLSSecret.KeystorePasswordKey == "" {
		cc.Spec.Encryption.Server.TLSSecret.KeystorePasswordKey = "keystore.password"
	}

	if cc.Spec.Encryption.Server.TLSSecret.TruststoreFileKey == "" {
		cc.Spec.Encryption.Server.TLSSecret.TruststoreFileKey = "truststore.jks"
	}

	if cc.Spec.Encryption.Server.TLSSecret.TruststorePasswordKey == "" {
		cc.Spec.Encryption.Server.TLSSecret.TruststorePasswordKey = "truststore.password"
	}

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
		cc.Spec.Encryption.Server.CipherSuites = []string{"TLS_RSA_WITH_AES_128_CBC_SHA", "TLS_RSA_WITH_AES_256_CBC_SHA"}
	}

	if cc.Spec.Encryption.Server.RequireClientAuth == nil {
		cc.Spec.Encryption.Server.RequireClientAuth = proto.Bool(true)
	}
}
