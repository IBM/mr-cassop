package reaper

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/google/go-querystring/query"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
)

type RepairSchedule struct {
	ID                  string   `json:"id"`
	Owner               string   `json:"owner"`
	State               string   `json:"state"`
	Intensity           float64  `json:"intensity"` // value between 0.0 and 1.0, but must never be 0.0.
	KeyspaceName        string   `json:"keyspace_name"`
	ColumnFamilies      []string `json:"column_families"`
	SegmentCount        int32    `json:"segment_count"`
	RepairParallelism   string   `json:"repair_parallelism"`
	ScheduleDaysBetween int32    `json:"scheduled_days_between"`
	Datacenters         []string `json:"datacenters"`
	IncrementalRepair   bool     `json:"incremental_repair"`
	Tables              []string `json:"tables"`
	RepairThreadCount   int32    `json:"repair_thread_count"`
}

func (r *reaperClient) CreateRepairSchedule(ctx context.Context, repair dbv1alpha1.RepairSchedule) error {
	route := r.url("/repair_schedule")
	// Reaper API requires URL query params instead of body
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, route, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	repairValues, _ := query.Values(repair)
	repairValues.Add("clusterName", r.clusterName)
	repairValues.Add("owner", OwnerCassandraOperator)
	req.URL.RawQuery = repairValues.Encode()
	req = req.WithContext(ctx)
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return &requestFailedWithStatus{code: resp.StatusCode, message: string(b)}
	}

	return nil
}

func (r *reaperClient) RepairSchedules(ctx context.Context) ([]RepairSchedule, error) {
	route := r.url("/repair_schedule")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, route, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	queryParams := url.Values{}
	queryParams.Add("clusterName", r.clusterName)
	req.URL.RawQuery = queryParams.Encode()
	req = req.WithContext(ctx)
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 300 {
		return nil, &requestFailedWithStatus{code: resp.StatusCode, message: string(b)}
	}

	repairSchedules := make([]RepairSchedule, 0)
	err = json.Unmarshal(b, &repairSchedules)
	if err != nil {
		return nil, err
	}

	return repairSchedules, nil
}

func (r *reaperClient) DeleteRepairSchedule(ctx context.Context, repairScheduleID string) error {
	route := r.url(fmt.Sprintf("/repair_schedule/%s", repairScheduleID))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, route, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	queryParams := url.Values{}
	queryParams.Add("owner", OwnerCassandraOperator)
	req.URL.RawQuery = queryParams.Encode()
	req = req.WithContext(ctx)
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	b, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return &requestFailedWithStatus{code: resp.StatusCode, message: string(b)}
	}

	return nil
}

func (r *reaperClient) SetRepairScheduleState(ctx context.Context, repairScheduleID string, active bool) error {
	route := r.url(fmt.Sprintf("/repair_schedule/%s", repairScheduleID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, route, nil)
	if err != nil {
		return err
	}

	state := "ACTIVE"
	if !active {
		state = "PAUSED"
	}

	req.Header.Set("Accept", "application/json")
	queryParams := url.Values{}
	queryParams.Add("state", state)
	req.URL.RawQuery = queryParams.Encode()
	req = req.WithContext(ctx)
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	b, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return &requestFailedWithStatus{code: resp.StatusCode, message: string(b)}
	}

	return nil
}
