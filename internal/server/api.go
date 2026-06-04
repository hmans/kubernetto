package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"kubernetto/internal/kube"
)

const apiMaxJSONBodyBytes = 64 * 1024

func (s *Server) apiRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/prometheus/", http.StripPrefix("/prometheus", s.prometheusAPIRoutes()))
	return mux
}

type preparedPrometheusRangeHandler[T any] struct {
	server        *Server
	validate      func(*T) error
	contextName   func(T) string
	windowSeconds func(T) int
	stepSeconds   func(T) int
	load          func(context.Context, *kube.ResourceStore, T, time.Duration, time.Duration) (kube.PrometheusRangeData, error)
}

func (h preparedPrometheusRangeHandler[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var request T
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, apiMaxJSONBodyBytes)).Decode(&request); err != nil {
		writeCompressedJSON(w, r, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body"})
		return
	}
	if h.validate != nil {
		if err := h.validate(&request); err != nil {
			writeCompressedJSON(w, r, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}

	session := h.server.session(h.contextName(request))
	if session == nil || session.store == nil {
		writeCompressedJSON(w, r, http.StatusServiceUnavailable, kube.PrometheusRangeData{
			Message:   "No Kubernetes client is configured.",
			Window:    "Last 60 minutes",
			UpdatedAt: time.Now(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	data, err := h.load(ctx, session.store, request, chartWindow(h.windowSeconds(request)), chartStep(h.stepSeconds(request)))
	if err != nil {
		data.Message = err.Error()
		writeCompressedJSON(w, r, http.StatusOK, data)
		return
	}
	writeCompressedJSON(w, r, http.StatusOK, data)
}
