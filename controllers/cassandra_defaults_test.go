package controllers

import (
	"github.com/gogo/protobuf/proto"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/config"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"testing"
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
	g.Expect(cc.Spec.Reaper.Keyspace).To(Equal("reaper_db"))
	g.Expect(cc.Spec.Reaper.Image).To(Equal("reaper/image"))
	g.Expect(cc.Spec.Reaper.ImagePullPolicy).To(Equal(v1.PullIfNotPresent))
	g.Expect(cc.Spec.Reaper.DatacenterAvailability).To(Equal("each"))
	g.Expect(cc.Spec.Reaper.RepairIntensity).To(Equal("1.0"))
	g.Expect(cc.Spec.Reaper.Tolerations).To(BeNil())
	g.Expect(cc.Spec.Reaper.NodeSelector).To(BeNil())
	g.Expect(cc.Spec.Maintenance).To(BeNil())
	g.Expect(cc.Status.MaintenanceState).To(BeNil())

	cc = &v1alpha1.CassandraCluster{
		Spec: v1alpha1.CassandraClusterSpec{
			DCs: []v1alpha1.DC{
				{
					Name:     "dc1",
					Replicas: proto.Int32(3),
				},
			},
			Reaper: &v1alpha1.Reaper{
				ScheduleRepairs: v1alpha1.ScheduleRepairs{
					Repairs: []v1alpha1.Repair{
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
		},
	}
	reconciler.defaultCassandraCluster(cc)
	g.Expect(cc.Spec.Reaper.DCs).To(Equal(cc.Spec.DCs))
	g.Expect(cc.Spec.SystemKeyspaces.DCs).To(Equal([]v1alpha1.SystemKeyspaceDC{{Name: "dc1", RF: 3}}))
	g.Expect(cc.Spec.Reaper.ScheduleRepairs.Repairs[0].Keyspace).To(Equal("system_auth"))
	g.Expect(cc.Spec.Reaper.ScheduleRepairs.Repairs[0].RepairParallelism).To(Equal("datacenter_aware"))
	g.Expect(cc.Spec.Reaper.ScheduleRepairs.Repairs[0].ScheduleDaysBetween).To(Equal(int32(7)))
	g.Expect(cc.Spec.Reaper.ScheduleRepairs.Repairs[0].Datacenters).To(Equal([]string{"dc1"}))
	g.Expect(cc.Spec.Reaper.ScheduleRepairs.Repairs[0].RepairThreadCount).To(Equal(int32(2)))
	g.Expect(cc.Spec.Maintenance[0].DC).To(Equal("dc1"))
	g.Expect(cc.Spec.Maintenance[0].Pods).ToNot(BeEmpty())
}
