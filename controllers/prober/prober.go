package prober

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/pkg/errors"
)

type ProberClient interface {
	Ready(ctx context.Context) (bool, error)
	GetSeeds(ctx context.Context, host string) ([]string, error)
	UpdateSeeds(ctx context.Context, seeds []string) error
	GetDCs(ctx context.Context, host string) ([]v1alpha1.DC, error)
	UpdateDCs(ctx context.Context, dcs []v1alpha1.DC) error
	UpdateRegionStatus(ctx context.Context, ready bool) error
	RegionReady(ctx context.Context, host string) (bool, error)
	ReaperReady(ctx context.Context, host string) (bool, error)
	UpdateReaperStatus(ctx context.Context, ready bool) error
	GetRegionIPs(ctx context.Context, host string) ([]string, error)
	UpdateRegionIPs(ctx context.Context, ips []string) error
	GetReaperIPs(ctx context.Context, host string) ([]string, error)
	UpdateReaperIPs(ctx context.Context, ips []string) error
}

type proberClient struct {
	baseUrl *url.URL
	client  *http.Client
	auth    Auth
}

type Auth struct {
	Username string
	Password string
}

func NewProberClient(url *url.URL, client *http.Client, user, password string) ProberClient {
	return &proberClient{url, client, Auth{
		Username: user,
		Password: password,
	}}
}

func (p *proberClient) url(path string) string {
	return p.baseUrl.String() + path
}

func (p *proberClient) newRequestWithAuth(ctx context.Context, method, url string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Can't create %s request to `%s` endpoint", method, url))
	}

	req.SetBasicAuth(p.auth.Username, p.auth.Password)
	return req, nil
}

func (p *proberClient) Ready(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url("/ping"), nil)
	if err != nil {
		return false, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false, errors.Wrap(err, "GET request to prober's `/ping` endpoint failed")
	}

	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

func (p *proberClient) GetSeeds(ctx context.Context, host string) ([]string, error) {
	req, err := p.newRequestWithAuth(ctx, http.MethodGet, fmt.Sprintf("https://%s/seeds", host), nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "GET request to prober's `/seeds` failed")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []string{}, fmt.Errorf("response status %q (code %v) is not %q",
			http.StatusText(resp.StatusCode), resp.StatusCode, http.StatusText(http.StatusOK))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return []string{}, errors.Wrap(err, "Unable to read response body")
	}

	var seeds []string
	if err := json.Unmarshal(body, &seeds); err != nil {
		return []string{}, errors.Wrap(err, "Error unmarshalling response body")
	}

	return seeds, nil
}

func (p *proberClient) UpdateSeeds(ctx context.Context, seeds []string) error {
	body, _ := json.Marshal(seeds)
	req, err := p.newRequestWithAuth(ctx, http.MethodPut, p.url("/seeds"), body)
	if err != nil {
		return err
	}

	if _, err := p.client.Do(req); err != nil {
		return errors.Wrap(err, "PUT request to prober's `/seeds` endpoint failed")
	}

	return nil
}

func (p *proberClient) GetDCs(ctx context.Context, host string) ([]v1alpha1.DC, error) {
	req, err := p.newRequestWithAuth(ctx, http.MethodGet, fmt.Sprintf("https://%s/dcs", host), nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "GET request to prober's `/dcs` endpoint failed")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("response status %q (code %v) is not %q",
			http.StatusText(resp.StatusCode), resp.StatusCode, http.StatusText(http.StatusOK))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to read response body")
	}

	var dcs []v1alpha1.DC
	if err := json.Unmarshal(body, &dcs); err != nil {
		return nil, errors.Wrap(err, "Error unmarshalling response body")
	}

	return dcs, nil
}

func (p *proberClient) UpdateDCs(ctx context.Context, dcs []v1alpha1.DC) error {
	body, _ := json.Marshal(dcs)
	req, err := p.newRequestWithAuth(ctx, http.MethodPut, p.url("/dcs"), body)
	if err != nil {
		return err
	}

	if _, err := p.client.Do(req); err != nil {
		return errors.Wrap(err, "PUT request to prober's `/dcs` endpoint failed")
	}

	return nil
}

func (p *proberClient) UpdateRegionStatus(ctx context.Context, ready bool) error {
	req, err := p.newRequestWithAuth(ctx, http.MethodPut, p.url("/region-ready"), []byte(strconv.FormatBool(ready)))
	if err != nil {
		return errors.Wrap(err, "Can't create request")
	}

	if _, err := p.client.Do(req); err != nil {
		return errors.Wrap(err, "PUT request to prober's `region-ready` failed")
	}

	return nil
}

func (p *proberClient) RegionReady(ctx context.Context, host string) (bool, error) {
	req, err := p.newRequestWithAuth(ctx, http.MethodGet, fmt.Sprintf("https://%s/region-ready", host), nil)
	if err != nil {
		return false, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return false, errors.Wrap(err, "GET request to prober's `/region-ready` failed")
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("response status %q (code %v) is not %q",
			http.StatusText(resp.StatusCode), resp.StatusCode, http.StatusText(http.StatusOK))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	ready, err := strconv.ParseBool(string(bytes.TrimSpace(body)))
	if err != nil {
		return false, errors.Wrapf(err, "Unexpected response from prober. Expect true or false")
	}

	return ready, nil
}

func (p *proberClient) UpdateReaperStatus(ctx context.Context, ready bool) error {
	req, err := p.newRequestWithAuth(ctx, http.MethodPut, p.url("/reaper-ready"), []byte(strconv.FormatBool(ready)))
	if err != nil {
		return err
	}

	if _, err := p.client.Do(req); err != nil {
		return errors.Wrap(err, "PUT request to prober's `/reaper-ready` endpoint failed")
	}

	return nil
}

func (p *proberClient) ReaperReady(ctx context.Context, host string) (bool, error) {
	req, err := p.newRequestWithAuth(ctx, http.MethodGet, fmt.Sprintf("https://%s/reaper-ready", host), nil)
	if err != nil {
		return false, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return false, errors.Wrap(err, "GET request to prober's `/reaper-ready` endpoint failed")
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("response status %q (code %v) is not %q",
			http.StatusText(resp.StatusCode), resp.StatusCode, http.StatusText(http.StatusOK))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	ready, err := strconv.ParseBool(string(bytes.TrimSpace(body)))
	if err != nil {
		return false, errors.Wrapf(err, "Unexpected response from prober. Expect true or false")
	}

	return ready, nil
}

func (p *proberClient) UpdateRegionIPs(ctx context.Context, ips []string) error {
	body, _ := json.Marshal(ips)
	req, err := p.newRequestWithAuth(ctx, http.MethodPut, p.url("/region-ips"), body)
	if err != nil {
		return err
	}

	if _, err := p.client.Do(req); err != nil {
		return errors.Wrap(err, "PUT request to prober's `/region-ips` endpoint failed")
	}

	return nil
}

func (p *proberClient) GetRegionIPs(ctx context.Context, host string) ([]string, error) {
	req, err := p.newRequestWithAuth(ctx, http.MethodGet, fmt.Sprintf("https://%s/region-ips", host), nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return []string{}, errors.Wrap(err, "Get request to prober's `/region-ips` failed")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []string{}, fmt.Errorf("response status %q (code %v) is not %q",
			http.StatusText(resp.StatusCode), resp.Status, http.StatusText(http.StatusOK))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return []string{}, errors.Wrap(err, "Unable to read response body")
	}

	var ips []string
	if err := json.Unmarshal(body, &ips); err != nil {
		return []string{}, errors.Wrap(err, "Error unmarshalling response body")
	}

	return ips, nil
}

func (p *proberClient) UpdateReaperIPs(ctx context.Context, ips []string) error {
	body, _ := json.Marshal(ips)
	req, err := p.newRequestWithAuth(ctx, http.MethodPut, p.url("/reaper-ips"), body)
	if err != nil {
		return err
	}

	if _, err := p.client.Do(req); err != nil {
		return errors.Wrap(err, "PUT request to prober's `/reaper-ips` endpoint failed")
	}

	return nil
}

func (p *proberClient) GetReaperIPs(ctx context.Context, host string) ([]string, error) {
	req, err := p.newRequestWithAuth(ctx, http.MethodGet, fmt.Sprintf("https://%s/reaper-ips", host), nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return []string{}, errors.Wrap(err, "Get request to prober's `/reaper-ips` failed")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []string{}, fmt.Errorf("response status %q (code %v) is not %q",
			http.StatusText(resp.StatusCode), resp.Status, http.StatusText(http.StatusOK))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return []string{}, errors.Wrap(err, "Unable to read response body")
	}

	var ips []string
	if err := json.Unmarshal(body, &ips); err != nil {
		return []string{}, errors.Wrap(err, "Error unmarshalling response body")
	}

	return ips, nil
}
