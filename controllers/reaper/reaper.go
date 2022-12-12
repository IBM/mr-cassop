package reaper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/pkg/errors"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
)

const (
	RepairStateRunning = "RUNNING"
	RepairStatePaused  = "PAUSED"

	OwnerCassandraOperator = "cassandra-operator"
)

type reaperClient struct {
	baseUrl           *url.URL
	client            *http.Client
	clusterName       string
	repairThreadCount int32
}

type ReaperClient interface {
	IsRunning(ctx context.Context) (bool, error)
	ClusterExists(ctx context.Context) (bool, error)
	Clusters(ctx context.Context) ([]string, error)
	DeleteCluster(ctx context.Context) error
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
	code    int
	message string
}

func (e *requestFailedWithStatus) Error() string {
	return fmt.Sprintf("Request failed with status code %d. Response body: %s", e.code, e.message)
}

func NewReaperClient(url *url.URL, clusterName string, client *http.Client, defaultRepairThreadCount int32) ReaperClient {
	return &reaperClient{
		baseUrl:           url,
		client:            client,
		clusterName:       clusterName,
		repairThreadCount: defaultRepairThreadCount,
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
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, &requestFailedWithStatus{code: resp.StatusCode, message: string(b)}
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
	b, _ := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound {
			return ClusterNotFound
		}
		return &requestFailedWithStatus{code: resp.StatusCode, message: string(b)}
	}
	return nil
}

func (r *reaperClient) Clusters(ctx context.Context) ([]string, error) {
	route := r.url("/cluster")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, route, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req = req.WithContext(ctx)
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	b, _ := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, &requestFailedWithStatus{code: resp.StatusCode, message: string(b)}
	}

	var clusters []string
	err = json.Unmarshal(b, &clusters)
	if err != nil {
		return nil, err
	}

	return clusters, nil
}

func (r *reaperClient) DeleteCluster(ctx context.Context) error {
	// ensure no repair are running
	repairRuns, err := r.getRepairRuns(ctx, "")
	if err != nil {
		return err
	}

	for _, repairRun := range repairRuns {
		err = r.deleteRepairRun(ctx, repairRun)
		if err != nil {
			return errors.Wrapf(err, "can't delete repair run %s for keyspace %s", repairRun.ID, repairRun.KeyspaceName)
		}
	}

	// ensure no repair schedules exist
	repairSchedules, err := r.RepairSchedules(ctx)
	if err != nil {
		return err
	}

	for _, repairSchedule := range repairSchedules {
		err = r.DeleteRepairSchedule(ctx, repairSchedule.ID)
		if err != nil {
			return errors.Wrapf(err, "can't delete repair schedule %s for keyspace %s", repairSchedule.ID, repairSchedule.KeyspaceName)
		}
	}

	route := r.url("/cluster/" + r.clusterName)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, route, nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	b, _ := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return &requestFailedWithStatus{code: resp.StatusCode, message: string(b)}
	}

	return nil
}

func (r *reaperClient) deleteRepairRun(ctx context.Context, repairRun RepairRun) error {
	if repairRun.State == RepairStateRunning {
		if err := r.setRepairState(ctx, repairRun.ID, RepairStatePaused); err != nil {
			return errors.Wrapf(err, "can't pause repair run %s for keyspace %s", repairRun.ID, repairRun.KeyspaceName)
		}
	}

	route := r.url("/repair_run/" + repairRun.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, route, nil)
	if err != nil {
		return err
	}
	q := req.URL.Query()
	q.Add("owner", repairRun.Owner)
	req.URL.RawQuery = q.Encode()
	req = req.WithContext(ctx)
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	b, _ := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return &requestFailedWithStatus{code: resp.StatusCode, message: string(b)}
	}

	return nil
}
