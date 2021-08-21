package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"net/http"
	"os"

	_ "github.com/joho/godotenv/autoload"
	"github.com/julienschmidt/httprouter"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	log        = ctrl.Log.WithName("prober-server")
	serverPort = os.Getenv("SERVER_PORT")
	seeds      []string
	readyLocalDCs bool
	Version       = "undefined"
	clientSet  *kubernetes.Clientset
	controller cache.Controller
	store      cache.Store
)

func setupRoutes(router *httprouter.Router) {
	router.GET("/healthz/:broadcastip", healthCheck)
	router.GET("/ping", ping)
	router.GET("/readylocaldcs", getReadyLocalDCs)
	router.PUT("/readylocaldcs", putReadyLocalDCs)
	router.GET("/localseeds", getSeeds)
	router.PUT("/localseeds", putSeeds)
}

func healthCheck(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	broadcastIp := ps.ByName("broadcastip")
	isReady, states := processReadinessProbe(r.RemoteAddr, broadcastIp)
	if isReady {
		w.WriteHeader(http.StatusOK)
	} else {
		log.V(1).Info("health check failed", "host", r.RemoteAddr, "broadcastIP", broadcastIp, "isReady", isReady)
		w.WriteHeader(http.StatusNotFound)
	}
	response, _ := json.Marshal(states)
	w.Write(response)
}

func ping(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	fmt.Fprint(w, "pong")
}

func getReadyLocalDCs(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	response, _ := json.Marshal(readyLocalDCs)
	w.Write(response)
}

func putReadyLocalDCs(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var ready bool
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	} else if json.Unmarshal(body, &ready) != nil {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		readyLocalDCs = ready
	}
}

func getSeeds(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	response, _ := json.Marshal(seeds)
	w.Write(response)
}

func putSeeds(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var s []string
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	} else if json.Unmarshal(body, &s) != nil {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		seeds = s
	}
}

func main() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)).V(1))
	router := httprouter.New()
	setupRoutes(router)

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Error(err, "Unable to get InCluster config")
	}

	clientSet, err = kubernetes.NewForConfig(config)

	authSecretCh := watchAuthSecret()
	defer close(authSecretCh)

	go pollNodeStates()

	log.WithValues("version", Version).Info("Cassandra's prober listening", "serverPort", serverPort)
	log.Error(http.ListenAndServe(fmt.Sprintf(":%s", serverPort), router), "http server error")
}
