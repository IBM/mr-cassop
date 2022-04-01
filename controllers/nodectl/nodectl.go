package nodectl

import (
	"context"

	"github.com/ibm/cassandra-operator/controllers/nodectl/jolokia"
	"go.uber.org/zap"
)

const (
	jmxRequestTypeExec = "exec"
	jmxRequestTypeRead = "read"

	mbeanCassandraDBStorageService = "org.apache.cassandra.db:type=StorageService"
	mbeanCassandraNetGossiper      = "org.apache.cassandra.net:type=Gossiper"
)

type Nodectl interface {
	Decommission(ctx context.Context, nodeIP string) error
	Assassinate(ctx context.Context, execNodeIP, assassinateNodeIP string) error
	Version(ctx context.Context, nodeIP string) (major, minor, patch int, err error)
	ClusterView(ctx context.Context, nodeIP string) (ClusterView, error)
	OperationMode(ctx context.Context, nodeIP string) (OperationMode, error)
}

func NewClient(jolokiaAddr, jmxUser, jmxPassword string, logr *zap.SugaredLogger) Nodectl {
	return &client{
		jolokia: jolokia.NewClient(jolokiaAddr, jmxUser, jmxPassword, logr),
		log:     logr,
	}
}

type client struct {
	jolokia *jolokia.Client
	log     *zap.SugaredLogger
}
