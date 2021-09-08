package reaper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/google/go-querystring/query"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
)

const (
	RepairStateRunning = "RUNNING"

	OwnerCassandraOperator = "cassandra-operator"
)

type reaperClient struct {
	baseUrl     *url.URL
	client      *http.Client
	clusterName string
}

type ReaperClient interface {
	IsRunning(ctx context.Context) (bool, error)
	ClusterExists(ctx context.Context) (bool, error)
	AddCluster(ctx context.Context, seed string) error
	ScheduleRepair(ctx context.Context, repair dbv1alpha1.Repair) error
	RunRepair(ctx context.Context, keyspace, cause string) error
}

var (
	ClusterNotFound = errors.New("cassandra cluster not found")
)

type requestFailedWithStatus struct {
	code int
}

type RepairRun struct {
	ID           string `json:"id"`
	State        string `json:"state"`
	Duration     string `json:"duration"`
	ClusterName  string `json:"cluster_name"`
	KeyspaceName string `json:"keyspace_name"`
	Owner        string `json:"owner"`
	Cause        string `json:"cause"`
}

func (e *requestFailedWithStatus) Error() string {
	return fmt.Sprintf("Request failed with status code %d", e.code)
}

func NewReaperClient(url *url.URL, clusterName string, client *http.Client) ReaperClient {
	return &reaperClient{
		baseUrl:     url,
		client:      client,
		clusterName: clusterName,
	}
}

func (r reaperClient) url(path string) string {
	return r.baseUrl.String() + path
}

func (r reaperClient) IsRunning(ctx context.Context) (bool, error) {
	route := r.url("/ping")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, route, nil)
	if err != nil {
		return false, err
	}
	req = req.WithContext(ctx)
	resp, err := r.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusNoContent, nil
}

func (r reaperClient) ClusterExists(ctx context.Context) (bool, error) {
	route := r.url("/cluster/" + r.clusterName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, route, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "application/json")
	req = req.WithContext(ctx)
	resp, err := r.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, &requestFailedWithStatus{code: resp.StatusCode}
	}
	return true, nil
}

func (r reaperClient) AddCluster(ctx context.Context, seed string) error {
	route := r.url("/cluster/" + r.clusterName)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, route, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	q := req.URL.Query()
	q.Add("seedHost", seed)
	req.URL.RawQuery = q.Encode()
	req = req.WithContext(ctx)
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound {
			return ClusterNotFound
		}
		return &requestFailedWithStatus{resp.StatusCode}
	}
	return nil
}

func (r reaperClient) ScheduleRepair(ctx context.Context, repair dbv1alpha1.Repair) error {
	route := r.url("/repair_schedule")
	// Reaper API requires URL query params instead of body
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, route, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	repairValues, _ := query.Values(repair)
	repairValues.Add("clusterName", r.clusterName)
	req.URL.RawQuery = repairValues.Encode()
	req = req.WithContext(ctx)
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound {
			return ClusterNotFound
		}
		return &requestFailedWithStatus{resp.StatusCode}
	}
	return nil
}

// RunRepair Creates and starts a repair run
func (r reaperClient) RunRepair(ctx context.Context, keyspace, cause string) error {
	repairRun, err := r.createRepairRun(ctx, keyspace, cause)
	if err != nil {
		return err
	}

	return r.setRepairState(ctx, repairRun.ID, RepairStateRunning)
}

func (r reaperClient) createRepairRun(ctx context.Context, keyspace, cause string) (RepairRun, error) {
	route := r.url("/repair_run")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, route, nil)
	if err != nil {
		return RepairRun{}, err
	}
	req.Header.Set("Accept", "application/json")
	urlParams := url.Values{}
	urlParams.Add("clusterName", r.clusterName)
	urlParams.Add("keyspace", keyspace)
	urlParams.Add("owner", OwnerCassandraOperator)
	urlParams.Add("cause", cause)

	req.URL.RawQuery = urlParams.Encode()
	req = req.WithContext(ctx)
	resp, err := r.client.Do(req)
	if err != nil {
		return RepairRun{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return RepairRun{}, &requestFailedWithStatus{resp.StatusCode}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return RepairRun{}, err
	}

	createdRepair := RepairRun{}
	err = json.Unmarshal(body, &createdRepair)
	if err != nil {
		return RepairRun{}, err
	}

	return createdRepair, nil
}

func (r reaperClient) setRepairState(ctx context.Context, runID, state string) error {
	route := r.url(fmt.Sprintf("/repair_run/%s/state/%s", runID, state))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, route, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req = req.WithContext(ctx)
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return &requestFailedWithStatus{resp.StatusCode}
	}

	return nil
}
