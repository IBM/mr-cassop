package prober

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"

	"github.com/ibm/cassandra-operator/prober/config"
	"github.com/ibm/cassandra-operator/prober/jolokia"
	"github.com/julienschmidt/httprouter"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
)

type Prober struct {
	cfg        config.Config
	jolokia    jolokia.Jolokia
	auth       UserAuth
	kubeClient *kubernetes.Clientset
	log        *zap.SugaredLogger
	state      state
}

type state struct {
	seeds       []string
	regionReady bool
	reaperReady bool
	dcs         []dc
	nodes       map[string]nodeState
}

type dc struct {
	Name     string `json:"name"`
	Replicas int    `json:"replicas"`
}

type UserAuth struct {
	User     string
	Password string
}

func NewProber(cfg config.Config, jolokiaClient jolokia.Jolokia, auth UserAuth, clientset *kubernetes.Clientset, logr *zap.SugaredLogger) *Prober {
	return &Prober{
		cfg:        cfg,
		jolokia:    jolokiaClient,
		auth:       auth,
		kubeClient: clientset,
		log:        logr,
		state: state{
			nodes: make(map[string]nodeState),
		},
	}
}

func (p *Prober) Run() error {
	router := httprouter.New()
	setupRoutes(router, p)

	authSecretCh := p.WatchAuthSecret()
	defer close(authSecretCh)

	baseSecretCh := p.WatchBaseSecret()
	defer close(baseSecretCh)

	go p.pollNodeStates()

	p.log.Infow("Cassandra's prober listening", "serverPort", p.cfg.ServerPort)
	return http.ListenAndServe(fmt.Sprintf(":%d", p.cfg.ServerPort), router)
}

func setupRoutes(router *httprouter.Router, prober *Prober) {
	router.GET("/healthz/:broadcastip", prometheusMiddleware(prober.healthCheck))
	router.GET("/ping", prometheusMiddleware(prober.ping))
	router.GET("/region-ready", prober.BasicAuth(prometheusMiddleware(prober.getRegionReady)))
	router.PUT("/region-ready", prober.BasicAuth(prometheusMiddleware(prober.putRegionReady)))
	router.GET("/reaper-ready", prober.BasicAuth(prometheusMiddleware(prober.getReaperReady)))
	router.PUT("/reaper-ready", prober.BasicAuth(prometheusMiddleware(prober.putReaperReady)))
	router.GET("/seeds", prober.BasicAuth(prometheusMiddleware(prober.getSeeds)))
	router.PUT("/seeds", prober.BasicAuth(prometheusMiddleware(prober.putSeeds)))
	router.GET("/dcs", prober.BasicAuth(prometheusMiddleware(prober.getDCs)))
	router.PUT("/dcs", prober.BasicAuth(prometheusMiddleware(prober.putDCs)))
	router.Handler("GET", "/metrics", promhttp.Handler())
}

func (p *Prober) BasicAuth(h httprouter.Handle) httprouter.Handle {
	return func(writer http.ResponseWriter, request *http.Request, params httprouter.Params) {
		user, password, hasAuth := request.BasicAuth()

		if hasAuth && user == p.auth.User && password == p.auth.Password {
			// Delegate request to the given handle
			h(writer, request, params)
		} else {
			// Request Basic Authentication otherwise
			writer.Header().Set("WWW-Authenticate", "Basic realm=Restricted")
			http.Error(writer, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		}
	}
}
