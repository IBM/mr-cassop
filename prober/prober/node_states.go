package prober

import (
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/ibm/cassandra-operator/prober/jolokia"
)

type nodeState struct {
	SimpleStates map[string]string
	jolokia.EndpointState
}

func (p *Prober) processReadinessProbe(podIp string, broadcastIp string) (bool, map[string]string) {
	ip := "/"
	if broadcastIp != "" {
		ip += broadcastIp
	} else {
		ip += podIp
	}
	if _, ok := p.state.nodes[ip]; !ok {
		p.log.Infow("new ip from readiness probe", "ip", ip)
		p.state.nodes[ip] = nodeState{}
	}

	return p.isNodeReady(ip)
}

// isNodeReady checks if all nodes (including the one being checked) see the node as ready
func (p *Prober) isNodeReady(ip string) (bool, map[string]string) {
	var peersUnreadyView []string // nodes that see the questioned node as not ready
	var ignoredPeerNodes []string // nodes that are not ready, so we don't take their view into account
	var nodeClusterView = make(map[string]string)
	for peerNodeIP, node := range p.state.nodes {
		simpleState, exists := node.SimpleStates[ip]
		if !exists {
			simpleState = "?"
		}
		nodeClusterView[peerNodeIP] = simpleState

		//ignore node's view if its status is not `NORMAL` and the node doesn't see itself as `UP`
		if strings.ToLower(node.Status) != "normal" || strings.ToLower(node.SimpleStates[peerNodeIP]) != "up" {
			ignoredPeerNodes = append(ignoredPeerNodes, peerNodeIP)
			continue
		}

		if strings.ToLower(node.SimpleStates[ip]) != "up" {
			peersUnreadyView = append(peersUnreadyView, peerNodeIP)
		}
	}

	if len(ignoredPeerNodes) > 0 {
		p.log.Debugf("ignoring the following node(s) view as they are not ready: %v", ignoredPeerNodes)
	}

	if len(peersUnreadyView) > 0 {
		p.log.Debugf("node %s not seen as ready by %v", ip, peersUnreadyView)
		return false, nodeClusterView
	}

	p.log.Debugf("all healthy nodes see node %s as ready", ip)
	return true, nodeClusterView
}

func (p *Prober) updateNodeStates() {
	if len(p.state.nodes) > 0 {
		p.updateNodesRequest()
	} else {
		p.log.Info("0 discovered nodes...")
	}
}

func (p *Prober) updateNodesRequest() {
	responses := p.allNodesStates()

	newNodeStates := make(map[string]nodeState)
	for polledIp, nodeStateResponse := range responses {
		newNodeState := nodeState{}

		if nodeStateResponse.Status == http.StatusOK { // responses[i].Value is not nil
			cassandraNodeState := nodeStateResponse.Value
			newNodeState.SimpleStates = cassandraNodeState.SimpleStates
			newNodeState.EndpointState = cassandraNodeState.AllEndpointStates[polledIp]
			// lookup new nodes from node's peers (`.AllEndpointsStates`)
			for ip, endpointState := range cassandraNodeState.AllEndpointStates {
				// if the peer node is not in the list of discovered DCs and it belongs to the DC owned by prober
				if _, polledNode := newNodeStates[ip]; !polledNode && p.ownedDC(endpointState.DC) {
					if _, knownNode := p.state.nodes[ip]; !knownNode {
						p.log.Infow("new node found", "ip", ip, "dc", endpointState.DC)
					}
					peerNode := nodeState{}
					peerNode.EndpointState = endpointState
					newNodeStates[ip] = peerNode
				}
			}
		} else {
			newNodeState.SimpleStates = make(map[string]string)
			newNodeState.SimpleStates[polledIp] = strconv.Itoa(nodeStateResponse.Status)
		}
		newNodeStates[polledIp] = newNodeState
	}

	for ip, newNodeState := range newNodeStates {
		if newNodeState.DC == "" {
			p.log.Infow("removing unreferenced node", "ip", ip)
			delete(newNodeStates, ip)
			continue
		}
		newNodeStates[ip] = newNodeState
	}

	if !reflect.DeepEqual(newNodeStates, p.state.nodes) {
		p.log.Info("Node states updated")
		p.log.Debug(cmp.Diff(p.state.nodes, newNodeStates))
		p.state.nodes = newNodeStates
	}
}

// allNodesStates returns JMX response for each discovered node, including failed requests
func (p *Prober) allNodesStates() map[string]jolokia.CassandraResponse {
	responses := make(map[string]jolokia.CassandraResponse)
	for nodeIP := range p.state.nodes {
		response, err := p.jolokia.CassandraNodeState(nodeIP)
		if err != nil {
			p.log.Errorf("jolokia request for IP %q failed: %s", nodeIP, err.Error())
			response = jolokia.CassandraResponse{
				Response: jolokia.Response{
					Status: http.StatusInternalServerError,
					Error:  err.Error(),
				},
			}
			responses[nodeIP] = response
			continue
		}

		responses[nodeIP] = response
	}

	return responses
}

func (p *Prober) ownedDC(dc string) bool {
	for _, ownedDC := range p.state.dcs {
		if ownedDC.Name == dc {
			return true
		}
	}

	return false
}

func (p *Prober) pollNodeStates() {
	for range time.Tick(p.cfg.JmxPollingInterval) {
		go p.updateNodeStates()
	}
}
