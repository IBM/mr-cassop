package nodectl

import "github.com/ibm/cassandra-operator/controllers/nodectl/jolokia"

func (n *client) Decommission(nodeIP string) error {
	req := jolokia.JMXRequest{
		Type:      jmxRequestTypeExec,
		Mbean:     mbeanCassandraDBStorageService,
		Operation: "decommission",
	}

	_, err := n.jolokia.Post(req, nodeIP)
	return err
}
