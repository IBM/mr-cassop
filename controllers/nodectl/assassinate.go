package nodectl

import (
	"context"

	"github.com/pkg/errors"

	"github.com/ibm/cassandra-operator/controllers/nodectl/jolokia"
)

func (n *client) Assassinate(ctx context.Context, execNodeIP, assassinateNodeIP string) error {
	req := jolokia.JMXRequest{
		Type:      jmxRequestTypeExec,
		Mbean:     mbeanCassandraNetGossiper,
		Operation: "assassinateEndpoint",
		Arguments: []string{assassinateNodeIP},
	}

	resp, err := n.jolokia.Post(ctx, req, execNodeIP)
	if err != nil {
		return err
	}

	if resp.Status != 200 {
		return errors.Errorf("unexpected status code: %d. Error: %s", resp.Status, resp.Error)
	}

	return nil
}
