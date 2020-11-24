package controllers

import (
	"github.com/golang/mock/gomock"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/config"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/nodetool"
	"github.com/ibm/cassandra-operator/controllers/prober"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"testing"
)

func createMockedReconciler(t *testing.T) (*CassandraClusterReconciler, *gomock.Controller, mockedClients) {
	mCtrl := gomock.NewController(t)
	proberClientMock := prober.NewMockClient(mCtrl)
	nodetoolClientMock := nodetool.NewMockClient(mCtrl)
	cqlClientMock := cql.NewMockClient(mCtrl)
	reconciler := &CassandraClusterReconciler{
		Client:     nil,
		Log:        zap.NewNop().Sugar(),
		Scheme:     nil,
		Cfg:        config.Config{},
		Clientset:  nil,
		RESTConfig: nil,
		ProberClient: func(host string) prober.Client {
			return proberClientMock
		},
		CqlClient: func(cluster *v1alpha1.CassandraCluster) (cql.Client, error) {
			return cqlClientMock, nil
		},
		NodetoolClient: func(clientset *kubernetes.Clientset, config *rest.Config) nodetool.Client {
			return nodetoolClientMock
		},
	}

	mocks := mockedClients{prober: proberClientMock, nodetool: nodetoolClientMock, cql: cqlClientMock}
	return reconciler, mCtrl, mocks
}

type mockedClients struct {
	prober   *prober.MockClient
	nodetool *nodetool.MockClient
	cql      *cql.MockClient
}
