package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	_ "github.com/joho/godotenv/autoload"
	"github.com/julienschmidt/httprouter"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	log = ctrl.Log.WithName("prober-server")
	serverPort                   = os.Getenv("SERVER_PORT")
	//localDcIngressDomain         = os.Getenv("LOCAL_DC_INGRESS_DOMAIN")
	//proberSubdomain              = os.Getenv("PROBER_SUBDOMAIN")
	//cassandraLocalSeedsHostnames = splitHostnames(strings.Split(os.Getenv("CASSANDRA_LOCAL_SEEDS_HOSTNAMES"), ","))
	//externalDcsIngressDomains    = os.Getenv("EXTERNAL_DCS_INGRESS_DOMAINS")
	//allDcsIngressDomains         = os.Getenv("ALL_DCS_INGRESS_DOMAINS")
	//localDcs                     = os.Getenv("LOCAL_DCS")
)

func setupRoutes(router *httprouter.Router) {
	router.GET("/healthz/:broadcastip", healthCheck)
	router.GET("/ping", ping)
	//router.GET("/readydc/:dc?", readyDc)
	//router.GET("/readyalldcs", readyAllDcs)
	//router.GET("/startdcinit/:dc", readyAllDcs)
	//router.GET("/localseeds", localSeeds)
	//router.GET("/seeds", seeds)
}

//func splitHostnames(hostnames []string) []string {
//	firstNames := make([]string, 0)
//	for _, name := range hostnames {
//		firstNames = append(firstNames, strings.Split(name, ".")[0])
//	}
//	return firstNames
//}

func healthCheck(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	broadcastIp := ps.ByName("broadcastip")
	log.V(1).Info("health check called", "host", r.RemoteAddr, "broadcastIP", broadcastIp)
	isReady, states := processReadinessProbe(r.RemoteAddr, broadcastIp)
	if isReady {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
	response, _ := json.Marshal(states)
	w.Write(response)
}

func ping(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	fmt.Fprint(w, "pong")
}

//func readyDc(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
//
//}
//
//func readyAllDcs(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
//
//}
//
//func localSeeds(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
//
//}
//
//func seeds(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
//
//}

func main() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)).V(1))
	router := httprouter.New()
	setupRoutes(router)
	go pollNodeStates()

	log.Info("Cassandra's prober listening", "serverPort", serverPort)
	log.Error(http.ListenAndServe(fmt.Sprintf(":%s", serverPort), router), "http server error")
}
