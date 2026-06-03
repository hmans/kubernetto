package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
)

const (
	usageMetricsWindow = 60 * time.Minute
	prometheusStep     = 60 * time.Second
)

type prometheusTarget struct {
	Namespace string
	Service   string
	Port      string
	Scheme    string
	Score     int
}

type prometheusRangeResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		Result []prometheusSeries `json:"result"`
	} `json:"data"`
}

type prometheusSeries struct {
	Metric map[string]string  `json:"metric"`
	Values []prometheusSample `json:"values"`
}

type prometheusSample struct {
	Timestamp time.Time
	Value     float64
}

type PrometheusRangeQuery struct {
	Name  string `json:"name"`
	Query string `json:"query"`
}

type PrometheusRangeData struct {
	Available bool                    `json:"available"`
	Message   string                  `json:"message"`
	Source    string                  `json:"source,omitempty"`
	Window    string                  `json:"window"`
	UpdatedAt time.Time               `json:"updatedAt"`
	Series    []PrometheusRangeSeries `json:"series"`
}

type PrometheusRangeSeries struct {
	Name    string                  `json:"name"`
	Metric  map[string]string       `json:"metric"`
	Samples []PrometheusRangeSample `json:"samples"`
}

type PrometheusRangeSample struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

func (s *ResourceStore) PrometheusRangeData(ctx context.Context, queries []PrometheusRangeQuery, window, step time.Duration) (PrometheusRangeData, error) {
	out := PrometheusRangeData{
		Message:   "Prometheus range data is unavailable.",
		Window:    usageWindowLabel(window),
		UpdatedAt: time.Now(),
	}
	if len(queries) == 0 {
		return out, errors.New("at least one PromQL query is required")
	}
	targets := prometheusTargets(s.listServices(""))
	if len(targets) == 0 {
		return out, errors.New("no Prometheus-looking services found")
	}
	if window <= 0 {
		window = usageMetricsWindow
	}
	if step <= 0 {
		step = prometheusStep
	}

	end := time.Now()
	start := end.Add(-window)
	var lastErr error
	for _, target := range targets {
		series := []PrometheusRangeSeries{}
		failed := false
		for _, query := range queries {
			if strings.TrimSpace(query.Query) == "" {
				lastErr = errors.New("PromQL query cannot be empty")
				failed = true
				break
			}
			result, err := s.queryPrometheusRange(ctx, target, query.Query, start, end, step)
			if err != nil {
				lastErr = err
				failed = true
				break
			}
			series = append(series, prometheusRangeSeries(query.Name, result)...)
		}
		if failed {
			continue
		}
		out.Available = true
		out.Message = "Prometheus is reporting range data."
		out.Source = "Prometheus " + target.Namespace + "/" + target.Service
		out.Window = usageWindowLabel(window)
		out.UpdatedAt = time.Now()
		out.Series = series
		return out, nil
	}
	if lastErr != nil {
		return out, lastErr
	}
	return out, errors.New("Prometheus range queries returned no usable target")
}

func (s *ResourceStore) loadPrometheusTimelines(ctx context.Context, cpuQuery, memoryQuery string) (map[string][]UsageSample, prometheusTarget, error) {
	targets := prometheusTargets(s.listServices(""))
	if len(targets) == 0 {
		return nil, prometheusTarget{}, errors.New("no Prometheus-looking services found")
	}
	if cpuQuery == "" {
		cpuQuery = podCPUQuery
	}
	if memoryQuery == "" {
		memoryQuery = podMemoryQuery
	}

	end := time.Now()
	start := end.Add(-usageMetricsWindow)
	var lastErr error
	for _, target := range targets {
		cpuSeries, err := s.queryPrometheusRange(ctx, target, cpuQuery, start, end, prometheusStep)
		if err != nil {
			lastErr = err
			continue
		}
		memorySeries, err := s.queryPrometheusRange(ctx, target, memoryQuery, start, end, prometheusStep)
		if err != nil {
			lastErr = err
			continue
		}
		timelines := mergePrometheusSeries(cpuSeries, memorySeries)
		return timelines, target, nil
	}
	if lastErr != nil {
		return nil, prometheusTarget{}, lastErr
	}
	return nil, prometheusTarget{}, errors.New("Prometheus range queries returned no usable target")
}

func (s *ResourceStore) loadPrometheusPodTimeline(ctx context.Context, namespace, name string) ([]UsageSample, prometheusTarget, error) {
	timelines, target, err := s.loadPrometheusTimelinesForQueries(ctx, podCPUQueryFor(namespace, name), podMemoryQueryFor(namespace, name))
	if err != nil {
		return nil, prometheusTarget{}, err
	}
	return timelines[podKey(namespace, name)], target, nil
}

// PodUsageDetailChart loads chart data on demand. Resource detail rendering
// deliberately does not call this so historical metrics stay lazy.
func (s *ResourceStore) PodUsageDetailChart(namespace, name, cpuQuery, memoryQuery string) PodUsageDetailChart {
	chart := PodUsageDetailChart{Name: name, Namespace: namespace}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	timelines, target, err := s.loadPrometheusTimelinesForQueries(ctx, normalizedPodCPUQuery(namespace, name, cpuQuery), normalizedPodMemoryQuery(namespace, name, memoryQuery))
	timeline := timelines[podKey(namespace, name)]
	if err == nil && len(timeline) > 0 {
		chart.Metrics = MetricsState{
			Available: true,
			Message:   "Prometheus is reporting pod usage.",
			Source:    "Prometheus " + target.Namespace + "/" + target.Service,
			Window:    "Last 60 minutes",
			UpdatedAt: time.Now(),
		}
	} else {
		timeline = s.podUsageTimelineFor(namespace, name)
		chart.Metrics = s.podMetricsState()
		if chart.Metrics.Source == "" && chart.Metrics.Available {
			chart.Metrics.Source = "metrics.k8s.io"
		}
	}

	latest, ok := latestUsageSample(timeline)
	if !ok {
		usage := s.podUsageFor(namespace, name)
		if len(usage) == 0 {
			return chart
		}
		latest = UsageSample{Timestamp: time.Now()}
		if cpu := usage[corev1.ResourceCPU]; !cpu.IsZero() {
			latest.CPU = cpu.MilliValue()
		}
		if memory := usage[corev1.ResourceMemory]; !memory.IsZero() {
			latest.Memory = memory.Value()
		}
		timeline = []UsageSample{latest}
	}
	if latest.CPU == 0 && latest.Memory == 0 {
		return chart
	}
	chart.Usage = usageGraphs(
		UsageGraph{Label: "CPU", Value: cpuMilliValue(latest.CPU)},
		UsageGraph{Label: "Memory", Value: byteValue(latest.Memory)},
	)
	chart.Samples = timeline
	return chart
}

func (s *ResourceStore) loadPrometheusTimelinesForQueries(ctx context.Context, cpuQuery, memoryQuery string) (map[string][]UsageSample, prometheusTarget, error) {
	targets := prometheusTargets(s.listServices(""))
	if len(targets) == 0 {
		return nil, prometheusTarget{}, errors.New("no Prometheus-looking services found")
	}

	end := time.Now()
	start := end.Add(-usageMetricsWindow)
	var lastErr error
	for _, target := range targets {
		cpuSeries, err := s.queryPrometheusRange(ctx, target, cpuQuery, start, end, prometheusStep)
		if err != nil {
			lastErr = err
			continue
		}
		memorySeries, err := s.queryPrometheusRange(ctx, target, memoryQuery, start, end, prometheusStep)
		if err != nil {
			lastErr = err
			continue
		}
		return mergePrometheusSeries(cpuSeries, memorySeries), target, nil
	}
	if lastErr != nil {
		return nil, prometheusTarget{}, lastErr
	}
	return nil, prometheusTarget{}, errors.New("Prometheus range queries returned no usable target")
}

const podCPUQuery = `sum by (namespace, pod) (rate(container_cpu_usage_seconds_total{pod!="",container!="",image!=""}[5m]))`
const podMemoryQuery = `sum by (namespace, pod) (container_memory_working_set_bytes{pod!="",container!="",image!=""})`

func PodCPUQuery() string {
	return podCPUQuery
}

func PodMemoryQuery() string {
	return podMemoryQuery
}

func PodCPUQueryFor(namespace, pod string) string {
	return podCPUQueryFor(namespace, pod)
}

func PodMemoryQueryFor(namespace, pod string) string {
	return podMemoryQueryFor(namespace, pod)
}

func podCPUQueryFor(namespace, pod string) string {
	return `sum by (namespace, pod) (rate(container_cpu_usage_seconds_total{namespace=` + strconv.Quote(namespace) + `,pod=` + strconv.Quote(pod) + `,container!="",image!=""}[5m]))`
}

func podMemoryQueryFor(namespace, pod string) string {
	return `sum by (namespace, pod) (container_memory_working_set_bytes{namespace=` + strconv.Quote(namespace) + `,pod=` + strconv.Quote(pod) + `,container!="",image!=""})`
}

func prometheusRangeSeries(name string, series []prometheusSeries) []PrometheusRangeSeries {
	out := make([]PrometheusRangeSeries, 0, len(series))
	for _, item := range series {
		samples := make([]PrometheusRangeSample, 0, len(item.Values))
		for _, sample := range item.Values {
			samples = append(samples, PrometheusRangeSample{
				Timestamp: sample.Timestamp,
				Value:     sample.Value,
			})
		}
		out = append(out, PrometheusRangeSeries{
			Name:    name,
			Metric:  item.Metric,
			Samples: samples,
		})
	}
	return out
}

func usageWindowLabel(window time.Duration) string {
	if window <= 0 {
		window = usageMetricsWindow
	}
	if window%time.Hour == 0 {
		hours := int(window / time.Hour)
		if hours == 1 {
			return "Last 1 hour"
		}
		return fmt.Sprintf("Last %d hours", hours)
	}
	if window%time.Minute == 0 {
		minutes := int(window / time.Minute)
		if minutes == 1 {
			return "Last 1 minute"
		}
		return fmt.Sprintf("Last %d minutes", minutes)
	}
	return "Last " + window.Round(time.Second).String()
}

func normalizedPodCPUQuery(namespace, pod, query string) string {
	if query != "" {
		return query
	}
	return podCPUQueryFor(namespace, pod)
}

func normalizedPodMemoryQuery(namespace, pod, query string) string {
	if query != "" {
		return query
	}
	return podMemoryQueryFor(namespace, pod)
}

func prometheusTargets(services []*corev1.Service) []prometheusTarget {
	targets := []prometheusTarget{}
	for _, service := range services {
		for _, port := range service.Spec.Ports {
			score := prometheusScore(*service, port)
			if score < 50 {
				continue
			}
			portValue := port.Name
			if portValue == "" {
				portValue = strconv.Itoa(int(port.Port))
			}
			scheme := "http"
			if strings.Contains(strings.ToLower(port.Name), "https") {
				scheme = "https"
			}
			targets = append(targets, prometheusTarget{
				Namespace: service.Namespace,
				Service:   service.Name,
				Port:      portValue,
				Scheme:    scheme,
				Score:     score,
			})
		}
	}
	sort.SliceStable(targets, func(i, j int) bool {
		if targets[i].Score == targets[j].Score {
			return targets[i].Namespace+"/"+targets[i].Service < targets[j].Namespace+"/"+targets[j].Service
		}
		return targets[i].Score > targets[j].Score
	})
	return targets
}

func prometheusScore(service corev1.Service, port corev1.ServicePort) int {
	score := 0
	name := strings.ToLower(service.Name)
	namespace := strings.ToLower(service.Namespace)
	portName := strings.ToLower(port.Name)
	if strings.Contains(name, "prometheus") {
		score += 60
	}
	if strings.Contains(namespace, "monitor") || strings.Contains(namespace, "observability") {
		score += 12
	}
	if port.Port == 9090 {
		score += 25
	}
	if strings.Contains(portName, "web") || strings.Contains(portName, "http") || strings.Contains(portName, "prometheus") {
		score += 15
	}
	for key, value := range service.Labels {
		label := strings.ToLower(key + "=" + value)
		if strings.Contains(label, "prometheus") {
			score += 40
		}
		if strings.Contains(label, "monitoring") {
			score += 10
		}
	}
	return score
}

func (s *ResourceStore) queryPrometheusRange(ctx context.Context, target prometheusTarget, query string, start, end time.Time, step time.Duration) ([]prometheusSeries, error) {
	raw, err := s.cluster.Clientset.CoreV1().RESTClient().
		Get().
		Namespace(target.Namespace).
		Resource("services").
		Name(target.proxyName()).
		SubResource("proxy").
		Suffix("api/v1/query_range").
		Param("query", query).
		Param("start", strconv.FormatInt(start.Unix(), 10)).
		Param("end", strconv.FormatInt(end.Unix(), 10)).
		Param("step", strconv.Itoa(int(step.Seconds()))+"s").
		Do(ctx).
		Raw()
	if err != nil {
		return nil, fmt.Errorf("query %s/%s: %w", target.Namespace, target.Service, err)
	}

	var response prometheusRangeResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("decode Prometheus response from %s/%s: %w", target.Namespace, target.Service, err)
	}
	if response.Status != "success" {
		if response.Error != "" {
			return nil, errors.New(response.Error)
		}
		return nil, errors.New("Prometheus query failed")
	}
	return response.Data.Result, nil
}

func (t prometheusTarget) proxyName() string {
	if t.Scheme != "" && t.Port != "" {
		return t.Scheme + ":" + t.Service + ":" + t.Port
	}
	if t.Port != "" {
		return t.Service + ":" + t.Port
	}
	return t.Service
}

func mergePrometheusSeries(cpuSeries, memorySeries []prometheusSeries) map[string][]UsageSample {
	timelines := map[string][]UsageSample{}
	for _, series := range cpuSeries {
		namespace, pod := seriesIdentity(series)
		if namespace == "" || pod == "" {
			continue
		}
		key := podKey(namespace, pod)
		samples := timelines[key]
		index := sampleIndex(samples)
		for _, sample := range series.Values {
			i, ok := index[sample.Timestamp.Unix()]
			if !ok {
				samples = append(samples, UsageSample{Timestamp: sample.Timestamp})
				i = len(samples) - 1
				index[sample.Timestamp.Unix()] = i
			}
			samples[i].CPU = int64(math.Round(sample.Value * 1000))
		}
		timelines[key] = samples
	}
	for _, series := range memorySeries {
		namespace, pod := seriesIdentity(series)
		if namespace == "" || pod == "" {
			continue
		}
		key := podKey(namespace, pod)
		samples := timelines[key]
		index := sampleIndex(samples)
		for _, sample := range series.Values {
			i, ok := index[sample.Timestamp.Unix()]
			if !ok {
				samples = append(samples, UsageSample{Timestamp: sample.Timestamp})
				i = len(samples) - 1
				index[sample.Timestamp.Unix()] = i
			}
			samples[i].Memory = int64(math.Round(sample.Value))
		}
		sort.Slice(samples, func(i, j int) bool {
			return samples[i].Timestamp.Before(samples[j].Timestamp)
		})
		timelines[key] = samples
	}
	for key, samples := range timelines {
		sort.Slice(samples, func(i, j int) bool {
			return samples[i].Timestamp.Before(samples[j].Timestamp)
		})
		timelines[key] = samples
	}
	return timelines
}

func seriesIdentity(series prometheusSeries) (string, string) {
	return series.Metric["namespace"], series.Metric["pod"]
}

func sampleIndex(samples []UsageSample) map[int64]int {
	index := make(map[int64]int, len(samples))
	for i, sample := range samples {
		index[sample.Timestamp.Unix()] = i
	}
	return index
}

func (s *ResourceStore) podUsageHistoryMap() map[string][]UsageSample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	source := s.podUsageHistory
	out := make(map[string][]UsageSample, len(source))
	cutoff := time.Now().Add(-usageMetricsWindow)
	for key, samples := range source {
		trimmed := trimUsageSamples(samples, cutoff)
		if len(trimmed) == 0 {
			continue
		}
		out[key] = append([]UsageSample(nil), trimmed...)
	}
	return out
}

func (s *ResourceStore) podUsageTimelineFor(namespace, name string) []UsageSample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := podKey(namespace, name)
	samples := s.podUsageHistory[key]
	return append([]UsageSample(nil), trimUsageSamples(samples, time.Now().Add(-usageMetricsWindow))...)
}

func (s *ResourceStore) prometheusTargetCount() int {
	return len(prometheusTargets(s.listServices("")))
}

func (sample *prometheusSample) UnmarshalJSON(data []byte) error {
	values := []json.RawMessage{}
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	if len(values) != 2 {
		return fmt.Errorf("expected Prometheus sample pair, got %d values", len(values))
	}
	var timestamp float64
	if err := json.Unmarshal(values[0], &timestamp); err != nil {
		return err
	}
	var value string
	if err := json.Unmarshal(values[1], &value); err != nil {
		return err
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return err
	}
	sample.Timestamp = time.Unix(int64(timestamp), int64((timestamp-math.Trunc(timestamp))*1e9))
	sample.Value = parsed
	return nil
}
