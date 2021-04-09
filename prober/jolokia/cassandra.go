package jolokia

import (
	"encoding/json"
	"regexp"

	"sigs.k8s.io/yaml"
)

var (
	CassandraStates = JmxObject{"org.apache.cassandra.net:type=FailureDetector", "SimpleStates,AllEndpointStates"}
	// regexEndpointStatesRemovals matches digits before a colon OR anything after a comma:
	// 	RACK:10:rack1 -> `10:`
	// 	STATUS:22:NORMAL,-2918089050085335913 -> `,-2918089050085335913`, `22:`
	regexEndpointStatesRemovals = regexp.MustCompile(`\d+:|,.+`)
	// regexEndpointStatesToYaml matches ips that start with a slash OR colons:
	// 	/10.244.0.22 -> `/10.244.0.22` // match group $1
	// 	RACK:rack1 -> `:`
	regexEndpointStatesToYamlColon = regexp.MustCompile(`(/\S+)|:`)
)

type CassResponse struct {
	// maps node IPs to a status of either "UP" or "DOWN.
	SimpleStates map[string]string
	// maps node IPs to a struct with the extended states defined in EndpointStates.
	AllEndpointStates AllEndpointStates
}

// EndpointStates of useful properties of a node's state
type EndpointStates struct {
	Status, DC, Rack, Internal_IP, RPC_Address string
}

// AllEndpointStates implements UnmarshalJSON to transform the Cassandra MBean to a Go struct.
type AllEndpointStates map[string]EndpointStates

// UnmarshalJSON interprets the MBean AllEndpointStates and unmarshalls it into a map[string]string.
// Parameter raw is a []byte that expects the following format for a map of known Endpoints:
// 	`"\/10.244.0.5\n  generation:1615484112\n  heartbeat:147677\n  STATUS:17:NORMAL,-1068096267908218392\n`
func (e *AllEndpointStates) UnmarshalJSON(raw []byte) error {
	// AllEndpointStates can be converted into valid yaml with a few regex operations
	var data string
	// unmarshalling raw to a string decodes backlash escapes (i.e. `\/`, `\n`)
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}

	data = regexEndpointStatesRemovals.ReplaceAllString(data, "")
	data = regexEndpointStatesToYamlColon.ReplaceAllString(data, "$1: ")

	var states map[string]EndpointStates
	if err := yaml.Unmarshal([]byte(data), &states); err != nil {
		return err
	}

	*e = states
	return nil
}
