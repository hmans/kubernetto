package kube

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	nodev1 "k8s.io/api/node/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	dynamicfake "k8s.io/client-go/dynamic/fake"
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
	if want := []string{"Name", "Namespace", "Replicas", "CPU", "CPU Req", "CPU Limit", "MEM", "MEM Req", "MEM Limit", "Age"}; !equalStrings(deployments.Columns, want) {
		t.Fatalf("deployment columns = %#v, want %#v", deployments.Columns, want)
	}
	if got, want := deployments.Rows[0].Cells[2].Value, "2/2 ready"; got != want {
		t.Fatalf("deployment replicas = %q, want %q", got, want)
	}
	if got, want := deployments.Rows[0].Cells[3].Value, "37m"; got != want {
		t.Fatalf("deployment cpu = %q, want %q", got, want)
	}
	if got, want := deployments.Rows[0].Cells[4].Value, "100m"; got != want {
		t.Fatalf("deployment cpu request = %q, want %q", got, want)
	}
	if got, want := deployments.Rows[0].Cells[5].Value, "500m"; got != want {
		t.Fatalf("deployment cpu limit = %q, want %q", got, want)
	}
	if got, want := deployments.Rows[0].Cells[6].Value, "214Mi"; got != want {
		t.Fatalf("deployment mem = %q, want %q", got, want)
	}
	if got, want := deployments.Rows[0].Cells[7].Value, "128Mi"; got != want {
		t.Fatalf("deployment mem request = %q, want %q", got, want)
	}
	if got, want := deployments.Rows[0].Cells[8].Value, "256Mi"; got != want {
		t.Fatalf("deployment mem limit = %q, want %q", got, want)
	}
}

func TestResourceStoreClusterMapBuildsTopology(t *testing.T) {
	controller := true
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			}},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr(int32(1)),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 1, UpdatedReplicas: 1},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-7f68",
				Namespace: "prod",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "api", Controller: &controller},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-7f68-abcde",
				Namespace: "prod",
				Labels:    map[string]string{"app": "api"},
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ReplicaSet", Name: "api-7f68", Controller: &controller},
				},
			},
			Spec: corev1.PodSpec{
				NodeName:   "node-1",
				Containers: []corev1.Container{{Name: "api"}},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "api", Ready: true},
				},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeClusterIP,
				Selector: map[string]string{"app": "api"},
			},
		},
		&corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: "api-pv"},
			Status:     corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
		},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "fast"}, Provisioner: "kubernetes.io/no-provisioner"},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "reader"}, Rules: []rbacv1.PolicyRule{{Verbs: []string{"get"}, Resources: []string{"pods"}}}},
		&schedulingv1.PriorityClass{ObjectMeta: metav1.ObjectMeta{Name: "high"}, Value: 1000},
		&nodev1.RuntimeClass{ObjectMeta: metav1.ObjectMeta{Name: "runc"}, Handler: "runc"},
		&admissionv1.MutatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: "mutating"}, Webhooks: []admissionv1.MutatingWebhook{{Name: "mutate.example.com"}}},
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "api-warning", Namespace: "prod"},
			Type:       corev1.EventTypeWarning,
			Reason:     "BackOff",
			Message:    "retrying",
			InvolvedObject: corev1.ObjectReference{
				Kind:      "Pod",
				Name:      "api-7f68-abcde",
				Namespace: "prod",
			},
			Count:          3,
			LastTimestamp:  metav1.Now(),
			EventTime:      metav1.MicroTime{Time: time.Now()},
			FirstTimestamp: metav1.Now(),
		},
	)
	store := syncedTestStore(t, clientset)

	clusterMap, err := store.ClusterMap()
	if err != nil {
		t.Fatalf("ClusterMap: %v", err)
	}

	if clusterMap.Context != "test" || clusterMap.Counts.Nodes != 1 || clusterMap.Counts.Pods != 1 || clusterMap.Counts.Services != 1 {
		t.Fatalf("unexpected map counts: %#v", clusterMap)
	}
	if len(clusterMap.Nodes) != 1 || !clusterMap.Nodes[0].Ready || clusterMap.Nodes[0].PodCount != 1 {
		t.Fatalf("nodes = %#v, want ready node with one pod", clusterMap.Nodes)
	}
	if len(clusterMap.Workloads) != 1 || clusterMap.Workloads[0].ID != "workload:Deployment:prod:api" {
		t.Fatalf("workloads = %#v, want deployment workload", clusterMap.Workloads)
	}
	if got := clusterMap.Workloads[0].PodIDs; len(got) != 1 || got[0] != "pod:prod:api-7f68-abcde" {
		t.Fatalf("workload pod IDs = %#v", got)
	}
	if len(clusterMap.Pods) != 1 || clusterMap.Pods[0].OwnerKind != "Deployment" || clusterMap.Pods[0].OwnerName != "api" || clusterMap.Pods[0].OwnerID != "workload:Deployment:prod:api" {
		t.Fatalf("pod owner = %#v, want resolved deployment owner", clusterMap.Pods)
	}
	if got := clusterMap.Services[0].TargetPodIDs; len(got) != 1 || got[0] != "pod:prod:api-7f68-abcde" {
		t.Fatalf("service targets = %#v", got)
	}
	if len(clusterMap.Warnings) != 1 || clusterMap.Warnings[0].TargetID != "pod:prod:api-7f68-abcde" {
		t.Fatalf("warnings = %#v, want pod target", clusterMap.Warnings)
	}
	categories := map[string]bool{}
	for _, resource := range clusterMap.ClusterResources {
		categories[resource.Category] = true
	}
	for _, category := range []string{"Storage", "RBAC", "Scheduling", "Webhooks"} {
		if !categories[category] {
			t.Fatalf("cluster resource categories = %#v, missing %s", categories, category)
		}
	}
}

func TestResourceStoreClusterMapCapsVisibleObjects(t *testing.T) {
	objects := []runtime.Object{&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}}
	for i := 0; i < mapPodLimit+5; i++ {
		objects = append(objects, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("pod-%03d", i), Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "pod"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "pod", Ready: true},
				},
			},
		})
	}
	store := syncedTestStore(t, fake.NewSimpleClientset(objects...))

	clusterMap, err := store.ClusterMap()
	if err != nil {
		t.Fatalf("ClusterMap: %v", err)
	}

	if !clusterMap.Truncated {
		t.Fatalf("ClusterMap truncated = false, want true")
	}
	if len(clusterMap.Pods) != mapPodLimit {
		t.Fatalf("visible pods = %d, want %d", len(clusterMap.Pods), mapPodLimit)
	}
	if clusterMap.Counts.Pods != mapPodLimit+5 {
		t.Fatalf("pod count = %d, want full count", clusterMap.Counts.Pods)
	}
}

func TestResourceStoreClusterMapBalancesClusterResourceCap(t *testing.T) {
	objects := []runtime.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "fast"}, Provisioner: "kubernetes.io/no-provisioner"},
		&schedulingv1.PriorityClass{ObjectMeta: metav1.ObjectMeta{Name: "high"}, Value: 1000},
		&admissionv1.ValidatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: "validating"}, Webhooks: []admissionv1.ValidatingWebhook{{Name: "validate.example.com"}}},
	}
	for i := 0; i < mapClusterResourceLimit+20; i++ {
		objects = append(objects, &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("role-%03d", i)},
			Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get"}, Resources: []string{"pods"}}},
		})
	}
	store := syncedTestStore(t, fake.NewSimpleClientset(objects...))

	clusterMap, err := store.ClusterMap()
	if err != nil {
		t.Fatalf("ClusterMap: %v", err)
	}

	if !clusterMap.Truncated {
		t.Fatalf("ClusterMap truncated = false, want true")
	}
	if len(clusterMap.ClusterResources) != mapClusterResourceLimit {
		t.Fatalf("cluster resources = %d, want %d", len(clusterMap.ClusterResources), mapClusterResourceLimit)
	}
	categories := map[string]bool{}
	for _, resource := range clusterMap.ClusterResources {
		categories[resource.Category] = true
	}
	for _, category := range []string{"Storage", "RBAC", "Scheduling", "Webhooks"} {
		if !categories[category] {
			t.Fatalf("cluster resource categories = %#v, missing %s", categories, category)
		}
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

func TestResourceStoreTableQueryMatchesOnlyResourceName(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		testPod("api", "prod", 0),
		testPod("worker", "default", 0),
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "alice-backend", Namespace: "chatto-dev"},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr(int32(1)),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "alice-backend"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "alice-backend"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "alice-backend"}}},
				},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "chatto-hub", Namespace: "platform"},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr(int32(1)),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "chatto-hub"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "chatto-hub"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "chatto-hub"}}},
				},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
		},
	)
	store := syncedTestStore(t, clientset)

	if got := rowNames(store.Table(KindPods, "", "prod").Rows); len(got) != 0 {
		t.Fatalf("pod query matched namespace instead of name: %#v", got)
	}
	if got := rowNames(store.Table(KindPods, "", "Running").Rows); len(got) != 0 {
		t.Fatalf("pod query matched status instead of name: %#v", got)
	}
	if got, want := rowNames(store.Table(KindPods, "", "api").Rows), []string{"api"}; !equalStrings(got, want) {
		t.Fatalf("pod query by name = %#v, want %#v", got, want)
	}
	if got, want := rowNames(store.Table(KindDeployments, "", "chatto").Rows), []string{"chatto-hub"}; !equalStrings(got, want) {
		t.Fatalf("deployment query by name = %#v, want %#v", got, want)
	}
}

func TestResourceStoreTablesAndDetailsForExpandedResources(t *testing.T) {
	port := int32(8080)
	protocol := corev1.ProtocolTCP
	minAvailable := intstr.FromInt(1)
	clientset := fake.NewSimpleClientset(
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "api-rs", Namespace: "prod"}, Spec: appsv1.ReplicaSetSpec{Replicas: ptr(int32(1))}, Status: appsv1.ReplicaSetStatus{Replicas: 1, ReadyReplicas: 1}},
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "api-job", Namespace: "prod"}, Spec: batchv1.JobSpec{Completions: ptr(int32(1))}, Status: batchv1.JobStatus{Succeeded: 1}},
		&batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: "api-cron", Namespace: "prod"}, Spec: batchv1.CronJobSpec{Schedule: "*/5 * * * *"}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "api-pvc", Namespace: "prod"}, Spec: corev1.PersistentVolumeClaimSpec{VolumeName: "api-pv"}, Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound, Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}}},
		&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "api-pv"}, Spec: corev1.PersistentVolumeSpec{Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}, PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain}, Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound}},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "fast"}, Provisioner: "kubernetes.io/no-provisioner"},
		&corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "api-endpoints", Namespace: "prod"}, Subsets: []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}}, Ports: []corev1.EndpointPort{{Port: 80, Protocol: corev1.ProtocolTCP}}}}},
		&discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: "api-slice", Namespace: "prod"}, AddressType: discoveryv1.AddressTypeIPv4, Endpoints: []discoveryv1.Endpoint{{Addresses: []string{"10.0.0.1"}}}, Ports: []discoveryv1.EndpointPort{{Port: &port, Protocol: &protocol}}},
		&networkingv1.IngressClass{ObjectMeta: metav1.ObjectMeta{Name: "nginx"}, Spec: networkingv1.IngressClassSpec{Controller: "k8s.io/ingress-nginx"}},
		&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "api-netpol", Namespace: "prod"}, Spec: networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "api-sa", Namespace: "prod"}},
		&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "api-role", Namespace: "prod"}, Rules: []rbacv1.PolicyRule{{Verbs: []string{"get"}, Resources: []string{"pods"}}}},
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "api-binding", Namespace: "prod"}, RoleRef: rbacv1.RoleRef{Kind: "Role", Name: "api-role"}},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "api-cluster-role"}, Rules: []rbacv1.PolicyRule{{Verbs: []string{"list"}, Resources: []string{"nodes"}}}},
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "api-cluster-binding"}, RoleRef: rbacv1.RoleRef{Kind: "ClusterRole", Name: "api-cluster-role"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "api-config", Namespace: "prod"}, Data: map[string]string{"key": "value"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "api-secret", Namespace: "prod"}, Type: corev1.SecretTypeOpaque, Data: map[string][]byte{"token": []byte("secret")}},
		&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "api-hpa", Namespace: "prod"}, Spec: autoscalingv2.HorizontalPodAutoscalerSpec{ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "api"}, MinReplicas: ptr(int32(1)), MaxReplicas: 3}, Status: autoscalingv2.HorizontalPodAutoscalerStatus{CurrentReplicas: 1}},
		&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "api-pdb", Namespace: "prod"}, Spec: policyv1.PodDisruptionBudgetSpec{MinAvailable: &minAvailable}, Status: policyv1.PodDisruptionBudgetStatus{DisruptionsAllowed: 1}},
		&corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "api-quota", Namespace: "prod"}, Status: corev1.ResourceQuotaStatus{Hard: corev1.ResourceList{corev1.ResourcePods: resource.MustParse("10")}}},
		&corev1.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "api-limits", Namespace: "prod"}, Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{Type: corev1.LimitTypeContainer}}}},
		&schedulingv1.PriorityClass{ObjectMeta: metav1.ObjectMeta{Name: "high"}, Value: 1000},
		&nodev1.RuntimeClass{ObjectMeta: metav1.ObjectMeta{Name: "runc"}, Handler: "runc"},
		&coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Name: "api-lease", Namespace: "prod"}, Spec: coordinationv1.LeaseSpec{HolderIdentity: ptr("api")}},
		&admissionv1.MutatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: "api-mutating"}, Webhooks: []admissionv1.MutatingWebhook{{Name: "mutate.example.com"}}},
		&admissionv1.ValidatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: "api-validating"}, Webhooks: []admissionv1.ValidatingWebhook{{Name: "validate.example.com"}}},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "api-event", Namespace: "prod"}, Type: corev1.EventTypeWarning, Reason: "BackOff", Message: "retrying", InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "api", Namespace: "prod"}},
	)
	store := syncedTestStore(t, clientset)

	tests := []struct {
		kind      ResourceKind
		namespace string
		name      string
	}{
		{KindReplicaSets, "prod", "api-rs"},
		{KindJobs, "prod", "api-job"},
		{KindCronJobs, "prod", "api-cron"},
		{KindPersistentVolumeClaims, "prod", "api-pvc"},
		{KindPersistentVolumes, "", "api-pv"},
		{KindStorageClasses, "", "fast"},
		{KindEndpoints, "prod", "api-endpoints"},
		{KindEndpointSlices, "prod", "api-slice"},
		{KindIngressClasses, "", "nginx"},
		{KindNetworkPolicies, "prod", "api-netpol"},
		{KindServiceAccounts, "prod", "api-sa"},
		{KindRoles, "prod", "api-role"},
		{KindRoleBindings, "prod", "api-binding"},
		{KindClusterRoles, "", "api-cluster-role"},
		{KindClusterRoleBindings, "", "api-cluster-binding"},
		{KindConfigMaps, "prod", "api-config"},
		{KindSecrets, "prod", "api-secret"},
		{KindHorizontalPodAutoscalers, "prod", "api-hpa"},
		{KindPodDisruptionBudgets, "prod", "api-pdb"},
		{KindResourceQuotas, "prod", "api-quota"},
		{KindLimitRanges, "prod", "api-limits"},
		{KindPriorityClasses, "", "high"},
		{KindRuntimeClasses, "", "runc"},
		{KindLeases, "prod", "api-lease"},
		{KindMutatingWebhookConfigurations, "", "api-mutating"},
		{KindValidatingWebhookConfigurations, "", "api-validating"},
		{KindEvents, "prod", "api-event"},
	}

	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			table := store.Table(tt.kind, tt.namespace, "")
			if table.Error != "" {
				t.Fatalf("table error = %q", table.Error)
			}
			if len(table.Rows) != 1 || table.Rows[0].Name != tt.name {
				t.Fatalf("rows = %#v, want single row %q", rowNames(table.Rows), tt.name)
			}
			detail := store.Detail(tt.kind, tt.namespace, tt.name)
			if detail.Error != "" {
				t.Fatalf("detail error = %q", detail.Error)
			}
			if detail.Name != tt.name || detail.YAML == "" {
				t.Fatalf("detail = %#v, want name and yaml", detail)
			}
			if tt.kind == KindSecrets && strings.Contains(detail.YAML, "c2VjcmV0") {
				t.Fatalf("secret detail yaml exposed encoded secret data: %q", detail.YAML)
			}
		})
	}
}

func TestResourceStoreLogsMissingPodMetricsAPIOnce(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	store := &ResourceStore{logger: logger}
	err := apierrors.NewNotFound(schema.GroupResource{Group: "metrics.k8s.io", Resource: "pods"}, "")

	logged := false
	store.logPodMetricsError(err, &logged)
	store.logPodMetricsError(err, &logged)

	if !logged {
		t.Fatal("missing pod metrics API was not marked as logged")
	}
	output := logs.String()
	if got := strings.Count(output, "pod metrics API unavailable"); got != 1 {
		t.Fatalf("missing metrics API log count = %d, want 1; logs:\n%s", got, output)
	}
	if strings.Contains(output, "pod metrics unavailable") {
		t.Fatalf("missing metrics API should not log warning; logs:\n%s", output)
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

func TestResourceStoreTableSortsResourceQuantitiesNumerically(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		testPod("tiny", "default", 0),
		testPod("small", "default", 0),
		testPod("large", "default", 0),
		testPod("huge", "default", 0),
	)
	store := syncedTestStore(t, clientset)
	metricsClient := metricsfake.NewSimpleClientset()
	metricsClient.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &metricsv1beta1.PodMetricsList{
			Items: []metricsv1beta1.PodMetrics{
				testPodMetrics("tiny", "default", "37m", "736Ki"),
				testPodMetrics("small", "default", "250m", "90.9Mi"),
				testPodMetrics("large", "default", "1", "788Mi"),
				testPodMetrics("huge", "default", "1500m", "1Gi"),
			},
		}, nil
	})
	if err := store.loadPodMetrics(context.Background(), metricsClient); err != nil {
		t.Fatalf("load pod metrics: %v", err)
	}

	table := store.TableWithSort(KindPods, "default", "", "MEM", "asc")
	if got := rowNames(table.Rows); !equalStrings(got, []string{"tiny", "small", "large", "huge"}) {
		t.Fatalf("rows sorted by mem asc = %#v", got)
	}
	if got := table.Rows[1].Cells[8].Value; got != "90.9Mi" {
		t.Fatalf("formatted mem = %q, want 90.9Mi", got)
	}

	table = store.TableWithSort(KindPods, "default", "", "MEM", "desc")
	if got := rowNames(table.Rows); !equalStrings(got, []string{"huge", "large", "small", "tiny"}) {
		t.Fatalf("rows sorted by mem desc = %#v", got)
	}

	table = store.TableWithSort(KindPods, "default", "", "CPU", "asc")
	if got := rowNames(table.Rows); !equalStrings(got, []string{"tiny", "small", "large", "huge"}) {
		t.Fatalf("rows sorted by cpu asc = %#v", got)
	}
}

func TestResourceStoreTableSortsCountsNumerically(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		testPod("zero", "default", 0),
		testPod("two", "default", 2),
		testPod("ten", "default", 10),
	)
	store := syncedTestStore(t, clientset)

	table := store.TableWithSort(KindPods, "default", "", "Restarts", "asc")
	if got := rowNames(table.Rows); !equalStrings(got, []string{"zero", "two", "ten"}) {
		t.Fatalf("rows sorted by restarts asc = %#v", got)
	}

	table = store.TableWithSort(KindPods, "default", "", "Restarts", "desc")
	if got := rowNames(table.Rows); !equalStrings(got, []string{"ten", "two", "zero"}) {
		t.Fatalf("rows sorted by restarts desc = %#v", got)
	}
}

func TestResourceStoreCustomResourceTableAndDetail(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "stable.example.com", Version: "v1", Resource: "widgets"}
	kind := CustomResourceID(gvr.Group, gvr.Version, gvr.Resource)
	widget := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "stable.example.com/v1",
		"kind":       "Widget",
		"metadata": map[string]any{
			"name":              "api",
			"namespace":         "prod",
			"creationTimestamp": metav1.Now().Format(time.RFC3339),
			"labels": map[string]any{
				"app": "api",
			},
		},
		"spec": map[string]any{
			"replicas": int64(2),
		},
		"status": map[string]any{
			"phase": "Ready",
		},
	}}
	clientset := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod"}})
	cluster := &Cluster{
		Clientset:     clientset,
		DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "WidgetList"}, widget),
		ContextName:   "test",
		Namespace:     "default",
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
	store.customResources = []ResourceDef{{
		Kind:        kind,
		Label:       "Widget",
		Scope:       "namespaced",
		Group:       "custom",
		APIGroup:    gvr.Group,
		APIVersion:  gvr.Version,
		APIResource: gvr.Resource,
		ObjectKind:  "Widget",
		Custom:      true,
	}}

	table := store.Table(kind, "prod", "")
	if table.Error != "" {
		t.Fatalf("table error = %q", table.Error)
	}
	if want := []string{"Name", "Namespace", "Status", "Age"}; !equalStrings(table.Columns, want) {
		t.Fatalf("columns = %#v, want %#v", table.Columns, want)
	}
	if len(table.Rows) != 1 || table.Rows[0].Name != "api" || table.Rows[0].Status != "Ready" || table.Rows[0].StatusKey != "good" {
		t.Fatalf("unexpected rows: %#v", table.Rows)
	}

	detail := store.Detail(kind, "prod", "api")
	if detail.Error != "" {
		t.Fatalf("detail error = %q", detail.Error)
	}
	if detail.Label != "Widget" || detail.Name != "api" || detail.Status != "Ready" {
		t.Fatalf("unexpected detail: %#v", detail)
	}
	if !strings.Contains(detail.YAML, "apiVersion: stable.example.com/v1") || !strings.Contains(detail.YAML, "kind: Widget") {
		t.Fatalf("detail yaml missing identity: %q", detail.YAML)
	}
	if got := detailFieldValue(detail.Fields, "Resource"); got != "widgets" {
		t.Fatalf("detail resource field = %q, want widgets", got)
	}
}

func TestCustomResourceDefsFromCRDs(t *testing.T) {
	defs := customResourceDefsFromCRDs(&unstructured.UnstructuredList{Items: []unstructured.Unstructured{{
		Object: map[string]any{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]any{
				"name": "widgets.stable.example.com",
			},
			"spec": map[string]any{
				"group": "stable.example.com",
				"names": map[string]any{
					"kind":   "Widget",
					"plural": "widgets",
				},
				"scope": "Namespaced",
				"versions": []any{
					map[string]any{"name": "v1beta1", "served": true, "storage": false},
					map[string]any{"name": "v1", "served": true, "storage": true},
				},
			},
		},
	}}})

	if len(defs) != 1 {
		t.Fatalf("defs = %d, want 1", len(defs))
	}
	def := defs[0]
	if def.Kind != "custom:stable.example.com/v1/widgets" || def.Label != "Widget" || def.Scope != "namespaced" || !def.Custom {
		t.Fatalf("unexpected custom resource def: %#v", def)
	}

	if defs := customResourceDefsFromCRDs(&unstructured.UnstructuredList{}); len(defs) != 0 {
		t.Fatalf("empty CRD list produced defs: %#v", defs)
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

func testPod(name, namespace string, restarts int32) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: name}}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: name, Ready: true, RestartCount: restarts},
			},
		},
	}
}

func testPodMetrics(name, namespace, cpu, memory string) metricsv1beta1.PodMetrics {
	return metricsv1beta1.PodMetrics{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Containers: []metricsv1beta1.ContainerMetrics{
			{
				Name: name,
				Usage: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse(cpu),
					corev1.ResourceMemory: resource.MustParse(memory),
				},
			},
		},
	}
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
