package nodectl

import (
	"github.com/ibm/cassandra-operator/controllers/nodectl/jolokia"
	"go.uber.org/zap"
)

const (
	jmxRequestTypeExec = "exec"
	jmxRequestTypeRead = "read"

	mbeanCassandraDBStorageService = "org.apache.cassandra.db:type=StorageService"
)

type Nodectl interface {
	Decommission(nodeIP string) error
	Version(nodeIP string) (major, minor, patch int, err error)
	ClusterView(nodeIP string) (ClusterView, error)
	OperationMode(nodeIP string) (OperationMode, error)
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
