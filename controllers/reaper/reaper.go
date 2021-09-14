package reaper

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

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
	CreateRepairSchedule(ctx context.Context, repair dbv1alpha1.RepairSchedule) error
	RepairSchedules(ctx context.Context) ([]RepairSchedule, error)
	DeleteRepairSchedule(ctx context.Context, repairScheduleID string) error
	SetRepairScheduleState(ctx context.Context, repairScheduleID string, active bool) error
	RunRepair(ctx context.Context, keyspace, cause string) error
}

var (
	ClusterNotFound = errors.New("cassandra cluster not found")
)

type requestFailedWithStatus struct {
	code int
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

func (r *reaperClient) url(path string) string {
	return r.baseUrl.String() + path
}

func (r *reaperClient) IsRunning(ctx context.Context) (bool, error) {
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

func (r *reaperClient) ClusterExists(ctx context.Context) (bool, error) {
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

func (r *reaperClient) AddCluster(ctx context.Context, seed string) error {
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
