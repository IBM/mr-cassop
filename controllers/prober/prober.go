package prober

import (
	"context"
	"github.com/pkg/errors"
	"net/http"
	"net/url"
)

type ProberClient interface {
	Ready(ctx context.Context) (bool, error)
}

type proberClient struct {
	baseUrl *url.URL
}

func NewProberClient(url *url.URL) ProberClient {
	return &proberClient{baseUrl: url}
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

	resp, err := http.DefaultClient.Do(proberReq)
	if err != nil {
		return false, errors.Wrap(err, "Request to prober failed")
	}

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	return true, nil
}
