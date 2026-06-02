package kube

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestResourceStoreReadsFromInformerCache(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "api"}},
				NodeName:   "node-1",
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
			},
			Status: appsv1.DeploymentStatus{
				ReadyReplicas:     2,
				UpdatedReplicas:   2,
				AvailableReplicas: 2,
			},
		},
	)

	store := syncedTestStore(t, clientset)

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
	}

	if got := len(clientset.Actions()); got != actionsAfterSync {
		t.Fatalf("client actions after cached reads = %d, want %d", got, actionsAfterSync)
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
