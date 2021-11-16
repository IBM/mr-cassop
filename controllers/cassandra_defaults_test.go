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
	g.Expect(cc.Spec.Cassandra).ToNot(BeNil())
	g.Expect(cc.Spec.Cassandra.Image).To(Equal("cassandra/image"))
	g.Expect(cc.Spec.Cassandra.ImagePullPolicy).To(Equal(v1.PullIfNotPresent))
	g.Expect(cc.Spec.Cassandra.NumSeeds).To(Equal(int32(2)))
	g.Expect(cc.Spec.Prober.Image).To(Equal("prober/image"))
	g.Expect(cc.Spec.Prober.ImagePullPolicy).To(Equal(v1.PullIfNotPresent))
	g.Expect(cc.Spec.Prober.Jolokia.Image).To(Equal("jolokia/image"))
	g.Expect(cc.Spec.Prober.Jolokia.ImagePullPolicy).To(Equal(v1.PullIfNotPresent))
	g.Expect(cc.Spec.Reaper).ToNot(BeNil())
	g.Expect(cc.Spec.Reaper.Keyspace).To(Equal("reaper"))
	g.Expect(cc.Spec.Reaper.Image).To(Equal("reaper/image"))
	g.Expect(cc.Spec.Reaper.ImagePullPolicy).To(Equal(v1.PullIfNotPresent))
	g.Expect(cc.Spec.Reaper.DatacenterAvailability).To(Equal("each"))
	g.Expect(cc.Spec.Reaper.RepairIntensity).To(Equal("1.0"))
	g.Expect(cc.Spec.Reaper.Tolerations).To(BeNil())
	g.Expect(cc.Spec.Reaper.NodeSelector).To(BeNil())
	g.Expect(cc.Spec.Maintenance).To(BeNil())
	g.Expect(cc.Status.MaintenanceState).To(BeNil())
	g.Expect(cc.Spec.Encryption.Server.InternodeEncryption).To(Equal(internodeEncryptionNone))
	g.Expect(cc.Spec.Encryption.Server.TLSSecret.Name).To(Equal(""))
	g.Expect(cc.Spec.Encryption.Server.TLSSecret.KeystoreFileKey).To(Equal("keystore.jks"))
	g.Expect(cc.Spec.Encryption.Server.TLSSecret.KeystorePasswordKey).To(Equal("keystore.password"))
	g.Expect(cc.Spec.Encryption.Server.TLSSecret.TruststoreFileKey).To(Equal("truststore.jks"))
	g.Expect(cc.Spec.Encryption.Server.TLSSecret.TruststorePasswordKey).To(BeEquivalentTo("truststore.password"))
	g.Expect(cc.Spec.Encryption.Server.Protocol).To(BeEquivalentTo("TLS"))
	g.Expect(cc.Spec.Encryption.Server.Algorithm).To(BeEquivalentTo("SunX509"))
	g.Expect(cc.Spec.Encryption.Server.StoreType).To(BeEquivalentTo("JKS"))
	g.Expect(cc.Spec.Encryption.Server.CipherSuites).To(BeEquivalentTo([]string{"TLS_RSA_WITH_AES_128_CBC_SHA", "TLS_RSA_WITH_AES_256_CBC_SHA"}))
	g.Expect(cc.Spec.Encryption.Server.RequireClientAuth).To(BeEquivalentTo(proto.Bool(true)))
	g.Expect(cc.Spec.Encryption.Server.RequireEndpointVerification).To(BeFalse())

	cc = &v1alpha1.CassandraCluster{
		Spec: v1alpha1.CassandraClusterSpec{
			DCs: []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(3),
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
			},
			Maintenance: []v1alpha1.Maintenance{
				{
					DC: "dc1",
				},
			},
			Encryption: v1alpha1.Encryption{
				Server: v1alpha1.Server{
					InternodeEncryption: "dc",
					TLSSecret: v1alpha1.TLSSecret{
						Name:                  "server-tls-secret",
						KeystoreFileKey:       "test.jks",
						KeystorePasswordKey:   "test.txt",
						TruststoreFileKey:     "test.jks",
						TruststorePasswordKey: "test.txt",
					},
					Protocol:                    "TLS",
					Algorithm:                   "SunX509",
					StoreType:                   "PKCS12",
					CipherSuites:                []string{"TLS_RSA_WITH_AES_256_CBC_SHA"},
					RequireClientAuth:           proto.Bool(false),
					RequireEndpointVerification: true,
				},
			},
		},
	}
	reconciler.defaultCassandraCluster(cc)
	g.Expect(cc.Spec.DCs[0].Tolerations).To(BeNil())
	g.Expect(cc.Spec.DCs[0].Affinity).To(BeNil())
	g.Expect(cc.Spec.SystemKeyspaces.DCs).To(BeNil())
	g.Expect(cc.Spec.Reaper.RepairSchedules.Repairs[0].Keyspace).To(Equal("system_auth"))
	g.Expect(cc.Spec.Reaper.RepairSchedules.Repairs[0].RepairParallelism).To(Equal("DATACENTER_AWARE"))
	g.Expect(cc.Spec.Reaper.RepairSchedules.Repairs[0].ScheduleDaysBetween).To(Equal(int32(7)))
	g.Expect(cc.Spec.Reaper.RepairSchedules.Repairs[0].Datacenters).To(Equal([]string{"dc1"}))
	g.Expect(cc.Spec.Reaper.RepairSchedules.Repairs[0].RepairThreadCount).To(Equal(int32(2)))
	g.Expect(cc.Spec.Maintenance[0].DC).To(Equal("dc1"))
	g.Expect(cc.Spec.Maintenance[0].Pods).ToNot(BeEmpty())
	g.Expect(cc.Spec.Encryption.Server.InternodeEncryption).To(Equal("dc"))
	g.Expect(cc.Spec.Encryption.Server.TLSSecret.Name).To(Equal("server-tls-secret"))
	g.Expect(cc.Spec.Encryption.Server.TLSSecret.KeystoreFileKey).To(Equal("test.jks"))
	g.Expect(cc.Spec.Encryption.Server.TLSSecret.KeystorePasswordKey).To(Equal("test.txt"))
	g.Expect(cc.Spec.Encryption.Server.TLSSecret.TruststoreFileKey).To(Equal("test.jks"))
	g.Expect(cc.Spec.Encryption.Server.TLSSecret.TruststorePasswordKey).To(BeEquivalentTo("test.txt"))
	g.Expect(cc.Spec.Encryption.Server.Protocol).To(BeEquivalentTo("TLS"))
	g.Expect(cc.Spec.Encryption.Server.Algorithm).To(BeEquivalentTo("SunX509"))
	g.Expect(cc.Spec.Encryption.Server.StoreType).To(BeEquivalentTo("PKCS12"))
	g.Expect(cc.Spec.Encryption.Server.CipherSuites).To(BeEquivalentTo([]string{"TLS_RSA_WITH_AES_256_CBC_SHA"}))
	g.Expect(cc.Spec.Encryption.Server.RequireClientAuth).To(BeEquivalentTo(proto.Bool(false)))
	g.Expect(cc.Spec.Encryption.Server.RequireEndpointVerification).To(BeTrue())
}
