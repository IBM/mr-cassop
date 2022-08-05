package prober

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/julienschmidt/httprouter"
)

func (p *Prober) healthCheck(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	broadcastIP := ps.ByName("broadcastip")
	podIP := strings.Split(r.RemoteAddr, ":")[0]

	isReady, states := p.processReadinessProbe(podIP, broadcastIP)
	if isReady {
		w.WriteHeader(http.StatusOK)
	} else {
		p.log.Infow("health check failed", "podIP", podIP, "broadcastIP", broadcastIP, "isReady", isReady)
		w.WriteHeader(http.StatusNotFound)
	}
	response, _ := json.Marshal(states)
	p.write(w, response)
}

func (p *Prober) ping(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	p.write(w, []byte("pong"))
}

func (p *Prober) getRegionReady(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	p.write(w, []byte(strconv.FormatBool(p.state.regionReady)))
}

func (p *Prober) putRegionReady(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var ready bool
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		p.log.Error(err, "can't ready body")
		w.WriteHeader(http.StatusInternalServerError)
	} else if ready, err = strconv.ParseBool(string(body)); err != nil {
		p.log.Error(err, "can't parse region readiness state")
		w.WriteHeader(http.StatusBadRequest)
	} else {
		p.state.regionReady = ready
	}
}

func (p *Prober) getReaperReady(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	p.write(w, []byte(strconv.FormatBool(p.state.reaperReady)))
}

func (p *Prober) putReaperReady(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var ready bool
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		p.log.Error(err, "can't ready body")
		w.WriteHeader(http.StatusInternalServerError)
	} else if ready, err = strconv.ParseBool(string(body)); err != nil {
		p.log.Error(err, "can't parse reaper readiness state")
		w.WriteHeader(http.StatusBadRequest)
	} else {
		p.state.reaperReady = ready
	}
}

func (p *Prober) getSeeds(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	response, _ := json.Marshal(p.state.seeds)
	p.write(w, response)
}

func (p *Prober) putSeeds(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var s []string
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	} else if json.Unmarshal(body, &s) != nil {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		p.state.seeds = s
	}
}

func (p *Prober) getDCs(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	response, _ := json.Marshal(p.state.dcs)
	p.write(w, response)
}

func (p *Prober) putDCs(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var dcs []dc
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	} else if json.Unmarshal(body, &dcs) != nil {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		p.state.dcs = dcs
	}
}

func (p *Prober) write(writer io.Writer, data []byte) {
	written, err := writer.Write(data)
	if err != nil {
		p.log.Error("Error writing data: %s, written %d bytes", err.Error(), written)
	}
}

func (p *Prober) getRegionIPs(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	response, _ := json.Marshal(p.state.regionIPs)
	p.write(w, response)
}

func (p *Prober) putRegionIPs(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var ips []string
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		p.log.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
	} else if json.Unmarshal(body, &ips) != nil {
		p.log.Error(err)
		w.WriteHeader(http.StatusBadRequest)
	} else {
		p.state.regionIPs = ips
	}
}
