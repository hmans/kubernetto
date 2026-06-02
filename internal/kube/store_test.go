package kube

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

func TestResourceStoreReadsFromInformerCache(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api",
				Namespace: "prod",
				Labels:    map[string]string{"app": "web"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "api",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:              resource.MustParse("250m"),
								corev1.ResourceMemory:           resource.MustParse("512Mi"),
								corev1.ResourceEphemeralStorage: resource.MustParse("1Gi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
						},
					},
				},
				NodeName: "node-1",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "api", Ready: true},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "worker"}},
				NodeName:   "node-1",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "worker", Ready: true},
				},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr(int32(2)),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "web"},
				},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "web",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("128Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("500m"),
										corev1.ResourceMemory: resource.MustParse("256Mi"),
									},
								},
							},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{
				ReadyReplicas:     2,
				UpdatedReplicas:   2,
				AvailableReplicas: 2,
			},
		},
	)

	store := syncedTestStore(t, clientset)
	metricsClient := metricsfake.NewSimpleClientset()
	metricsClient.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &metricsv1beta1.PodMetricsList{
			Items: []metricsv1beta1.PodMetrics{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
					Containers: []metricsv1beta1.ContainerMetrics{
						{
							Name: "api",
							Usage: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("37m"),
								corev1.ResourceMemory: resource.MustParse("214Mi"),
							},
						},
					},
				},
			},
		}, nil
	})
	if err := store.loadPodMetrics(context.Background(), metricsClient); err != nil {
		t.Fatalf("load pod metrics: %v", err)
	}

	summary := store.Summary()
	if summary.Error != "" {
		t.Fatalf("summary error = %q", summary.Error)
	}
	if summary.Nodes != 1 || summary.Namespaces != 2 || summary.Pods != 2 || summary.Deployments != 1 {
		t.Fatalf("unexpected summary counts: %#v", summary)
	}

	namespaces, err := store.Namespaces()
	if err != nil {
		t.Fatalf("namespaces: %v", err)
	}
	if want := []string{"default", "prod"}; !equalStrings(namespaces, want) {
		t.Fatalf("namespaces = %#v, want %#v", namespaces, want)
	}

	table := store.Table(KindPods, "prod", "")
	if table.Error != "" {
		t.Fatalf("table error = %q", table.Error)
	}
	if len(table.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(table.Rows))
	}
	if table.Rows[0].Name != "api" || table.Rows[1].Name != "worker" {
		t.Fatalf("rows not sorted by name: %#v", table.Rows)
	}

	detail := store.Detail(KindPods, "prod", "api")
	if detail.Error != "" {
		t.Fatalf("detail error = %q", detail.Error)
	}
	if detail.Name != "api" || detail.Namespace != "prod" || detail.Status != string(corev1.PodRunning) {
		t.Fatalf("unexpected pod detail: %#v", detail)
	}
	if !strings.Contains(detail.YAML, "apiVersion: v1") || !strings.Contains(detail.YAML, "kind: Pod") {
		t.Fatalf("detail yaml missing Kubernetes identity: %q", detail.YAML)
	}

	if want := []string{"Name", "Namespace", "Ready", "Status", "Restarts", "CPU", "CPU Req", "CPU Limit", "MEM", "MEM Req", "MEM Limit", "Node", "Age"}; !equalStrings(table.Columns, want) {
		t.Fatalf("pod columns = %#v, want %#v", table.Columns, want)
	}
	if got, want := table.Rows[0].Cells[5].Value, "37m"; got != want {
		t.Fatalf("pod cpu = %q, want %q", got, want)
	}
	if got, want := table.Rows[0].Cells[6].Value, "250m"; got != want {
		t.Fatalf("pod cpu request = %q, want %q", got, want)
	}
	if got, want := table.Rows[0].Cells[7].Value, "1"; got != want {
		t.Fatalf("pod cpu limit = %q, want %q", got, want)
	}
	if got, want := table.Rows[0].Cells[8].Value, "214Mi"; got != want {
		t.Fatalf("pod mem = %q, want %q", got, want)
	}
	if got, want := table.Rows[0].Cells[9].Value, "512Mi"; got != want {
		t.Fatalf("pod mem request = %q, want %q", got, want)
	}
	if got, want := table.Rows[0].Cells[10].Value, "1Gi"; got != want {
		t.Fatalf("pod mem limit = %q, want %q", got, want)
	}

	deployments := store.Table(KindDeployments, "prod", "")
	if deployments.Error != "" {
		t.Fatalf("deployments error = %q", deployments.Error)
	}
	if want := []string{"Name", "Namespace", "Ready", "Up-to-date", "Available", "CPU", "CPU Req", "CPU Limit", "MEM", "MEM Req", "MEM Limit", "Age"}; !equalStrings(deployments.Columns, want) {
		t.Fatalf("deployment columns = %#v, want %#v", deployments.Columns, want)
	}
	if got, want := deployments.Rows[0].Cells[5].Value, "37m"; got != want {
		t.Fatalf("deployment cpu = %q, want %q", got, want)
	}
	if got, want := deployments.Rows[0].Cells[6].Value, "100m"; got != want {
		t.Fatalf("deployment cpu request = %q, want %q", got, want)
	}
	if got, want := deployments.Rows[0].Cells[7].Value, "500m"; got != want {
		t.Fatalf("deployment cpu limit = %q, want %q", got, want)
	}
	if got, want := deployments.Rows[0].Cells[8].Value, "214Mi"; got != want {
		t.Fatalf("deployment mem = %q, want %q", got, want)
	}
	if got, want := deployments.Rows[0].Cells[9].Value, "128Mi"; got != want {
		t.Fatalf("deployment mem request = %q, want %q", got, want)
	}
	if got, want := deployments.Rows[0].Cells[10].Value, "256Mi"; got != want {
		t.Fatalf("deployment mem limit = %q, want %q", got, want)
	}
}

func TestResourceStoreReadsDoNotCallKubeAPI(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "api"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "api", Ready: true},
				},
			},
		},
	)

	store := syncedTestStore(t, clientset)
	actionsAfterSync := len(clientset.Actions())

	for range 5 {
		_ = store.Summary()
		if _, err := store.Namespaces(); err != nil {
			t.Fatalf("namespaces: %v", err)
		}
		table := store.Table(KindPods, "", "")
		if table.Error != "" {
			t.Fatalf("table error = %q", table.Error)
		}
		detail := store.Detail(KindPods, "default", "api")
		if detail.Error != "" {
			t.Fatalf("detail error = %q", detail.Error)
		}
	}

	if got := len(clientset.Actions()); got != actionsAfterSync {
		t.Fatalf("client actions after cached reads = %d, want %d", got, actionsAfterSync)
	}
}

func TestResourceStoreTableSortsBySelectedColumn(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "api"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "api", Ready: true},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "cache"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "cache", Ready: true},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "prod"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "worker"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "worker", Ready: true},
				},
			},
		},
	)

	store := syncedTestStore(t, clientset)

	table := store.TableWithSort(KindPods, "", "", "Name", "desc")
	if table.SortColumn != "Name" || table.SortOrder != "desc" {
		t.Fatalf("sort state = %q/%q, want Name/desc", table.SortColumn, table.SortOrder)
	}
	if got := rowNames(table.Rows); !equalStrings(got, []string{"worker", "cache", "api"}) {
		t.Fatalf("rows sorted by name desc = %#v", got)
	}

	table = store.TableWithSort(KindPods, "", "", "Unknown", "desc")
	if table.SortColumn != "" || table.SortOrder != "" {
		t.Fatalf("invalid sort state = %q/%q, want empty", table.SortColumn, table.SortOrder)
	}
	if got := rowNames(table.Rows); !equalStrings(got, []string{"cache", "api", "worker"}) {
		t.Fatalf("rows with invalid sort = %#v", got)
	}
}

func syncedTestStore(t *testing.T, clientset *fake.Clientset) *ResourceStore {
	t.Helper()

	cluster := &Cluster{
		Clientset:   clientset,
		ContextName: "test",
		Namespace:   "default",
	}
	store := NewResourceStore(cluster, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	store.Start(ctx)

	syncCtx, syncCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer syncCancel()
	if !store.WaitForSync(syncCtx) {
		t.Fatal("store did not sync")
	}
	return store
}

func rowNames(rows []Row) []string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	return names
}

func ptr[T any](value T) *T {
	return &value
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
