package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/ibm/cassandra-operator/prober/jolokia"
)

type nodeState struct {
	StatesInDc   []string
	SimpleStates map[string]string
	jolokia.EndpointStates
}

type userAuth struct {
	user, password string
}

var (
	jmxPollPeriodSeconds, _ = strconv.Atoi(os.Getenv("JMX_POLL_PERIOD_SECONDS"))
	jmxPort                 = os.Getenv("JMX_PORT")
	jolokiaPort             = os.Getenv("JOLOKIA_PORT")

	pollingIntervalDuration = time.Duration(jmxPollPeriodSeconds) * time.Second
	jolokiaClient           = jolokia.NewClient("localhost", jolokiaPort, pollingIntervalDuration/2)
	nodeStates              = map[string]nodeState{}
	dcIsPolled              = map[string]bool{}
	fallbackAuth            = userAuth{"cassandra", "cassandra"}
	isFallbackAuth          = true
	// TODO: set userAuth using file watcher
	auth userAuth
)

func processReadinessProbe(podIp string, broadcastIp string) (bool, []string) {
	ip := "/"
	if broadcastIp != "" {
		ip += broadcastIp
	} else {
		ip += podIp
	}
	if _, ok := nodeStates[ip]; !ok {
		log.Info("new ip from readiness probe", "ip", ip)
		nodeStates[ip] = nodeState{}
	}
	return isNodeReady(ip), nodeStates[ip].StatesInDc
}

func isNodeReady(ip string) bool {
	nodesDownCount := 0
	for _, node := range nodeStates[ip].StatesInDc {
		if node != "UP" {
			nodesDownCount++
		}
	}
	return nodesDownCount <= len(nodeStates[ip].StatesInDc)-1
}

func updateNodeStates() {
	if len(nodeStates) > 0 {
		err := updateNodesRequest()
		if err != nil {
			log.Error(err, "Failed updateNodesRequest")
			log.Info("Retrying with fallback auth", "isFallbackAuth", isFallbackAuth)
			isFallbackAuth = !isFallbackAuth
		}
	} else {
		log.Info("0 discovered nodes...")
	}
}

func getUserAuth() userAuth {
	if isFallbackAuth {
		return fallbackAuth
	}
	return auth
}

func updateNodesRequest() error {
	var polledIps []string
	for ip, _ := range nodeStates {
		polledIps = append(polledIps, ip)
	}

	// Construct Cassandra request for each polled ip
	auth := getUserAuth()
	body := jolokia.ProxyRequests(jolokia.CassandraStates, auth.user, auth.password, jmxPort, polledIps...)

	jmxResponses, err := jolokiaClient.Post(body)
	if err != nil {
		return err
	}

	var responses []struct {
		jolokia.Response
		Value jolokia.CassResponse
	}
	if err := json.Unmarshal(jmxResponses, &responses); err != nil {
		return err
	}

	newNodeStates := make(map[string]nodeState)
	for i, polledIp := range polledIps {
		node := nodeState{}
		
		if responses[i].Status == http.StatusOK {  // responses[i].Value is not nil
			res := responses[i].Value
			node.SimpleStates = res.SimpleStates
			node.EndpointStates = res.AllEndpointStates[polledIp]
			dcIsPolled[node.DC] = true
			// lookup new nodes from node's peers (`.AllEndpointsStates`)
			for ip, endpointState := range res.AllEndpointStates {
				if _, ok := newNodeStates[ip]; !ok && dcIsPolled[endpointState.DC] {
					if _, ok := nodeStates[ip]; !ok {
						log.Info("new node found", "IP", ip, "DC", endpointState.DC)
					}
					peerNode := nodeState{}
					peerNode.EndpointStates = endpointState
					newNodeStates[ip] = peerNode
				}
			}
		} else {
			node.SimpleStates = make(map[string]string)
			node.SimpleStates[polledIp] = strconv.Itoa(responses[i].Status)
		}
		newNodeStates[polledIp] = node
	}

	for ip, state := range newNodeStates {
		if state.DC == "" {
			log.Info("removing unreferenced node", "IP", ip)
			delete(newNodeStates, ip)
			continue
		}
		for _, otherState := range newNodeStates {
			if state.DC == otherState.DC {
				state.StatesInDc = append(state.StatesInDc, otherState.SimpleStates[ip])
			}
		}
		newNodeStates[ip] = state
	}

	if !reflect.DeepEqual(newNodeStates, nodeStates) {
		log.Info("Node states updated")
		fmt.Println("Diff: \n", cmp.Diff(nodeStates, newNodeStates))
		nodeStates = newNodeStates
	}

	return nil
}

func pollNodeStates() {
	for range time.Tick(pollingIntervalDuration) {
		go updateNodeStates()
	}
}
