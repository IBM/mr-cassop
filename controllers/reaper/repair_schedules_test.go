package reaper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
)

func TestRepairSchedules(t *testing.T) {
	asserts := NewWithT(t)
	clusterName := "test-cluster"
	repair := v1alpha1.RepairSchedule{
		Keyspace:            "test_keyspace",
		Tables:              []string{"test_table"},
		ScheduleDaysBetween: 7,
		ScheduleTriggerTime: "2020-01-01T12:00:00",
		Datacenters:         []string{"dc1"},
		RepairThreadCount:   2,
		Intensity:           "1.0",
		IncrementalRepair:   false,
		RepairParallelism:   "DATACENTER_AWARE",
	}
	badRepair := v1alpha1.RepairSchedule{
		Keyspace:            "no",
		Tables:              []string{"no"},
		ScheduleDaysBetween: 7,
		ScheduleTriggerTime: "2020-01-01T12:00:00",
		Datacenters:         []string{"dc1"},
		RepairThreadCount:   2,
		Intensity:           "1.0",
		IncrementalRepair:   false,
		RepairParallelism:   "DATACENTER_AWARE",
	}

	tests := []test{
		{
			name:         "returns no error if repair is successfully scheduled",
			context:      context.Background(),
			handler:      handleResponseStatus(http.StatusOK),
			errorMatcher: BeNil(),
			params: map[string]interface{}{
				"clusterName": clusterName,
				"repair":      repair,
			},
		},
		{
			name:         "returns error if cluster is not found",
			context:      context.Background(),
			handler:      handleResponseError(testError, http.StatusNotFound),
			errorMatcher: BeEquivalentTo(ClusterNotFound),
			params: map[string]interface{}{
				"clusterName": "no",
				"repair":      repair,
			},
		},
		{
			name:         "returns error if repair object is invalid",
			context:      context.Background(),
			handler:      handleResponseError(testError, http.StatusBadRequest),
			errorMatcher: BeEquivalentTo(&requestFailedWithStatus{code: http.StatusBadRequest, message: "test error message\n"}),
			params: map[string]interface{}{
				"clusterName": "no",
				"repair":      badRepair,
			},
		},
		{
			name:         "returns error if response status code >= 300",
			context:      context.Background(),
			handler:      handleResponseError(testError, http.StatusForbidden),
			errorMatcher: BeEquivalentTo(&requestFailedWithStatus{code: http.StatusForbidden, message: "test error message\n"}),
			params: map[string]interface{}{
				"clusterName": clusterName,
				"repair":      repair,
			},
		},
		{
			name:         "returns error if context is nil",
			context:      nil,
			handler:      handleResponseError(testError, http.StatusInternalServerError),
			errorMatcher: BeEquivalentTo(nilContextError),
			params: map[string]interface{}{
				"clusterName": clusterName,
				"repair":      repair,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(tc.handler)
			reaperUrl, err := url.Parse(ts.URL)
			asserts.Expect(err).To(BeNil())
			rc := NewReaperClient(reaperUrl, tc.params["clusterName"].(string), defaultClient)
			err = rc.CreateRepairSchedule(tc.context, tc.params["repair"].(v1alpha1.RepairSchedule))
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
		asserts.Expect(err).To(BeNil())
		rc := NewReaperClient(reaperUrl, clusterName, &http.Client{
			Timeout: 100 * time.Microsecond,
		})
		err = rc.CreateRepairSchedule(tc.context, repair)
		asserts.Expect(err.Error()).To(tc.errorMatcher)
		ts.Close()
	})
}
