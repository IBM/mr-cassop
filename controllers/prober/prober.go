package prober

import (
	"context"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

type ProberClient interface {
	Ready(ctx context.Context) (bool, error)
	Seeds(ctx context.Context) ([]string, error)
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

func (p proberClient) Ready(ctx context.Context) (bool, error) {
	route := p.url("/ping")
	proberReq, err := http.NewRequestWithContext(ctx, http.MethodGet, route, nil)
	if err != nil {
		return false, errors.Wrap(err, "Can't create request")
	}

	resp, err := p.client.Do(proberReq)
	if err != nil {
		return false, errors.Wrap(err, "Request to prober failed")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	return true, nil
}

func (p proberClient) Seeds(ctx context.Context) ([]string, error) {
	route := p.url("/seeds")
	proberReq, err := http.NewRequestWithContext(ctx, http.MethodGet, route, nil)
	if err != nil {
		return []string{}, errors.Wrap(err, "Can't create request")
	}

	resp, err := p.client.Do(proberReq)
	if err != nil {
		return []string{}, errors.Wrap(err, "Request to prober failed")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []string{}, errors.Wrap(nil, "Response status is not ok")
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []string{}, errors.Wrap(err, "Unable to read response body")
	}

	respBody := string(body)

	if len(respBody) == 0 {
		return []string{}, errors.Wrap(nil, "Prober response body is empty")
	}

	return strings.Split(respBody, ","), nil
}
