package prober

import (
	"context"
	"github.com/pkg/errors"
	"net/http"
)

type Client interface {
	Ready(ctx context.Context) (bool, error)
	ReadyAllDCs(ctx context.Context) (bool, error)
}

type proberClient struct {
	host string
}

func NewProberClient(host string) Client {
	return &proberClient{host: host}
}

func (p proberClient) Ready(ctx context.Context) (bool, error) {
	proberReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+p.host+"/ping", nil)
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

func (p proberClient) ReadyAllDCs(ctx context.Context) (bool, error) {
	proberReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+p.host+"/readyalldcs", nil)
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
