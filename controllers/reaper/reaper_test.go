package reaper

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

var (
	testError            = errors.New("test error message")
	nilContextError      = errors.New("net/http: nil Context")
	timeoutExceededError = errors.New("(Client.Timeout exceeded while awaiting headers)")
	defaultClient        = http.DefaultClient
)

type test struct {
	name           string
	context        context.Context
	handler        http.HandlerFunc
	errorMatcher   types.GomegaMatcher
	expectedResult bool
	params         map[string]interface{}
}

func TestRequestFailedWithStatusError(t *testing.T) {
	asserts := NewWithT(t)
	t.Run("returns error message", func(t *testing.T) {
		err := &requestFailedWithStatus{code: http.StatusInternalServerError}
		asserts.Expect(err).To(Not(BeNil()))
		asserts.Expect(err.Error()).To(Equal(fmt.Sprintf("Request failed with status code %d. Response body: %s", err.code, err.message)))
	})
}

func TestNewReaperClient(t *testing.T) {
	asserts := NewWithT(t)
	t.Run("returns reaper client", func(t *testing.T) {
		reaperUrl, err := url.Parse("http://127.0.0.1:12345")
		asserts.Expect(err).To(BeNil())
		clusterName := "test_cluster"
		rc := NewReaperClient(reaperUrl, clusterName, defaultClient, 1)
		asserts.Expect(rc).To(Equal(&reaperClient{
			baseUrl: &url.URL{
				Scheme: "http",
				Host:   "127.0.0.1:12345",
			},
			client:            defaultClient,
			clusterName:       clusterName,
			repairThreadCount: 1,
		}))
	})
}

func TestIsRunning(t *testing.T) {
	asserts := NewWithT(t)
	tests := []test{
		{
			name:           "returns true if reaper is running",
			context:        context.Background(),
			handler:        handleResponse("pong", http.StatusNoContent),
			expectedResult: true,
			errorMatcher:   BeNil(),
		},
		{
			name:           "returns false if reaper is not running",
			context:        context.Background(),
			handler:        handleResponseStatus(http.StatusNotFound),
			expectedResult: false,
			errorMatcher:   BeNil(),
		},
		{
			name:           "returns error if context is nil",
			context:        nil,
			handler:        handleResponseError(testError, http.StatusInternalServerError),
			expectedResult: false,
			errorMatcher:   BeEquivalentTo(nilContextError),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(tc.handler)
			reaperUrl, err := url.Parse(ts.URL)
			asserts.Expect(err).To(BeNil())
			rc := NewReaperClient(reaperUrl, "testCluster", defaultClient, 1)
			result, err := rc.IsRunning(tc.context)
			asserts.Expect(result).To(Equal(tc.expectedResult))
			asserts.Expect(err).To(tc.errorMatcher)
			ts.Close()
		})
	}
	tc := test{
		name:           "returns error if http request fails",
		context:        context.Background(),
		handler:        handleResponseError(testError, http.StatusInternalServerError),
		expectedResult: false,
		errorMatcher:   ContainSubstring(timeoutExceededError.Error()),
	}
	t.Run(tc.name, func(t *testing.T) {
		ts := httptest.NewServer(tc.handler)
		reaperUrl, err := url.Parse(ts.URL)
		asserts.Expect(err).To(BeNil())
		rc := NewReaperClient(reaperUrl, "testCluster", &http.Client{
			Timeout: 1 * time.Microsecond,
		}, 1)
		result, err := rc.IsRunning(tc.context)
		asserts.Expect(result).To(Equal(tc.expectedResult))
		asserts.Expect(err.Error()).To(tc.errorMatcher)
		ts.Close()
	})
}

func TestClusterExists(t *testing.T) {
	asserts := NewWithT(t)
	clusterName := "test-cluster"
	tests := []test{
		{
			name:           "returns true if cluster exists in reaper",
			context:        context.Background(),
			handler:        handleResponseStatus(http.StatusOK),
			expectedResult: true,
			errorMatcher:   BeNil(),
		},
		{
			name:           "returns false if cluster does not exist in reaper",
			context:        context.Background(),
			handler:        handleResponseError(testError, http.StatusNotFound),
			expectedResult: false,
			errorMatcher:   BeNil(),
		},
		{
			name:           "returns error if response status code >= 300",
			context:        context.Background(),
			handler:        handleResponseError(testError, http.StatusForbidden),
			expectedResult: false,
			errorMatcher:   BeEquivalentTo(&requestFailedWithStatus{code: http.StatusForbidden, message: "test error message\n"}),
		},
		{
			name:           "returns error if context is nil",
			context:        nil,
			handler:        handleResponseError(testError, http.StatusInternalServerError),
			expectedResult: false,
			errorMatcher:   BeEquivalentTo(nilContextError),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(tc.handler)
			reaperUrl, err := url.Parse(ts.URL)
			asserts.Expect(err).To(BeNil())
			rc := NewReaperClient(reaperUrl, clusterName, defaultClient, 1)
			result, err := rc.ClusterExists(tc.context)
			asserts.Expect(result).To(Equal(tc.expectedResult))
			asserts.Expect(err).To(tc.errorMatcher)
			ts.Close()
		})
	}
	tc := test{
		name:           "returns error if http request fails",
		context:        context.Background(),
		handler:        handleResponseError(testError, http.StatusInternalServerError),
		expectedResult: false,
		errorMatcher:   ContainSubstring(timeoutExceededError.Error()),
	}
	t.Run(tc.name, func(t *testing.T) {
		ts := httptest.NewServer(tc.handler)
		reaperUrl, err := url.Parse(ts.URL)
		asserts.Expect(err).To(BeNil())
		rc := NewReaperClient(reaperUrl, clusterName, &http.Client{
			Timeout: 1 * time.Microsecond,
		}, 1)
		result, err := rc.ClusterExists(tc.context)
		asserts.Expect(result).To(Equal(tc.expectedResult))
		asserts.Expect(err.Error()).To(tc.errorMatcher)
		ts.Close()
	})
}

func TestAddCluster(t *testing.T) {
	asserts := NewWithT(t)
	clusterName := "test-cluster"
	seed := "example-cassandra-dc1-0.cassandra.svc.cluster.local"
	tests := []test{
		{
			name:         "returns no error if cluster is added successfully",
			context:      context.Background(),
			handler:      handleResponseStatus(http.StatusOK),
			errorMatcher: BeNil(),
		},
		{
			name:         "returns error if cluster does not exist in reaper",
			context:      context.Background(),
			handler:      handleResponseError(testError, http.StatusNotFound),
			errorMatcher: BeEquivalentTo(ClusterNotFound),
		},
		{
			name:         "returns error if response status code >= 300",
			context:      context.Background(),
			handler:      handleResponseError(testError, http.StatusForbidden),
			errorMatcher: BeEquivalentTo(&requestFailedWithStatus{code: http.StatusForbidden, message: "test error message\n"}),
		},
		{
			name:         "returns error if context is nil",
			context:      nil,
			handler:      handleResponseError(testError, http.StatusInternalServerError),
			errorMatcher: BeEquivalentTo(nilContextError),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(tc.handler)
			reaperUrl, err := url.Parse(ts.URL)
			asserts.Expect(err).To(BeNil())
			rc := NewReaperClient(reaperUrl, clusterName, defaultClient, 1)
			err = rc.AddCluster(tc.context, seed)
			asserts.Expect(err).To(tc.errorMatcher)
			ts.Close()
		})
	}
	tc := test{
		name:         "returns error if http request fails",
		context:      context.Background(),
		handler:      handleResponseError(testError, http.StatusInternalServerError),
		errorMatcher: ContainSubstring(timeoutExceededError.Error()),
	}
	t.Run(tc.name, func(t *testing.T) {
		ts := httptest.NewServer(tc.handler)
		reaperUrl, err := url.Parse(ts.URL)
		asserts.Expect(err).ToNot(HaveOccurred())
		rc := NewReaperClient(reaperUrl, clusterName, &http.Client{
			Timeout: 1 * time.Microsecond,
		}, 1)
		err = rc.AddCluster(tc.context, seed)
		asserts.Expect(err.Error()).To(tc.errorMatcher)
		ts.Close()
	})
}

func handleResponseStatus(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
	}
}

func handleResponse(response string, code int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
		fmt.Fprint(w, response)
	}
}

func handleResponseError(err error, code int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, err.Error(), code)
	}
}
