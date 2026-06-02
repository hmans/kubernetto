package kube

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestResourceStoreOverviewSummarizesClusterHealth(t *testing.T) {
	now := time.Now()
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
		testNode("ready-node", true),
		testNode("offline-node", false),
		testPod("api", "default", 0),
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "starting", Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "starting"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "starting", Ready: false},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "job", Namespace: "prod"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "job"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodSucceeded,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "job", Ready: false},
				},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr(int32(2)),
			},
			Status: appsv1.DeploymentStatus{
				ReadyReplicas:   1,
				UpdatedReplicas: 1,
			},
		},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"}},
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "api-warning", Namespace: "default"},
			Type:           corev1.EventTypeWarning,
			Reason:         "BackOff",
			Message:        "back-off restarting failed container",
			Count:          3,
			LastTimestamp:  metav1.NewTime(now.Add(-2 * time.Minute)),
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "api", UID: types.UID("pod-1")},
		},
	)
	store := syncedTestStore(t, clientset)
	store.setPodUsage(map[string]corev1.ResourceList{
		podKey("default", "api"): {
			corev1.ResourceCPU:    resource.MustParse("250m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
	})

	overview := store.Overview()

	if overview.Error != "" {
		t.Fatalf("overview error = %q", overview.Error)
	}
	if got := overviewMetricValue(overview.Stats, "Nodes ready"); got != "1/2" {
		t.Fatalf("node health = %q", got)
	}
	if got := overviewMetricValue(overview.Stats, "Pods healthy"); got != "1/3" {
		t.Fatalf("pod health = %q", got)
	}
	if got := overviewMetricValue(overview.Stats, "Workloads ready"); got != "0/1" {
		t.Fatalf("workload health = %q", got)
	}
	if got := overviewMetricValue(overview.Resources, "Services"); got != "1" {
		t.Fatalf("service count = %q", got)
	}
	if got := overviewMetricValue(overview.Stats, "CPU"); got != "250m" {
		t.Fatalf("cpu usage = %q", got)
	}
	if got := overviewMetricValue(overview.Stats, "Memory"); got != "128Mi" {
		t.Fatalf("memory usage = %q", got)
	}
	if len(overview.WarningEvents) != 1 {
		t.Fatalf("warnings = %d, want 1", len(overview.WarningEvents))
	}
	if event := overview.WarningEvents[0]; event.Reason != "BackOff" || event.InvolvedObject != "Pod/api" || event.Count != 3 {
		t.Fatalf("warning event = %#v", event)
	}
}

func overviewMetricValue(metrics []OverviewMetric, label string) string {
	for _, metric := range metrics {
		if metric.Label == label {
			return metric.Value
		}
	}
	return ""
}

func testNode(name string, ready bool) *corev1.Node {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: status},
			},
		},
	}
}
