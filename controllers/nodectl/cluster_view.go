package nodectl

import (
	"encoding/json"

	"github.com/ibm/cassandra-operator/controllers/nodectl/jolokia"
)

type ClusterView struct {
	LiveNodes        []string `json:"LiveNodes"`
	LeavingNodes     []string `json:"LeavingNodes"`
	JoiningNodes     []string `json:"JoiningNodes"`
	UnreachableNodes []string `json:"UnreachableNodes"`
	MovingNodes      []string `json:"MovingNodes"`
}

func (n *client) ClusterView(nodeIP string) (ClusterView, error) {
	req := jolokia.JMXRequest{
		Type:       jmxRequestTypeRead,
		Mbean:      mbeanCassandraDBStorageService,
		Attributes: []string{"LiveNodes", "LeavingNodes", "JoiningNodes", "UnreachableNodes", "MovingNodes"},
	}

	resp, err := n.jolokia.Post(req, nodeIP)
	if err != nil {
		return ClusterView{}, err
	}

	view := ClusterView{}
	err = json.Unmarshal(resp.Value, &view)
	if err != nil {
		return ClusterView{}, err
	}

	return view, nil
}
