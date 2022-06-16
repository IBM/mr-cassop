package prober

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ibm/cassandra-operator/prober/config"
	"k8s.io/client-go/kubernetes"

	"github.com/julienschmidt/httprouter"
	"github.com/onsi/gomega"
	"go.uber.org/zap"
)

func TestHealthCheck(t *testing.T) {
	asserts := gomega.NewWithT(t)
	testCases := []struct {
		name           string
		state          state
		remoteAddr     string
		broadcastAddr  string
		expectedBody   []byte
		expectedStatus int
		expectedState  state
	}{
		{
			name:          "healthcheck for a healthy node",
			remoteAddr:    "172.16.16.4",
			broadcastAddr: "10.134.3.4",
			state: state{
				nodes: map[string]nodeState{
					"/10.134.3.4": {
						SimpleStates:  map[string]string{"/10.134.3.4": "UP"},
						EndpointState: endpointState("/10.134.3.4", "NORMAL"),
					},
				},
				podIPs: map[string]string{
					"/10.134.3.4": "/172.16.16.4",
				},
			},
			expectedState: state{
				nodes: map[string]nodeState{
					"/10.134.3.4": {
						SimpleStates:  map[string]string{"/10.134.3.4": "UP"},
						EndpointState: endpointState("/10.134.3.4", "NORMAL"),
					},
				},
				podIPs: map[string]string{
					"/10.134.3.4": "/172.16.16.4",
				},
			},
			expectedStatus: http.StatusOK,
			expectedBody:   []byte(`{"/10.134.3.4":"UP"}`),
		},
		{
			name:          "new node",
			remoteAddr:    "172.16.16.5",
			broadcastAddr: "10.134.3.5",
			state: state{
				nodes: map[string]nodeState{
					"/10.134.3.4": {
						SimpleStates:  map[string]string{"/10.134.3.4": "UP"},
						EndpointState: endpointState("/10.134.3.4", "NORMAL"),
					},
				},
				podIPs: map[string]string{},
			},
			expectedState: state{
				nodes: map[string]nodeState{
					"/10.134.3.4": {
						SimpleStates:  map[string]string{"/10.134.3.4": "UP"},
						EndpointState: endpointState("/10.134.3.4", "NORMAL"),
					},
					"/10.134.3.5": {},
				},
				podIPs: map[string]string{
					"/10.134.3.5": "/192.0.2.1", // 192.x is default RemoteAddr from net/http/httptest/httptest_test.go
				},
			},
			expectedStatus: http.StatusNotFound,
			expectedBody:   []byte(`{"/10.134.3.4":"?","/10.134.3.5":"?"}`),
		},
		{
			name:          "node not ready",
			remoteAddr:    "172.16.16.5",
			broadcastAddr: "10.134.3.5",
			state: state{
				nodes: map[string]nodeState{
					"/10.134.3.4": {
						SimpleStates: map[string]string{
							"/10.134.3.4": "UP",
							"/10.134.3.5": "DOWN", // seen as down from this node
						},
						EndpointState: endpointState("/10.134.3.4", "NORMAL"),
					},
					"/10.134.3.5": {
						SimpleStates: map[string]string{
							"/10.134.3.4": "UP",
							"/10.134.3.5": "UP",
						},
						EndpointState: endpointState("/10.134.3.5", "NORMAL"),
					},
				},
				podIPs: map[string]string{
					"/10.134.3.5": "/172.16.16.5",
				},
			},
			expectedState: state{
				nodes: map[string]nodeState{
					"/10.134.3.4": {
						SimpleStates: map[string]string{
							"/10.134.3.4": "UP",
							"/10.134.3.5": "DOWN",
						},
						EndpointState: endpointState("/10.134.3.4", "NORMAL"),
					},
					"/10.134.3.5": {
						SimpleStates: map[string]string{
							"/10.134.3.4": "UP",
							"/10.134.3.5": "UP",
						},
						EndpointState: endpointState("/10.134.3.5", "NORMAL"),
					},
				},
				podIPs: map[string]string{
					"/10.134.3.5": "/172.16.16.5",
				},
			},
			expectedStatus: http.StatusNotFound,
			expectedBody:   []byte(`{"/10.134.3.4":"DOWN","/10.134.3.5":"UP"}`),
		},
		{
			name:          "broadcast address is set",
			remoteAddr:    "172.16.16.213",
			broadcastAddr: "43.23.111.213",
			state: state{
				nodes: map[string]nodeState{
					"/43.23.111.212": {
						SimpleStates:  map[string]string{"/43.23.111.212": "UP", "/43.23.111.213": "UP"},
						EndpointState: endpointState("/43.23.111.212", "NORMAL"),
					},
					"/43.23.111.213": {
						SimpleStates:  map[string]string{"/43.23.111.212": "UP", "/43.23.111.213": "UP"},
						EndpointState: endpointState("/43.23.111.213", "NORMAL"),
					},
				},
				podIPs: map[string]string{
					"/43.23.111.212": "/172.16.16.212",
					"/43.23.111.213": "/172.16.16.213",
				},
			},
			expectedState: state{
				nodes: map[string]nodeState{
					"/43.23.111.212": {
						SimpleStates:  map[string]string{"/43.23.111.212": "UP", "/43.23.111.213": "UP"},
						EndpointState: endpointState("/43.23.111.212", "NORMAL"),
					},
					"/43.23.111.213": {
						SimpleStates:  map[string]string{"/43.23.111.212": "UP", "/43.23.111.213": "UP"},
						EndpointState: endpointState("/43.23.111.213", "NORMAL"),
					},
				},
				podIPs: map[string]string{
					"/43.23.111.212": "/172.16.16.212",
					"/43.23.111.213": "/172.16.16.213",
				},
			},
			expectedStatus: http.StatusOK,
			expectedBody:   []byte(`{"/43.23.111.212":"UP","/43.23.111.213":"UP"}`),
		},
		{
			name:          "none of the nodes are ready",
			remoteAddr:    "172.16.16.213",
			broadcastAddr: "43.23.111.213",
			state: state{
				nodes: map[string]nodeState{
					"/43.23.111.212": {
						SimpleStates:  map[string]string{"/43.23.111.212": "DOWN", "/43.23.111.213": "DOWN"},
						EndpointState: endpointState("/43.23.111.212", "DOWN"),
					},
					"/43.23.111.213": {
						SimpleStates:  map[string]string{"/43.23.111.212": "DOWN", "/43.23.111.213": "DOWN"},
						EndpointState: endpointState("/43.23.111.213", "DOWN"),
					},
				},
				podIPs: map[string]string{
					"/43.23.111.212": "/172.16.16.212",
					"/43.23.111.213": "/172.16.16.213",
				},
			},
			expectedState: state{
				nodes: map[string]nodeState{
					"/43.23.111.212": {
						SimpleStates:  map[string]string{"/43.23.111.212": "DOWN", "/43.23.111.213": "DOWN"},
						EndpointState: endpointState("/43.23.111.212", "DOWN"),
					},
					"/43.23.111.213": {
						SimpleStates:  map[string]string{"/43.23.111.212": "DOWN", "/43.23.111.213": "DOWN"},
						EndpointState: endpointState("/43.23.111.213", "DOWN"),
					},
				},
				podIPs: map[string]string{
					"/43.23.111.212": "/172.16.16.212",
					"/43.23.111.213": "/172.16.16.213",
				},
			},
			expectedStatus: http.StatusNotFound,
			expectedBody:   []byte(`{"/43.23.111.212":"DOWN","/43.23.111.213":"DOWN"}`),
		},
	}

	for _, testCase := range testCases {
		var testProber = NewProber(
			config.Config{},
			&jolokiaMock{},
			UserAuth{},
			&kubernetes.Clientset{},
			zap.NewNop().Sugar(),
		)

		testProber.state = testCase.state

		request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/healthz/%s", testCase.broadcastAddr), nil)
		recorder := httptest.NewRecorder()
		router := httprouter.New()
		setupRoutes(router, testProber)

		router.ServeHTTP(recorder, request)
		b, err := io.ReadAll(recorder.Result().Body)
		t.Log(testCase.name)
		asserts.Expect(err).ToNot(gomega.HaveOccurred())
		asserts.Expect(string(b)).To(gomega.Equal(string(testCase.expectedBody)))
		asserts.Expect(testProber.state).To(gomega.Equal(testCase.expectedState))
		asserts.Expect(recorder.Code).To(gomega.Equal(testCase.expectedStatus))
	}
}

func TestPing(t *testing.T) {
	asserts := gomega.NewWithT(t)
	testProber := &Prober{
		auth: UserAuth{},
		log:  zap.NewNop().Sugar(),
	}

	request := httptest.NewRequest(http.MethodGet, "/ping", nil)
	recorder := httptest.NewRecorder()
	router := httprouter.New()
	setupRoutes(router, testProber)

	router.ServeHTTP(recorder, request)
	b, err := io.ReadAll(recorder.Result().Body)
	asserts.Expect(err).ToNot(gomega.HaveOccurred())
	asserts.Expect(b).To(gomega.Equal([]byte("pong")))
	asserts.Expect(recorder.Code).To(gomega.Equal(http.StatusOK))
}

func TestGetRegionReady(t *testing.T) {
	asserts := gomega.NewWithT(t)
	testProber := &Prober{
		auth: UserAuth{
			User:     "cassandra",
			Password: "cassandra",
		},
		log: zap.NewNop().Sugar(),
		state: state{
			regionReady: false,
		},
	}

	request := httptest.NewRequest(http.MethodGet, "/region-ready", nil)
	request.SetBasicAuth("cassandra", "cassandra")
	recorder := httptest.NewRecorder()
	router := httprouter.New()
	setupRoutes(router, testProber)

	router.ServeHTTP(recorder, request)
	b, err := io.ReadAll(recorder.Result().Body)
	asserts.Expect(err).ToNot(gomega.HaveOccurred())
	asserts.Expect(b).To(gomega.Equal([]byte("false")))
	asserts.Expect(recorder.Code).To(gomega.Equal(http.StatusOK))

	testProber.state.regionReady = true
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	b, err = io.ReadAll(recorder.Result().Body)
	asserts.Expect(err).ToNot(gomega.HaveOccurred())
	asserts.Expect(b).To(gomega.Equal([]byte("true")))
	asserts.Expect(recorder.Code).To(gomega.Equal(http.StatusOK))
}

func TestPutRegionReady(t *testing.T) {
	testCases := []struct {
		requestBody           io.Reader
		expectedCode          int
		expectedLocalDCsState bool
	}{
		{
			requestBody:           bytes.NewReader([]byte("true")),
			expectedCode:          http.StatusOK,
			expectedLocalDCsState: true,
		},
		{
			requestBody:           bytes.NewReader([]byte("false")),
			expectedCode:          http.StatusOK,
			expectedLocalDCsState: false,
		},
		{
			requestBody:           bytes.NewReader([]byte("invalid")),
			expectedCode:          http.StatusBadRequest,
			expectedLocalDCsState: false,
		},
		{
			requestBody:           failingReader("err"),
			expectedCode:          http.StatusInternalServerError,
			expectedLocalDCsState: false,
		},
	}

	for _, testCase := range testCases {
		asserts := gomega.NewWithT(t)
		testProber := &Prober{
			auth: UserAuth{
				User:     "cassandra",
				Password: "cassandra",
			},
			log:   zap.NewNop().Sugar(),
			state: state{},
		}

		router := httprouter.New()
		setupRoutes(router, testProber)

		request := httptest.NewRequest(http.MethodPut, "/region-ready", testCase.requestBody)
		request.SetBasicAuth("cassandra", "cassandra")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		asserts.Expect(recorder.Code).To(gomega.Equal(testCase.expectedCode))
		asserts.Expect(testProber.state.regionReady).To(gomega.Equal(testCase.expectedLocalDCsState))
	}
}

func TestGetSeeds(t *testing.T) {
	asserts := gomega.NewWithT(t)
	testProber := &Prober{
		auth: UserAuth{
			User:     "cassandra",
			Password: "cassandra",
		},
		log: zap.NewNop().Sugar(),
		state: state{
			seeds: []string{"seed1", "seed2"},
		},
	}
	router := httprouter.New()
	setupRoutes(router, testProber)

	request := httptest.NewRequest(http.MethodGet, "/seeds", nil)
	request.SetBasicAuth("cassandra", "cassandra")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)
	asserts.Expect(recorder.Code).To(gomega.Equal(http.StatusOK))
	b, err := io.ReadAll(recorder.Result().Body)
	asserts.Expect(err).ToNot(gomega.HaveOccurred())
	asserts.Expect(b).To(gomega.BeEquivalentTo([]byte("[\"seed1\",\"seed2\"]")))
}

func TestPutSeeds(t *testing.T) {
	asserts := gomega.NewWithT(t)

	testCases := []struct {
		requestBody       io.Reader
		expectedCode      int
		expectedSeedState []string
	}{
		{
			requestBody:       bytes.NewReader([]byte("[\"seed1\",\"seed2\"]")),
			expectedCode:      http.StatusOK,
			expectedSeedState: []string{"seed1", "seed2"},
		},
		{
			requestBody:       bytes.NewReader([]byte("invalid")),
			expectedCode:      http.StatusBadRequest,
			expectedSeedState: nil,
		},
		{
			requestBody:       failingReader("err"),
			expectedCode:      http.StatusInternalServerError,
			expectedSeedState: nil,
		},
	}

	for _, testCase := range testCases {
		testProber := &Prober{
			auth: UserAuth{
				User:     "cassandra",
				Password: "cassandra",
			},
			log:   zap.NewNop().Sugar(),
			state: state{},
		}

		router := httprouter.New()
		setupRoutes(router, testProber)

		request := httptest.NewRequest(http.MethodPut, "/seeds", testCase.requestBody)
		request.SetBasicAuth("cassandra", "cassandra")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		asserts.Expect(recorder.Code).To(gomega.Equal(testCase.expectedCode))
		asserts.Expect(testProber.state.seeds).To(gomega.Equal(testCase.expectedSeedState))
	}
}

func TestGetDCs(t *testing.T) {
	asserts := gomega.NewWithT(t)
	testProber := &Prober{
		auth: UserAuth{
			User:     "cassandra",
			Password: "cassandra",
		},
		log: zap.NewNop().Sugar(),
		state: state{
			dcs: []dc{{Name: "dc1", Replicas: 3}, {Name: "dc2", Replicas: 4}},
		},
	}
	router := httprouter.New()
	setupRoutes(router, testProber)

	request := httptest.NewRequest(http.MethodGet, "/dcs", nil)
	request.SetBasicAuth("cassandra", "cassandra")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	asserts.Expect(recorder.Code).To(gomega.Equal(http.StatusOK))
	b, err := io.ReadAll(recorder.Result().Body)
	asserts.Expect(err).ToNot(gomega.HaveOccurred())
	asserts.Expect(b).To(gomega.BeEquivalentTo([]byte("[{\"name\":\"dc1\",\"replicas\":3},{\"name\":\"dc2\",\"replicas\":4}]")))
}

func TestPutDCs(t *testing.T) {
	asserts := gomega.NewWithT(t)

	testCases := []struct {
		requestBody  io.Reader
		expectedCode int
		expectedDCs  []dc
	}{
		{
			requestBody:  bytes.NewReader([]byte("[{\"name\":\"dc1\",\"replicas\":3},{\"name\":\"dc2\",\"replicas\":4}]")),
			expectedCode: http.StatusOK,
			expectedDCs:  []dc{{Name: "dc1", Replicas: 3}, {Name: "dc2", Replicas: 4}},
		},
		{
			requestBody:  bytes.NewReader([]byte("invalid")),
			expectedCode: http.StatusBadRequest,
			expectedDCs:  nil,
		},
		{
			requestBody:  failingReader("err"),
			expectedCode: http.StatusInternalServerError,
			expectedDCs:  nil,
		},
	}

	for _, testCase := range testCases {
		testProber := &Prober{
			auth: UserAuth{
				User:     "cassandra",
				Password: "cassandra",
			},
			log:   zap.NewNop().Sugar(),
			state: state{},
		}

		router := httprouter.New()
		setupRoutes(router, testProber)

		request := httptest.NewRequest(http.MethodPut, "/dcs", testCase.requestBody)
		request.SetBasicAuth("cassandra", "cassandra")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		asserts.Expect(recorder.Code).To(gomega.Equal(testCase.expectedCode))
		asserts.Expect(testProber.state.dcs).To(gomega.Equal(testCase.expectedDCs))
	}
}

type failingReader string

func (f failingReader) Read(_ []byte) (n int, err error) { return 0, errors.New(string(f)) }
