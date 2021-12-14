package prober

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/ibm/cassandra-operator/prober/jolokia"

	"github.com/onsi/gomega"
	"go.uber.org/zap"
)

type jolokiaMock struct {
	nodeStates         map[string]jolokia.CassandraResponse
	username, password string
}

func (j *jolokiaMock) SetAuth(username, password string) {
	j.username = username
	j.password = password
}

func (j *jolokiaMock) CassandraNodeState(ip string) (jolokia.CassandraResponse, error) {
	resp, nodeFound := j.nodeStates[ip]
	if !nodeFound {
		return jolokia.CassandraResponse{}, fmt.Errorf("node %s not found", ip)
	}
	return resp, nil
}

func endpointState(ip, state string) jolokia.EndpointState {
	return jolokia.EndpointState{
		Status:      state,
		DC:          "dc1",
		Rack:        "rack1",
		Internal_IP: ip,
		RPC_Address: ip,
	}
}

func cassandraResponse(endpoints map[string]string) jolokia.CassandraNodeState {
	cassResp := jolokia.CassandraNodeState{
		SimpleStates:      map[string]string{},
		AllEndpointStates: map[string]jolokia.EndpointState{},
	}

	for ip, cassandraNodeState := range endpoints {
		cassResp.SimpleStates["/"+ip] = cassandraNodeState
		cassResp.AllEndpointStates["/"+ip] = endpointState(ip, cassandraNodeState)
	}
	return cassResp
}

func TestUpdateNodeStates(t *testing.T) {
	asserts := gomega.NewWithT(t)
	successJMXResponse := jolokia.Response{
		Status: http.StatusOK,
	}
	testCases := []struct {
		name          string
		initialState  state
		expectedState state
		nodeStates    map[string]jolokia.CassandraResponse
	}{
		{
			name: "new ready nodes were registered",
			initialState: state{
				nodes: map[string]nodeState{
					"/10.12.13.43": {},
					"/10.12.13.44": {},
					"/10.12.13.45": {},
				},
				dcs: []dc{
					{
						Name:     "dc1",
						Replicas: 3,
					},
				},
			},
			expectedState: state{
				nodes: map[string]nodeState{
					"/10.12.13.43": {
						SimpleStates:  map[string]string{"/10.12.13.45": "UP", "/10.12.13.43": "UP", "/10.12.13.44": "UP", "/10.12.13.46": "UP"},
						EndpointState: endpointState("10.12.13.43", "UP"),
					},
					"/10.12.13.44": {
						SimpleStates:  map[string]string{"/10.12.13.45": "UP", "/10.12.13.43": "UP", "/10.12.13.44": "UP", "/10.12.13.46": "UP"},
						EndpointState: endpointState("10.12.13.44", "UP"),
					},
					"/10.12.13.45": {
						SimpleStates:  map[string]string{"/10.12.13.45": "UP", "/10.12.13.43": "UP", "/10.12.13.44": "UP", "/10.12.13.46": "UP"},
						EndpointState: endpointState("10.12.13.45", "UP"),
					},
					"/10.12.13.46": {
						EndpointState: endpointState("10.12.13.46", "UP"),
					},
				},
				dcs: []dc{
					{
						Name:     "dc1",
						Replicas: 3,
					},
				},
			},
			nodeStates: map[string]jolokia.CassandraResponse{
				"/10.12.13.43": {
					Response: successJMXResponse,
					Value: cassandraResponse(map[string]string{
						"10.12.13.43": "UP",
						"10.12.13.44": "UP",
						"10.12.13.45": "UP",
						"10.12.13.46": "UP",
					}),
				},
				"/10.12.13.44": {
					Response: successJMXResponse,
					Value: cassandraResponse(map[string]string{
						"10.12.13.43": "UP",
						"10.12.13.44": "UP",
						"10.12.13.45": "UP",
						"10.12.13.46": "UP",
					}),
				},
				"/10.12.13.45": {
					Response: successJMXResponse,
					Value: cassandraResponse(map[string]string{
						"10.12.13.43": "UP",
						"10.12.13.44": "UP",
						"10.12.13.45": "UP",
						"10.12.13.46": "UP",
					}),
				},
			},
		},
		{
			name: "0 discovered nodes",
			initialState: state{
				nodes: map[string]nodeState{},
				dcs:   []dc{},
			},
			expectedState: state{
				nodes: map[string]nodeState{},
				dcs:   []dc{},
			},
			nodeStates: map[string]jolokia.CassandraResponse{},
		},
		{
			name: "node removed",
			initialState: state{
				nodes: map[string]nodeState{
					"/10.12.13.43": {},
					"/10.12.13.44": {},
					"/10.12.13.45": {},
					"/10.12.13.46": {}, // doesn't exist anymore
				},
				dcs: []dc{
					{
						Name:     "dc1",
						Replicas: 3,
					},
				},
			},
			expectedState: state{
				nodes: map[string]nodeState{
					"/10.12.13.43": {
						SimpleStates:  map[string]string{"/10.12.13.45": "UP", "/10.12.13.43": "UP", "/10.12.13.44": "UP"},
						EndpointState: endpointState("10.12.13.43", "UP"),
					},
					"/10.12.13.44": {
						SimpleStates:  map[string]string{"/10.12.13.45": "UP", "/10.12.13.43": "UP", "/10.12.13.44": "UP"},
						EndpointState: endpointState("10.12.13.44", "UP"),
					},
					"/10.12.13.45": {
						SimpleStates:  map[string]string{"/10.12.13.45": "UP", "/10.12.13.43": "UP", "/10.12.13.44": "UP"},
						EndpointState: endpointState("10.12.13.45", "UP"),
					},
				},
				dcs: []dc{
					{
						Name:     "dc1",
						Replicas: 3,
					},
				},
			},
			nodeStates: map[string]jolokia.CassandraResponse{
				"/10.12.13.43": {
					Response: successJMXResponse,
					Value: cassandraResponse(map[string]string{
						"10.12.13.43": "UP",
						"10.12.13.44": "UP",
						"10.12.13.45": "UP",
					}),
				},
				"/10.12.13.44": {
					Response: successJMXResponse,
					Value: cassandraResponse(map[string]string{
						"10.12.13.43": "UP",
						"10.12.13.44": "UP",
						"10.12.13.45": "UP",
					}),
				},
				"/10.12.13.45": {
					Response: successJMXResponse,
					Value: cassandraResponse(map[string]string{
						"10.12.13.43": "UP",
						"10.12.13.44": "UP",
						"10.12.13.45": "UP",
					}),
				},
			},
		},
		{
			name: "node becomes unready",
			initialState: state{
				nodes: map[string]nodeState{
					"/10.12.13.43": {
						SimpleStates:  map[string]string{"/10.12.13.45": "UP", "/10.12.13.43": "UP", "/10.12.13.44": "UP"},
						EndpointState: endpointState("10.12.13.43", "UP"),
					},
					"/10.12.13.44": {
						SimpleStates:  map[string]string{"/10.12.13.45": "UP", "/10.12.13.43": "UP", "/10.12.13.44": "UP"},
						EndpointState: endpointState("10.12.13.44", "UP"),
					},
					"/10.12.13.45": {
						SimpleStates:  map[string]string{"/10.12.13.45": "UP", "/10.12.13.43": "UP", "/10.12.13.44": "UP"},
						EndpointState: endpointState("10.12.13.45", "UP"),
					},
				},
				dcs: []dc{
					{
						Name:     "dc1",
						Replicas: 3,
					},
				},
			},
			expectedState: state{
				nodes: map[string]nodeState{
					"/10.12.13.43": {
						SimpleStates: map[string]string{"/10.12.13.45": "UP", "/10.12.13.43": "UP", "/10.12.13.44": "DOWN"},
						EndpointState: jolokia.EndpointState{
							Status:      "UP",
							DC:          "dc1",
							Rack:        "rack1",
							Internal_IP: "10.12.13.43",
							RPC_Address: "10.12.13.43",
						},
					},
					"/10.12.13.44": {
						SimpleStates: map[string]string{"/10.12.13.45": "UP", "/10.12.13.43": "UP", "/10.12.13.44": "UP"},
						EndpointState: jolokia.EndpointState{
							Status:      "UP",
							DC:          "dc1",
							Rack:        "rack1",
							Internal_IP: "10.12.13.44",
							RPC_Address: "10.12.13.44",
						},
					},
					"/10.12.13.45": {
						SimpleStates: map[string]string{"/10.12.13.45": "UP", "/10.12.13.43": "UP", "/10.12.13.44": "DOWN"},
						EndpointState: jolokia.EndpointState{
							Status:      "UP",
							DC:          "dc1",
							Rack:        "rack1",
							Internal_IP: "10.12.13.45",
							RPC_Address: "10.12.13.45",
						},
					},
				},
				dcs: []dc{
					{
						Name:     "dc1",
						Replicas: 3,
					},
				},
			},
			nodeStates: map[string]jolokia.CassandraResponse{
				"/10.12.13.43": {
					Response: successJMXResponse,
					Value: cassandraResponse(map[string]string{
						"10.12.13.43": "UP",
						"10.12.13.44": "DOWN",
						"10.12.13.45": "UP",
					}),
				},
				"/10.12.13.44": {
					Response: successJMXResponse,
					Value: cassandraResponse(map[string]string{
						"10.12.13.43": "UP",
						"10.12.13.44": "UP",
						"10.12.13.45": "UP",
					}),
				},
				"/10.12.13.45": {
					Response: successJMXResponse,
					Value: cassandraResponse(map[string]string{
						"10.12.13.43": "UP",
						"10.12.13.44": "DOWN",
						"10.12.13.45": "UP",
					}),
				},
			},
		},
	}

	for _, testCase := range testCases {
		testProber := &Prober{
			auth:  UserAuth{},
			log:   zap.NewNop().Sugar(),
			state: testCase.initialState,
			jolokia: &jolokiaMock{
				nodeStates: testCase.nodeStates,
			},
		}

		testProber.updateNodeStates()
		asserts.Expect(testProber.state).To(gomega.Equal(testCase.expectedState), cmp.Diff(testCase.expectedState, testProber.state, cmp.Options{cmp.AllowUnexported(state{})}))
	}
}
