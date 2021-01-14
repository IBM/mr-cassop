package controllers

import (
	"github.com/gocql/gocql"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/mock/gomock"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/config"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/mocks"
	"github.com/ibm/cassandra-operator/controllers/nodetool"
	"github.com/ibm/cassandra-operator/controllers/prober"
	"github.com/ibm/cassandra-operator/controllers/reaper"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net/url"
	"testing"
)

func createMockedReconciler(t *testing.T) (*CassandraClusterReconciler, *gomock.Controller, mockedClients) {
	mCtrl := gomock.NewController(t)
	proberClientMock := mocks.NewMockProberClient(mCtrl)
	nodetoolClientMock := mocks.NewMockNodetoolClient(mCtrl)
	cqlClientMock := mocks.NewMockCqlClient(mCtrl)
	reaperClientMock := mocks.NewMockReaperClient(mCtrl)
	reconciler := &CassandraClusterReconciler{
		Client:     nil,
		Log:        zap.NewNop().Sugar(),
		Scheme:     nil,
		Cfg:        config.Config{},
		Clientset:  nil,
		RESTConfig: nil,
		ProberClient: func(url *url.URL) prober.ProberClient {
			return proberClientMock
		},
		CqlClient: func(clusterConfig *gocql.ClusterConfig) (cql.CqlClient, error) {
			return cqlClientMock, nil
		},
		NodetoolClient: func(clientset *kubernetes.Clientset, config *rest.Config) nodetool.NodetoolClient {
			return nodetoolClientMock
		},
		ReaperClient: func(url *url.URL) reaper.ReaperClient {
			return reaperClientMock
		},
	}

	m := mockedClients{prober: proberClientMock, nodetool: nodetoolClientMock, cql: cqlClientMock, reaper: reaperClientMock}
	return reconciler, mCtrl, m
}

type mockedClients struct {
	prober   *mocks.MockProberClient
	nodetool *mocks.MockNodetoolClient
	cql      *mocks.MockCqlClient
	reaper   *mocks.MockReaperClient
}

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
}
