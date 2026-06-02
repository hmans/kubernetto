package kube

import (
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
