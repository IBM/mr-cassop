package jolokia

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var (
	allEndpointsValue = AllEndpointStates{
		"/10.244.0.5": EndpointStates{
			Status:      "NORMAL",
			DC:          "dc1",
			Rack:        "rack1",
			Internal_IP: "10.244.0.5",
			RPC_Address: "10.244.0.5",
		},
		"/10.244.0.6": EndpointStates{
			Status: "NORMAL",
			DC: "dc1",
			Rack: "rack1",
			Internal_IP: "10.244.0.6",
			RPC_Address: "10.244.0.6",
		},
		"/10.244.0.7": EndpointStates{
			Status: "NORMAL",
			DC: "dc2",
			Rack: "rack1",
			Internal_IP: "10.244.0.7",
			RPC_Address: "10.244.0.7",
		},
	}
)

func TestAllEndpointStates_UnmarshaText(t *testing.T) {
	tests := []struct {
		name string
		e    AllEndpointStates
		data string
	}{
		{
			name: "unmarshalls unescaped newlines",
			data: `"/10.244.0.6\n  generation:1614289198\n  heartbeat:828499\n  STATUS:22:NORMAL,-2918089050085335913\n  LOAD:828474:134596.0\n  SCHEMA:144:69ea6896-bc4b-3690-8d18-50ee71f33237\n  DC:8:dc1\n  RACK:10:rack1\n  RELEASE_VERSION:4:3.11.9\n  INTERNAL_IP:6:10.244.0.6\n  RPC_ADDRESS:3:10.244.0.6\n  NET_VERSION:1:11\n  HOST_ID:2:3e0d7191-84af-40cf-9e7f-0ce11c925e7f\n  RPC_READY:33:true\n  TOKENS:21:<hidden>\n/10.244.0.7\n  generation:1614289198\n  heartbeat:828572\n  STATUS:22:NORMAL,-139581499681091162\n  LOAD:828543:140333.0\n  SCHEMA:143:69ea6896-bc4b-3690-8d18-50ee71f33237\n  DC:8:dc2\n  RACK:10:rack1\n  RELEASE_VERSION:4:3.11.9\n  INTERNAL_IP:6:10.244.0.7\n  RPC_ADDRESS:3:10.244.0.7\n  NET_VERSION:1:11\n  HOST_ID:2:070ef8d2-7f54-4fd4-b34d-dfd8c2690588\n  RPC_READY:32:true\n  TOKENS:21:<hidden>\n/10.244.0.5\n  generation:1614289203\n  heartbeat:828488\n  STATUS:33:NORMAL,-1068096267908218392\n  LOAD:828452:165615.0\n  SCHEMA:146:69ea6896-bc4b-3690-8d18-50ee71f33237\n  DC:8:dc1\n  RACK:10:rack1\n  RELEASE_VERSION:4:3.11.9\n  INTERNAL_IP:6:10.244.0.5\n  RPC_ADDRESS:3:10.244.0.5\n  NET_VERSION:1:11\n  HOST_ID:2:d629438b-7158-4558-8675-80dc705ddc8e\n  RPC_READY:43:true\n  TOKENS:32:<hidden>\n"`,
		},
		{
			name: "unmarshalls escaped backlashes and newlines",
			data: `"\/10.244.0.6\n  generation:1614289198\n  heartbeat:828499\n  STATUS:22:NORMAL,-2918089050085335913\n  LOAD:828474:134596.0\n  SCHEMA:144:69ea6896-bc4b-3690-8d18-50ee71f33237\n  DC:8:dc1\n  RACK:10:rack1\n  RELEASE_VERSION:4:3.11.9\n  INTERNAL_IP:6:10.244.0.6\n  RPC_ADDRESS:3:10.244.0.6\n  NET_VERSION:1:11\n  HOST_ID:2:3e0d7191-84af-40cf-9e7f-0ce11c925e7f\n  RPC_READY:33:true\n  TOKENS:21:<hidden>\n\/10.244.0.7\n  generation:1614289198\n  heartbeat:828572\n  STATUS:22:NORMAL,-139581499681091162\n  LOAD:828543:140333.0\n  SCHEMA:143:69ea6896-bc4b-3690-8d18-50ee71f33237\n  DC:8:dc2\n  RACK:10:rack1\n  RELEASE_VERSION:4:3.11.9\n  INTERNAL_IP:6:10.244.0.7\n  RPC_ADDRESS:3:10.244.0.7\n  NET_VERSION:1:11\n  HOST_ID:2:070ef8d2-7f54-4fd4-b34d-dfd8c2690588\n  RPC_READY:32:true\n  TOKENS:21:<hidden>\n/10.244.0.5\n  generation:1614289203\n  heartbeat:828488\n  STATUS:33:NORMAL,-1068096267908218392\n  LOAD:828452:165615.0\n  SCHEMA:146:69ea6896-bc4b-3690-8d18-50ee71f33237\n  DC:8:dc1\n  RACK:10:rack1\n  RELEASE_VERSION:4:3.11.9\n  INTERNAL_IP:6:10.244.0.5\n  RPC_ADDRESS:3:10.244.0.5\n  NET_VERSION:1:11\n  HOST_ID:2:d629438b-7158-4558-8675-80dc705ddc8e\n  RPC_READY:43:true\n  TOKENS:32:<hidden>\n"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tt.data), &tt.e); err != nil {
				t.Error("Unmarshal() error = ", err)
			}
			if !cmp.Equal(tt.e, allEndpointsValue) {
				t.Error("Unmarshalled value is not equal to expected", cmp.Diff(allEndpointsValue, tt.e))
			}
		})
	}
}

func TestCassResponse_UnmarshalText(t *testing.T) {
	tests := []struct {
		name string
		e    CassResponse
		data string
	}{
		{
			name: "unmarshalls prettified SimpleStates and unescaped AllEndpointStates",
			data: `{
						"SimpleStates": {
							"/10.244.0.6": "UP",
							"/10.244.0.7": "UP",
							"/10.244.0.5": "UP"
						},
						"AllEndpointStates": "/10.244.0.6\n  generation:1615430009\n  heartbeat:24904\n  STATUS:17:NORMAL,-1068096267908218392\n  LOAD:24878:237311.0\n  SCHEMA:13:fed73249-15e3-378a-946f-7847dc4ed28d\n  DC:9:dc1\n  RACK:11:rack1\n  RELEASE_VERSION:5:3.11.9\n  INTERNAL_IP:7:10.244.0.6\n  RPC_ADDRESS:4:10.244.0.6\n  NET_VERSION:2:11\n  HOST_ID:3:d629438b-7158-4558-8675-80dc705ddc8e\n  RPC_READY:29:true\n  TOKENS:16:<hidden>\n/10.244.0.7\n  generation:1615429985\n  heartbeat:24927\n  STATUS:18:NORMAL,-2918089050085335913\n  LOAD:24877:265004.0\n  SCHEMA:13:fed73249-15e3-378a-946f-7847dc4ed28d\n  DC:9:dc2\n  RACK:11:rack1\n  RELEASE_VERSION:5:3.11.9\n  INTERNAL_IP:7:10.244.0.7\n  RPC_ADDRESS:4:10.244.0.7\n  NET_VERSION:2:11\n  HOST_ID:3:3e0d7191-84af-40cf-9e7f-0ce11c925e7f\n  RPC_READY:31:true\n  TOKENS:17:<hidden>\n/10.244.0.5\n  generation:1615429985\n  heartbeat:24927\n  STATUS:17:NORMAL,-139581499681091162\n  LOAD:24878:254587.0\n  SCHEMA:13:fed73249-15e3-378a-946f-7847dc4ed28d\n  DC:9:dc1\n  RACK:11:rack1\n  RELEASE_VERSION:5:3.11.9\n  INTERNAL_IP:7:10.244.0.5\n  RPC_ADDRESS:4:10.244.0.5\n  NET_VERSION:2:11\n  HOST_ID:3:070ef8d2-7f54-4fd4-b34d-dfd8c2690588\n  RPC_READY:32:true\n  TOKENS:16:<hidden>\n"
					}`,
		},
		{
			name: "unmarshalls minified and unescaped backlashes and newlines",
			data: `{"SimpleStates":{"\/10.244.0.6":"UP","\/10.244.0.7":"UP","\/10.244.0.5":"UP"},"AllEndpointStates":"\/10.244.0.5\n  generation:1615484112\n  heartbeat:147677\n  STATUS:17:NORMAL,-1068096267908218392\n  LOAD:147670:284363.0\n  SCHEMA:13:fed73249-15e3-378a-946f-7847dc4ed28d\n  DC:9:dc1\n  RACK:11:rack1\n  RELEASE_VERSION:5:3.11.9\n  INTERNAL_IP:7:10.244.0.5\n  RPC_ADDRESS:4:10.244.0.5\n  NET_VERSION:2:11\n  HOST_ID:3:d629438b-7158-4558-8675-80dc705ddc8e\n  RPC_READY:29:true\n  TOKENS:16:<hidden>\n\/10.244.0.6\n  generation:1615484110\n  heartbeat:147680\n  STATUS:17:NORMAL,-2918089050085335913\n  LOAD:147672:274238.0\n  SCHEMA:13:fed73249-15e3-378a-946f-7847dc4ed28d\n  DC:9:dc1\n  RACK:11:rack1\n  RELEASE_VERSION:5:3.11.9\n  INTERNAL_IP:7:10.244.0.6\n  RPC_ADDRESS:4:10.244.0.6\n  NET_VERSION:2:11\n  HOST_ID:3:3e0d7191-84af-40cf-9e7f-0ce11c925e7f\n  RPC_READY:29:true\n  TOKENS:16:<hidden>\n\/10.244.0.7\n  generation:1615484111\n  heartbeat:147680\n  STATUS:17:NORMAL,-139581499681091162\n  LOAD:147672:285552.0\n  SCHEMA:13:fed73249-15e3-378a-946f-7847dc4ed28d\n  DC:9:dc2\n  RACK:11:rack1\n  RELEASE_VERSION:5:3.11.9\n  INTERNAL_IP:7:10.244.0.7\n  RPC_ADDRESS:4:10.244.0.7\n  NET_VERSION:2:11\n  HOST_ID:3:070ef8d2-7f54-4fd4-b34d-dfd8c2690588\n  RPC_READY:29:true\n  TOKENS:16:<hidden>\n"}`,
		},
	}

	cassResponseValue := CassResponse{
		SimpleStates: map[string]string{
			"/10.244.0.5": "UP",
			"/10.244.0.6": "UP",
			"/10.244.0.7": "UP",
		},
		AllEndpointStates: allEndpointsValue,
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tt.data), &tt.e); err != nil {
				t.Error("UnmarshalJSON() error = ", err)
			}
			if !cmp.Equal(tt.e, cassResponseValue) {
				t.Error("Unmarshalled value is not equal to expected", cmp.Diff(cassResponseValue, tt.e))
			}
		})
	}
}
