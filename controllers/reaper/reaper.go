package reaper

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/go-querystring/query"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"net/http"
	"net/url"
)

type reaperClient struct {
	baseUrl *url.URL
	client  *http.Client
}

type ReaperClient interface {
	IsRunning(ctx context.Context) (bool, error)
	ClusterExists(ctx context.Context, clusterName string) (bool, error)
	AddCluster(ctx context.Context, clusterName, seed string) error
	ScheduleRepair(ctx context.Context, clusterName string, repair dbv1alpha1.Repair) error
}

var (
	ClusterNotFound = errors.New("Cassandra cluster not found")
)

type requestFailedWithStatus struct {
	code int
}

func (e *requestFailedWithStatus) Error() string {
	return fmt.Sprintf("Request failed with status code %d", e.code)
}

func NewReaperClient(url *url.URL, client *http.Client) ReaperClient {
	return &reaperClient{url, client}
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
	resp, err := r.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusNoContent, nil
}

func (r reaperClient) ClusterExists(ctx context.Context, clusterName string) (bool, error) {
	route := r.url("/cluster/" + clusterName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, route, nil)
	if err != nil {
		return false, err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, &requestFailedWithStatus{resp.StatusCode}
	}
	return true, nil
}

func (r reaperClient) AddCluster(ctx context.Context, clusterName, seed string) error {
	route := r.url("/cluster/" + clusterName)
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

func (r reaperClient) ScheduleRepair(ctx context.Context, clusterName string, repair dbv1alpha1.Repair) error {
	route := r.url("/repair_schedule")
	// Reaper API requires URL query params instead of body
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, route, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	repairValues, _ := query.Values(repair)
	repairValues.Add("clusterName", clusterName)
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
