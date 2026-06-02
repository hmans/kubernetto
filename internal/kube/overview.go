package kube

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func (s *ResourceStore) Overview() ClusterOverview {
	overview := ClusterOverview{UpdatedAt: time.Now()}
	if s == nil || s.cluster == nil {
		overview.Error = "No Kubernetes client is configured."
		return overview
	}
	overview.Identity = detailFields(
		"Context", s.cluster.ContextName,
		"Cluster", s.cluster.ClusterName,
		"Config", s.cluster.ConfigSource,
		"Default namespace", s.cluster.Namespace,
		"Version", s.serverVersionValue(),
	)

	if err := s.readinessError(); err != nil {
		overview.Error = err.Error()
		return overview
	}

	nodes := s.listNodes()
	pods := s.listPods("")
	deployments := s.listDeployments("")
	statefulSets := s.listStatefulSets("")
	daemonSets := s.listDaemonSets("")
	warningEvents := s.recentWarningEvents(6)
	usage := s.totalPodUsage()
	overview.Metrics, overview.PodUsage = s.podUsageTimelinesForOverview(8)

	readyNodes := 0
	for _, node := range nodes {
		if nodeReady(*node) {
			readyNodes++
		}
	}

	healthyPods := 0
	pendingPods := 0
	failedPods := 0
	succeededPods := 0
	for _, pod := range pods {
		switch pod.Status.Phase {
		case corev1.PodPending:
			pendingPods++
		case corev1.PodFailed:
			failedPods++
		case corev1.PodSucceeded:
			succeededPods++
		}
		if podHealthy(*pod) {
			healthyPods++
		}
	}

	readyWorkloads, totalWorkloads := workloadHealth(deployments, statefulSets, daemonSets)
	overview.Stats = []OverviewMetric{
		{Label: "Nodes ready", Value: fmt.Sprintf("%d/%d", readyNodes, len(nodes)), Kind: KindNodes},
		{Label: "Pods healthy", Value: fmt.Sprintf("%d/%d", healthyPods, len(pods)), Detail: podPhaseDetail(pendingPods, failedPods, succeededPods), Kind: KindPods},
		{Label: "Workloads ready", Value: fmt.Sprintf("%d/%d", readyWorkloads, totalWorkloads), Kind: KindDeployments},
		{Label: "Warnings", Value: fmt.Sprint(len(warningEvents)), Detail: "Recent warning events", Kind: KindOverview},
	}
	overview.Stats = append(overview.Stats, usageMetrics(usage)...)
	overview.Resources = []OverviewMetric{
		{Label: "Pods", Value: fmt.Sprint(len(pods)), Kind: KindPods},
		{Label: "Deployments", Value: fmt.Sprint(len(deployments)), Kind: KindDeployments},
		{Label: "StatefulSets", Value: fmt.Sprint(len(statefulSets)), Kind: KindStatefulSet},
		{Label: "DaemonSets", Value: fmt.Sprint(len(daemonSets)), Kind: KindDaemonSet},
		{Label: "Services", Value: fmt.Sprint(len(s.listServices(""))), Kind: KindServices},
		{Label: "Ingresses", Value: fmt.Sprint(len(s.listIngresses(""))), Kind: KindIngresses},
		{Label: "Nodes", Value: fmt.Sprint(len(nodes)), Kind: KindNodes},
		{Label: "Namespaces", Value: fmt.Sprint(len(s.listNamespaces())), Kind: KindNamespaces},
	}
	overview.WarningEvents = warningEvents
	return overview
}

func podHealthy(pod corev1.Pod) bool {
	ready, total, _ := podReady(pod)
	return pod.DeletionTimestamp == nil && pod.Status.Phase == corev1.PodRunning && total > 0 && ready == total
}

func nodeReady(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func workloadHealth(deployments []*appsv1.Deployment, statefulSets []*appsv1.StatefulSet, daemonSets []*appsv1.DaemonSet) (int, int) {
	ready := 0
	total := len(deployments) + len(statefulSets) + len(daemonSets)
	for _, deployment := range deployments {
		desired := int32(0)
		if deployment.Spec.Replicas != nil {
			desired = *deployment.Spec.Replicas
		}
		if deployment.Status.ReadyReplicas == desired && deployment.Status.UpdatedReplicas == desired {
			ready++
		}
	}
	for _, statefulSet := range statefulSets {
		desired := int32(0)
		if statefulSet.Spec.Replicas != nil {
			desired = *statefulSet.Spec.Replicas
		}
		if statefulSet.Status.ReadyReplicas == desired {
			ready++
		}
	}
	for _, daemonSet := range daemonSets {
		if daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled {
			ready++
		}
	}
	return ready, total
}

func podPhaseDetail(pending, failed, succeeded int) string {
	parts := []string{}
	if pending > 0 {
		parts = append(parts, fmt.Sprintf("%d pending", pending))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if succeeded > 0 {
		parts = append(parts, fmt.Sprintf("%d succeeded", succeeded))
	}
	if len(parts) == 0 {
		return "No pending or failed pods"
	}
	return strings.Join(parts, ", ")
}

func usageMetrics(usage corev1.ResourceList) []OverviewMetric {
	if len(usage) == 0 {
		return []OverviewMetric{
			{Label: "CPU", Value: "Unavailable", Detail: "Metrics API has not reported pod usage", StatusKey: "neutral"},
			{Label: "Memory", Value: "Unavailable", Detail: "Metrics API has not reported pod usage", StatusKey: "neutral"},
		}
	}
	return []OverviewMetric{
		{Label: "CPU", Value: resourceListValue(usage, corev1.ResourceCPU), Detail: "Current pod usage", StatusKey: "neutral", Kind: KindPods},
		{Label: "Memory", Value: resourceListValue(usage, corev1.ResourceMemory), Detail: "Current pod usage", StatusKey: "neutral", Kind: KindPods},
	}
}

func (s *ResourceStore) podUsageTimelinesForOverview(limit int) (MetricsState, []PodUsageGraph) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if timelines, target, err := s.loadPrometheusTimelines(ctx); err == nil {
		return MetricsState{
			Available: true,
			Message:   "Prometheus is reporting pod usage.",
			Source:    "Prometheus " + target.Namespace + "/" + target.Service,
			Window:    "Last 60 minutes",
			UpdatedAt: time.Now(),
		}, s.topPodUsageFromTimelines(timelines, limit)
	}

	state := s.podMetricsState()
	if state.Source == "" && state.Available {
		state.Source = "metrics.k8s.io"
	}
	return state, s.topPodUsageFromTimelines(s.podUsageHistoryMap(), limit)
}

func (s *ResourceStore) topPodUsageFromTimelines(timelines map[string][]UsageSample, limit int) []PodUsageGraph {
	pods := s.listPods("")
	items := make([]podUsageValue, 0, len(pods))
	for _, pod := range pods {
		samples := timelines[podKey(pod.Namespace, pod.Name)]
		latest, ok := latestUsageSample(samples)
		if !ok {
			continue
		}
		cpu := latest.CPU
		memory := latest.Memory
		if cpu == 0 && memory == 0 {
			continue
		}
		items = append(items, podUsageValue{
			name:      pod.Name,
			namespace: pod.Namespace,
			cpu:       cpu,
			memory:    memory,
			cpuValue:  cpuMilliValue(cpu),
			memValue:  byteValue(memory),
			samples:   samples,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].cpu == items[j].cpu {
			if items[i].memory == items[j].memory {
				return strings.ToLower(items[i].name) < strings.ToLower(items[j].name)
			}
			return items[i].memory > items[j].memory
		}
		return items[i].cpu > items[j].cpu
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]PodUsageGraph, 0, len(items))
	for _, item := range items {
		out = append(out, PodUsageGraph{
			Name:      item.name,
			Namespace: item.namespace,
			CPU:       item.cpuValue,
			Memory:    item.memValue,
			Samples:   item.samples,
		})
	}
	return out
}

type podUsageValue struct {
	name      string
	namespace string
	cpu       int64
	memory    int64
	cpuValue  string
	memValue  string
	samples   []UsageSample
}

func latestUsageSample(samples []UsageSample) (UsageSample, bool) {
	for i := len(samples) - 1; i >= 0; i-- {
		if samples[i].CPU != 0 || samples[i].Memory != 0 {
			return samples[i], true
		}
	}
	return UsageSample{}, false
}

func cpuMilliValue(milli int64) string {
	if milli == 0 {
		return "-"
	}
	if milli%1000 == 0 {
		return fmt.Sprintf("%d", milli/1000)
	}
	return fmt.Sprintf("%dm", milli)
}

func (s *ResourceStore) totalPodUsage() corev1.ResourceList {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := corev1.ResourceList{}
	for _, usage := range s.podUsage {
		addResourceList(total, usage)
	}
	if len(total) == 0 {
		return nil
	}
	return total
}

func (s *ResourceStore) recentWarningEvents(limit int) []OverviewEvent {
	events := []OverviewEvent{}
	for _, event := range s.listEvents() {
		if strings.ToLower(event.Type) != "warning" {
			continue
		}
		lastSeen := eventTimestamp(*event)
		events = append(events, OverviewEvent{
			Type:           event.Type,
			Reason:         event.Reason,
			Message:        event.Message,
			Namespace:      event.Namespace,
			InvolvedObject: involvedObjectName(event.InvolvedObject),
			Count:          event.Count,
			Age:            age(lastSeen),
			LastSeen:       lastSeen,
		})
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].LastSeen.After(events[j].LastSeen)
	})
	if limit > 0 && len(events) > limit {
		return events[:limit]
	}
	return events
}

func involvedObjectName(ref corev1.ObjectReference) string {
	if ref.Kind == "" {
		return ref.Name
	}
	if ref.Name == "" {
		return ref.Kind
	}
	return ref.Kind + "/" + ref.Name
}
