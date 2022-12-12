package jolokia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ibm/cassandra-operator/api/v1alpha1"

	"github.com/pkg/errors"

	"go.uber.org/zap"
)

type Client struct {
	url    string
	client *http.Client
	auth   auth
	log    *zap.SugaredLogger
}

type JMXRequest struct {
	Type       string   `json:"type"`
	Mbean      string   `json:"mbean"`
	Attributes []string `json:"attribute,omitempty"`
	Operation  string   `json:"operation,omitempty"`
	Arguments  []string `json:"arguments,omitempty"` //args are identified based on the order they are passed
}

type JMXResponse struct {
	Request json.RawMessage `json:"request"`
	Value   json.RawMessage `json:"value"`
	Status  int             `json:"status"`
	Error   string          `json:"error"`
}

type auth struct {
	username string
	password string
}

type request struct {
	JMXRequest
	Target Target `json:"target"`
}

type Target struct {
	URL      string `json:"url"`
	User     string `json:"user"`
	Password string `json:"password"`
}

func NewClient(jolokiaURL string, username, password string, logr *zap.SugaredLogger) *Client {
	return &Client{
		url: jolokiaURL,
		client: &http.Client{
			Timeout: 0,
		},
		auth: auth{
			username: username,
			password: password,
		},
		log: logr,
	}
}

func (j *Client) Post(ctx context.Context, jmxReq JMXRequest, ip string) (JMXResponse, error) {
	req := request{
		Target: Target{
			URL:      jmxUrl(ip, v1alpha1.JmxPort),
			User:     j.auth.username,
			Password: j.auth.password,
		},
		JMXRequest: jmxReq,
	}

	b, err := json.Marshal(req)
	if err != nil {
		return JMXResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, j.url, bytes.NewReader(b))
	if err != nil {
		return JMXResponse{}, err
	}
	resp, err := j.client.Do(httpReq)
	if err != nil {
		return JMXResponse{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return JMXResponse{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return JMXResponse{}, errors.New(resp.Status + ": " + string(responseBody))
	}

	var response JMXResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return JMXResponse{}, fmt.Errorf("cannot unmarshall response. Error %s. Raw body: %s", err.Error(), string(responseBody))
	}

	if len(response.Error) != 0 {
		errMsg := fmt.Sprintf("received error JMX response; error: %s", response.Error)
		if j.log.Desugar().Core().Enabled(zap.DebugLevel) {
			errMsg += "; raw response: " + string(responseBody)
		}
		return JMXResponse{}, errors.New(errMsg)
	}

	return response, nil
}

func jmxUrl(ip string, port int) string {
	return fmt.Sprintf("service:jmx:rmi:///jndi/rmi://%s:%d/jmxrmi", ip, port)
}
