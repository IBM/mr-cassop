package jolokia

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"

	"go.uber.org/zap"

	jsoniter "github.com/json-iterator/go"
	"github.com/json-iterator/go/extra"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	userRegExp = regexp.MustCompile(`("password":)".*?"`)
	passRegExp = regexp.MustCompile(`("user":)".*?"`)
)

type Jolokia interface {
	CassandraNodeState(ip string) (CassandraResponse, error)
	SetAuth(username, password string)
}

type Client struct {
	url string
	*http.Client
	auth    auth
	jmxPort int
	log     *zap.SugaredLogger
}

type Response struct {
	Request, Value interface{}
	Status         int
	Error          string
}

type auth struct {
	username string
	password string
}

type jmxRequest struct{ Type, Mbean, Attribute string }

type CassandraResponse struct {
	Response
	Value CassandraNodeState
}

func NewClient(jolokiaPort, jmxPort int, timeout time.Duration, logr *zap.SugaredLogger, username, password string) Jolokia {
	return &Client{
		url:    fmt.Sprintf("http://localhost:%d/jolokia", jolokiaPort),
		Client: &http.Client{Timeout: timeout},
		auth: auth{
			username: username,
			password: password,
		},
		jmxPort: jmxPort,
		log:     logr,
	}
}

func jmxUrl(ip string, port int) string {
	return fmt.Sprintf("service:jmx:rmi:///jndi/rmi:/%s:%d/jmxrmi", ip, port)
}

func (j *Client) CassandraNodeState(ip string) (CassandraResponse, error) {
	resp, err := j.Client.Post(j.url, runtime.ContentTypeJSON, bytes.NewReader(j.cassandraNodeStateRequest(ip)))
	if err != nil {
		return CassandraResponse{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return CassandraResponse{}, err
	}

	responseBodyModified := userRegExp.ReplaceAllString(string(responseBody), `$1"***"`)
	responseBodyModified = passRegExp.ReplaceAllString(responseBodyModified, `$1"***"`)

	if resp.StatusCode != http.StatusOK {
		return CassandraResponse{}, errors.New(resp.Status + ": " + responseBodyModified)
	}

	var responses []CassandraResponse
	if err := json.Unmarshal(responseBody, &responses); err != nil {
		return CassandraResponse{}, err
	}

	if len(responses) == 0 {
		return CassandraResponse{}, fmt.Errorf("empty response. Raw body: %s", responseBodyModified)
	}

	if len(responses[0].Error) != 0 {
		j.log.Debugf(responseBodyModified)
	}

	if len(responses) > 1 {
		j.log.Warnf("expected one response, %d given. Response body: %s", len(responses), responseBodyModified)
	}

	return responses[0], nil
}

func (j *Client) cassandraNodeStateRequest(ip string) []byte {
	type Target struct{ Url, User, Password string }
	type Request struct {
		jmxRequest
		Target Target
	}
	req := []Request{
		{
			jmxRequest: jmxRequest{
				Type:      "read",
				Mbean:     "org.apache.cassandra.net:type=FailureDetector",
				Attribute: "SimpleStates,AllEndpointStates",
			},
			Target: Target{jmxUrl(ip, j.jmxPort), j.auth.username, j.auth.password},
		},
	}

	extra.SetNamingStrategy(extra.LowerCaseWithUnderscores)
	body, _ := jsoniter.Marshal(req)
	return body
}

func (j *Client) SetAuth(username, password string) {
	j.auth.username = username
	j.auth.password = password
}
