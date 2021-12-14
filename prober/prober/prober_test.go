package prober

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/julienschmidt/httprouter"

	"github.com/ibm/cassandra-operator/prober/config"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
)

func TestProberInit(t *testing.T) {
	testProber := NewProber(config.Config{}, &jolokiaMock{}, UserAuth{}, &kubernetes.Clientset{}, zap.NewNop().Sugar())
	testRouter := httprouter.New()
	setupRoutes(testRouter, testProber)

	responseRecorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ping", nil)
	testRouter.ServeHTTP(responseRecorder, request)
	b, err := io.ReadAll(responseRecorder.Result().Body)
	if err != nil {
		t.Logf("failed to read body: %s", err.Error())
		t.Fail()
	}

	if string(b) != "pong" {
		t.Logf("failed to call ping endpoing")
		t.Fail()
	}
}
