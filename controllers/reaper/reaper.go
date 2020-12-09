package reaper

import (
	"context"
	"fmt"
	"github.com/google/go-querystring/query"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"net/url"
)

type reaperClient struct {
	baseUrl *url.URL
}

type Client interface {
	IsRunning(ctx context.Context) (bool, error)
	ClusterExists(ctx context.Context, name string) (bool, error)
	AddCluster(ctx context.Context, name, seed string) error
	ScheduleRepair(ctx context.Context, clusterName string, repair dbv1alpha1.Repair) error
}

func NewReaperClient(host string) (Client, error) {
	baseUrl, err := url.Parse(host)
	if err != nil {
		return nil, err
	}
	return &reaperClient{baseUrl: baseUrl}, nil
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
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	return resp.StatusCode == http.StatusNoContent, nil
}

func (r reaperClient) ClusterExists(ctx context.Context, name string) (bool, error) {
	route := r.url("/cluster/" + name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, route, nil)
	if err != nil {
		return false, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	if resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, fmt.Errorf("Request failed with status code %d", resp.StatusCode)
	}
	return true, nil
}

func (r reaperClient) AddCluster(ctx context.Context, name, seed string) error {
	route := r.url("/cluster/" + name)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, route, nil)
	if err != nil {
		return errors.Wrap(err, "Failed to create request")
	}
	req.Header.Set("Accept", "application/json")
	q := req.URL.Query()
	q.Add("seedHost", seed)
	req.URL.RawQuery = q.Encode()
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "Request to reaper API failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("Cassandra cluster %s not found", name)
		}
		return fmt.Errorf("Request failed with status code %d", resp.StatusCode)
	}
	return nil
}

func (r reaperClient) ScheduleRepair(ctx context.Context, clusterName string, repair dbv1alpha1.Repair) error {
	route := r.url("/repair_schedule")
	// Reaper API requires URL query params instead of body
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, route, nil)
	if err != nil {
		return errors.Wrap(err, "Failed to create request")
	}
	req.Header.Set("Accept", "application/json")
	repairValues, _ := query.Values(repair)
	repairValues.Add("clusterName", clusterName)
	req.URL.RawQuery = repairValues.Encode()
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "Request to reaper API failed")
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("Request to reaper API failed: %s", string(body))
		}
		return fmt.Errorf("Request failed with status code %d", resp.StatusCode)
	}
	return nil
}
