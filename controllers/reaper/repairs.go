package reaper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
)

type RepairRun struct {
	ID           string `json:"id"`
	State        string `json:"state"`
	Duration     string `json:"duration"`
	ClusterName  string `json:"cluster_name"`
	KeyspaceName string `json:"keyspace_name"`
	Owner        string `json:"owner"`
	Cause        string `json:"cause"`
}

// RunRepair Creates and starts a repair run
func (r *reaperClient) RunRepair(ctx context.Context, keyspace, cause string) error {
	existingRepairRuns, err := r.getRepairRuns(ctx, keyspace)
	if err != nil {
		return err
	}

	if len(existingRepairRuns) > 0 {
		for _, run := range existingRepairRuns {
			if run.State == "RUNNING" || run.State == "NOT_STARTED" {
				return nil // don't start a repair if there's one running or scheduled
			}
		}
	}

	repairRun, err := r.createRepairRun(ctx, keyspace, cause)
	if err != nil {
		return err
	}

	return r.setRepairState(ctx, repairRun.ID, RepairStateRunning)
}

func (r *reaperClient) createRepairRun(ctx context.Context, keyspace, cause string) (RepairRun, error) {
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
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return RepairRun{}, err
	}

	if resp.StatusCode >= 300 {
		return RepairRun{}, &requestFailedWithStatus{code: resp.StatusCode, message: string(body)}
	}

	createdRepair := RepairRun{}
	err = json.Unmarshal(body, &createdRepair)
	if err != nil {
		return RepairRun{}, err
	}

	return createdRepair, nil
}

func (r *reaperClient) getRepairRuns(ctx context.Context, keyspace string) ([]RepairRun, error) {
	route := r.url("/repair_run")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, route, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	urlParams := url.Values{}
	urlParams.Add("clusterName", r.clusterName)
	urlParams.Add("keyspace", keyspace)

	req.URL.RawQuery = urlParams.Encode()
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 300 {
		return nil, &requestFailedWithStatus{code: resp.StatusCode, message: string(body)}
	}

	repairRuns := []RepairRun{}
	err = json.Unmarshal(body, &repairRuns)
	if err != nil {
		return nil, err
	}

	return repairRuns, nil
}

func (r *reaperClient) setRepairState(ctx context.Context, runID, state string) error {
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
	b, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return &requestFailedWithStatus{code: resp.StatusCode, message: string(b)}
	}

	return nil
}
