package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"kubernetto/internal/kube"
)

type prometheusAPI struct {
	server *Server
}

type prometheusPodUsageRangeRequest struct {
	Context       string                  `json:"context"`
	Pods          []kube.PodUsageQueryPod `json:"pods"`
	WindowSeconds int                     `json:"windowSeconds"`
	StepSeconds   int                     `json:"stepSeconds"`
}

const prometheusPodUsageRangeMaxPods = 8

func (s *Server) prometheusAPIRoutes() http.Handler {
	api := prometheusAPI{server: s}
	mux := http.NewServeMux()
	mux.Handle("POST /pod-usage-range", api.podUsageRangeHandler())
	return mux
}

func (api prometheusAPI) podUsageRangeHandler() http.Handler {
	return preparedPrometheusRangeHandler[prometheusPodUsageRangeRequest]{
		server: api.server,
		validate: func(request *prometheusPodUsageRangeRequest) error {
			return validatePrometheusPodUsageRangeRequest(request)
		},
		contextName: func(request prometheusPodUsageRangeRequest) string {
			return request.Context
		},
		windowSeconds: func(request prometheusPodUsageRangeRequest) int {
			return request.WindowSeconds
		},
		stepSeconds: func(request prometheusPodUsageRangeRequest) int {
			return request.StepSeconds
		},
		load: func(ctx context.Context, store *kube.ResourceStore, request prometheusPodUsageRangeRequest, window, step time.Duration) (kube.PrometheusRangeData, error) {
			return store.PrometheusPodUsageRangeData(ctx, request.Pods, window, step)
		},
	}
}

func validatePrometheusPodUsageRangeRequest(request *prometheusPodUsageRangeRequest) error {
	if len(request.Pods) == 0 {
		return errors.New("at least one pod is required")
	}
	if len(request.Pods) > prometheusPodUsageRangeMaxPods {
		return errors.New("at most eight pod timelines can be loaded at once")
	}
	for index, pod := range request.Pods {
		request.Pods[index].Namespace = strings.TrimSpace(pod.Namespace)
		request.Pods[index].Pod = strings.TrimSpace(pod.Pod)
		if request.Pods[index].Namespace == "" || request.Pods[index].Pod == "" {
			return errors.New("pod namespace and name are required")
		}
	}
	return nil
}
