package prober

import (
	"fmt"
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

	go p.pollNodeStates()

	p.log.Infow("Cassandra's prober listening", "serverPort", p.cfg.ServerPort)
	return http.ListenAndServe(fmt.Sprintf(":%d", p.cfg.ServerPort), router)
}

func setupRoutes(router *httprouter.Router, prober *Prober) {
	router.GET("/healthz/:broadcastip", prober.healthCheck)
	router.GET("/ping", prober.ping)
	router.GET("/region-ready", prober.getRegionReady)
	router.PUT("/region-ready", prober.putRegionReady)
	router.GET("/reaper-ready", prober.getReaperReady)
	router.PUT("/reaper-ready", prober.putReaperReady)
	router.GET("/seeds", prober.getSeeds)
	router.PUT("/seeds", prober.putSeeds)
	router.GET("/dcs", prober.getDCs)
	router.PUT("/dcs", prober.putDCs)
}
