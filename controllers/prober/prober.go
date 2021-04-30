package prober

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
	"golang.org/x/net/context/ctxhttp"
)

type ProberClient interface {
	Ready(ctx context.Context) (bool, error)
	GetSeeds(ctx context.Context, host string) ([]string, error)
	UpdateSeeds(ctx context.Context, seeds []string) error
}

type proberClient struct {
	baseUrl *url.URL
	client  *http.Client
}

func NewProberClient(url *url.URL, client *http.Client) ProberClient {
	return &proberClient{url, client}
}

func (p proberClient) url(path string) string {
	return p.baseUrl.String() + path
}

func (p *proberClient) Ready(ctx context.Context) (bool, error) {
	resp, err := ctxhttp.Get(ctx, p.client, p.url("/ping"))
	if err != nil {
		return false, errors.Wrap(err, "Request to prober failed")
	}

	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

func (p *proberClient) GetSeeds(ctx context.Context, host string) ([]string, error) {
	resp, err := ctxhttp.Get(ctx, p.client, fmt.Sprintf("https://%s/localseeds", host))
	if err != nil {
		return []string{}, errors.Wrap(err, "Request to prober failed")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []string{}, fmt.Errorf("response status %q (code %v) is not %q",
			http.StatusText(resp.StatusCode), resp.StatusCode, http.StatusText(http.StatusOK))
	}

	body, err := ioutil.ReadAll(resp.Body)
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
	req, err := http.NewRequest(http.MethodPut, p.url("/localseeds"), bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "Can't create request")
	}

	if _, err := ctxhttp.Do(ctx, p.client, req); err != nil {
		return errors.Wrap(err, "PUT Request to prober failed")
	}

	return nil
}
