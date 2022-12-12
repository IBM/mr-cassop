package nodectl

import (
	"context"

	"github.com/ibm/cassandra-operator/controllers/nodectl/jolokia"
)

func (n *client) Decommission(ctx context.Context, nodeIP string) error {
	req := jolokia.JMXRequest{
		Type:      jmxRequestTypeExec,
		Mbean:     mbeanCassandraDBStorageService,
		Operation: "decommission",
	}

	_, err := n.jolokia.Post(ctx, req, nodeIP)
	return err
}
