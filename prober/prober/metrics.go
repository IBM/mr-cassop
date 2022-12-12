package prober

import (
	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"net/http"
)

var (
	totalRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of requests",
	}, []string{"path", "status"})
	responseTimeMillis = promauto.NewSummaryVec(prometheus.SummaryOpts{
		Name: "http_response_time_milliseconds",
		Help: "Duration of HTTP requests in milliseconds",
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	}, []string{"path"})
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func NewResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{w, http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func prometheusMiddleware(next httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		path := r.URL.Path
		timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
			millis := v * 1000 // make milliseconds
			responseTimeMillis.WithLabelValues(path).Observe(millis)
		}))
		defer timer.ObserveDuration()
		rw := NewResponseWriter(w)
		next(rw, r, ps)
		if rw.statusCode >= 100 && rw.statusCode < 300 {
			totalRequests.With(prometheus.Labels{"path": path, "status": "success"}).Inc()
		} else {
			totalRequests.With(prometheus.Labels{"path": path, "status": "fail"}).Inc()
		}
	}
}
