package kube

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPrometheusTargetsPreferLikelyServices(t *testing.T) {
	services := []*corev1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{
				{Name: "http", Port: 8080},
			}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "metrics",
				Namespace: "observability",
				Labels:    map[string]string{"app.kubernetes.io/name": "prometheus"},
			},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{
				{Name: "web", Port: 9090},
			}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "prometheus-kube-state-metrics", Namespace: "monitoring"},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{
				{Name: "http", Port: 8080},
			}},
		},
	}

	targets := prometheusTargets(services)

	if len(targets) != 2 {
		t.Fatalf("targets = %d, want 2", len(targets))
	}
	if got := targets[0]; got.Namespace != "observability" || got.Service != "metrics" || got.Port != "web" {
		t.Fatalf("first target = %#v, want observability/metrics:web", got)
	}
	if got := targets[0].proxyName(); got != "http:metrics:web" {
		t.Fatalf("proxy name = %q", got)
	}
}

func TestMergePrometheusSeriesCombinesAndSortsSamples(t *testing.T) {
	base := time.Unix(1_800_000_000, 0)
	cpuSeries := []prometheusSeries{
		{
			Metric: map[string]string{"namespace": "prod", "pod": "api"},
			Values: []prometheusSample{
				{Timestamp: base.Add(2 * time.Minute), Value: 0.75},
				{Timestamp: base, Value: 0.25},
			},
		},
	}
	memorySeries := []prometheusSeries{
		{
			Metric: map[string]string{"namespace": "prod", "pod": "api"},
			Values: []prometheusSample{
				{Timestamp: base.Add(time.Minute), Value: 32 * 1024 * 1024},
				{Timestamp: base, Value: 16 * 1024 * 1024},
			},
		},
	}

	timelines := mergePrometheusSeries(cpuSeries, memorySeries)
	samples := timelines[podKey("prod", "api")]

	if len(samples) != 3 {
		t.Fatalf("samples = %d, want 3: %#v", len(samples), samples)
	}
	if samples[0].Timestamp != base || samples[0].CPU != 250 || samples[0].Memory != 16*1024*1024 {
		t.Fatalf("first sample = %#v", samples[0])
	}
	if samples[1].Timestamp != base.Add(time.Minute) || samples[1].CPU != 0 || samples[1].Memory != 32*1024*1024 {
		t.Fatalf("second sample = %#v", samples[1])
	}
	if samples[2].Timestamp != base.Add(2*time.Minute) || samples[2].CPU != 750 || samples[2].Memory != 0 {
		t.Fatalf("third sample = %#v", samples[2])
	}
}

func TestPrometheusSampleParsesFractionalTimestamp(t *testing.T) {
	var sample prometheusSample
	if err := json.Unmarshal([]byte(`[1800000000.5,"1.25"]`), &sample); err != nil {
		t.Fatalf("unmarshal sample: %v", err)
	}

	if !sample.Timestamp.Equal(time.Unix(1_800_000_000, 500_000_000)) {
		t.Fatalf("timestamp = %s", sample.Timestamp)
	}
	if sample.Value != 1.25 {
		t.Fatalf("value = %f", sample.Value)
	}
}

func TestPodUsageRangeQueriesBuildPreparedPromQL(t *testing.T) {
	queries := podUsageRangeQueries([]PodUsageQueryPod{
		{Namespace: "prod", Pod: "api"},
		{Namespace: "prod", Pod: "worker.1"},
		{Namespace: "prod", Pod: "api"},
		{Namespace: "qa", Pod: `job"quoted`},
	})

	if len(queries) != 2 {
		t.Fatalf("queries = %d, want 2: %#v", len(queries), queries)
	}
	if queries[0].Name != "cpu" || queries[1].Name != "memory" {
		t.Fatalf("query names = %#v", queries)
	}
	for _, want := range []string{
		`container_cpu_usage_seconds_total{namespace="prod",pod=~"api|worker\\.1",container!="",image!=""}`,
		`container_cpu_usage_seconds_total{namespace="qa",pod=~"job\"quoted",container!="",image!=""}`,
		`container_memory_working_set_bytes{namespace="prod",pod=~"api|worker\\.1",container!="",image!=""}`,
		`container_memory_working_set_bytes{namespace="qa",pod=~"job\"quoted",container!="",image!=""}`,
	} {
		found := queries[0].Query + "\n" + queries[1].Query
		if !strings.Contains(found, want) {
			t.Fatalf("prepared queries did not contain %q:\n%s", want, found)
		}
	}
}

func TestReadLimitedPrometheusResponseRejectsOversizedBody(t *testing.T) {
	_, err := readLimitedPrometheusResponse(strings.NewReader(strings.Repeat("a", prometheusMaxResponseBytes+1)))
	if err == nil {
		t.Fatalf("expected oversized response error")
	}
}

func TestPodUsageHistoryReadsTrimStaleSamples(t *testing.T) {
	now := time.Now()
	key := podKey("prod", "api")
	staleKey := podKey("prod", "old")
	store := &ResourceStore{
		podUsageHistory: map[string][]UsageSample{
			key: {
				{Timestamp: now.Add(-2 * usageMetricsWindow), CPU: 100},
				{Timestamp: now.Add(-10 * time.Minute), CPU: 250, Memory: 64 * 1024 * 1024},
			},
			staleKey: {
				{Timestamp: now.Add(-2 * usageMetricsWindow), CPU: 50},
			},
		},
	}

	history := store.podUsageHistoryMap()
	if _, ok := history[staleKey]; ok {
		t.Fatalf("stale-only history was returned: %#v", history[staleKey])
	}
	if len(history[key]) != 1 || history[key][0].CPU != 250 {
		t.Fatalf("history = %#v, want only fresh sample", history[key])
	}

	timeline := store.podUsageTimelineFor("prod", "api")
	if len(timeline) != 1 || timeline[0].Memory != 64*1024*1024 {
		t.Fatalf("timeline = %#v, want only fresh sample", timeline)
	}
}
