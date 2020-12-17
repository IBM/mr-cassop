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
	"github.com/onsi/gomega"
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

	mocks := mockedClients{prober: proberClientMock, nodetool: nodetoolClientMock, cql: cqlClientMock, reaper: reaperClientMock}
	return reconciler, mCtrl, mocks
}

type mockedClients struct {
	prober   *mocks.MockProberClient
	nodetool *mocks.MockNodetoolClient
	cql      *mocks.MockCqlClient
	reaper   *mocks.MockReaperClient
}

func TestDefaultingFunction(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	reconciler := &CassandraClusterReconciler{
		Cfg: config.Config{
			DefaultKwatcherImage:  "kwatcher/image",
			DefaultProberImage:    "prober/image",
			DefaultJolokiaImage:   "jolokia/image",
			DefaultCassandraImage: "cassandra/image",
			DefaultReaperImage:    "reaper/image",
		},
	}

	cc := &v1alpha1.CassandraCluster{}
	reconciler.defaultCassandraCluster(cc)
	g.Expect(cc.Spec.CQLConfigMapLabelKey).To(gomega.Equal(defaultCQLConfigMapLabelKey))
	g.Expect(cc.Spec.Cassandra).ToNot(gomega.BeNil())
	g.Expect(cc.Spec.Cassandra.Image).To(gomega.Equal("cassandra/image"))
	g.Expect(cc.Spec.Cassandra.ImagePullPolicy).To(gomega.Equal(v1.PullIfNotPresent))
	g.Expect(cc.Spec.Cassandra.NumSeeds).To(gomega.Equal(int32(2)))
	g.Expect(cc.Spec.Prober.Image).To(gomega.Equal("prober/image"))
	g.Expect(cc.Spec.Prober.ImagePullPolicy).To(gomega.Equal(v1.PullIfNotPresent))
	g.Expect(cc.Spec.Prober.Jolokia.Image).To(gomega.Equal("jolokia/image"))
	g.Expect(cc.Spec.Prober.Jolokia.ImagePullPolicy).To(gomega.Equal(v1.PullIfNotPresent))
	g.Expect(cc.Spec.Kwatcher.Image).To(gomega.Equal("kwatcher/image"))
	g.Expect(cc.Spec.Kwatcher.ImagePullPolicy).To(gomega.Equal(v1.PullIfNotPresent))
	g.Expect(cc.Spec.Reaper).ToNot(gomega.BeNil())
	g.Expect(cc.Spec.Reaper.Keyspace).To(gomega.Equal("reaper_db"))
	g.Expect(cc.Spec.Reaper.Image).To(gomega.Equal("reaper/image"))
	g.Expect(cc.Spec.Reaper.ImagePullPolicy).To(gomega.Equal(v1.PullIfNotPresent))
	g.Expect(cc.Spec.Reaper.DatacenterAvailability).To(gomega.Equal("each"))
	g.Expect(cc.Spec.Reaper.RepairIntensity).To(gomega.Equal("1.0"))
	g.Expect(cc.Spec.Reaper.Tolerations).To(gomega.BeNil())
	g.Expect(cc.Spec.Reaper.NodeSelector).To(gomega.BeNil())

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
	g.Expect(cc.Spec.Reaper.DCs).To(gomega.Equal(cc.Spec.DCs))
	g.Expect(cc.Spec.Reaper.ScheduleRepairs.Repairs[0].Keyspace).To(gomega.Equal("system_auth"))
	g.Expect(cc.Spec.Reaper.ScheduleRepairs.Repairs[0].RepairParallelism).To(gomega.Equal("datacenter_aware"))
	g.Expect(cc.Spec.Reaper.ScheduleRepairs.Repairs[0].ScheduleDaysBetween).To(gomega.Equal(int32(7)))
	g.Expect(cc.Spec.Reaper.ScheduleRepairs.Repairs[0].Datacenters).To(gomega.Equal([]string{"dc1"}))
	g.Expect(cc.Spec.Reaper.ScheduleRepairs.Repairs[0].RepairThreadCount).To(gomega.Equal(int32(2)))
}
