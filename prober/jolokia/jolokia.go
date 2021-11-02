package jolokia

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/json-iterator/go/extra"
	"k8s.io/apimachinery/pkg/runtime"
)

type Client struct {
	url string
	*http.Client
}

type Response struct {
	Request, Value interface{}
	Status         int
	Error          string
}

type jmxRequest struct{ Type, Mbean, Attribute string }

func NewClient(host, port string, timeout time.Duration) Client {
	return Client{
		url:    fmt.Sprintf("http://%s:%s/jolokia", host, port),
		Client: &http.Client{Timeout: timeout},
	}
}

func jmxUrl(ip, port string) string {
	return fmt.Sprintf("service:jmx:rmi:///jndi/rmi:/%s:%s/jmxrmi", ip, port)
}

func (j *Client) Post(body []byte) ([]byte, error) {
	resp, err := j.Client.Post(j.url, runtime.ContentTypeJSON, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status + ": " + string(responseBody))
	}

	return responseBody, nil
}

func ProxyRequests(request jmxRequest, user, password, port string, ips ...string) []byte {
	type Target struct{ Url, User, Password string }
	type Request struct {
		jmxRequest
		Target Target
	}
	reqs := make([]Request, 0, len(ips))
	for _, ip := range ips {
		r := Request{request, Target{jmxUrl(ip, port), user, password}}
		reqs = append(reqs, r)
	}

	extra.SetNamingStrategy(extra.LowerCaseWithUnderscores)
	body, _ := jsoniter.Marshal(reqs)
	return body
}
