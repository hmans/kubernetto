package kube

import (
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	mapPodLimit             = 300
	mapWorkloadLimit        = 120
	mapServiceLimit         = 120
	mapNamespaceLimit       = 40
	mapWarningLimit         = 20
	mapClusterResourceLimit = 180
)

type ClusterMap struct {
	Context          string               `json:"context"`
	Cluster          string               `json:"cluster"`
	UpdatedAt        time.Time            `json:"updatedAt"`
	Counts           MapCounts            `json:"counts"`
	Truncated        bool                 `json:"truncated"`
	Nodes            []MapNode            `json:"nodes"`
	Namespaces       []MapNamespace       `json:"namespaces"`
	Workloads        []MapWorkload        `json:"workloads"`
	Pods             []MapPod             `json:"pods"`
	Services         []MapService         `json:"services"`
	Warnings         []MapWarning         `json:"warnings"`
	ClusterResources []MapClusterResource `json:"clusterResources"`
}

type MapCounts struct {
	Nodes      int `json:"nodes"`
	Namespaces int `json:"namespaces"`
	Workloads  int `json:"workloads"`
	Pods       int `json:"pods"`
	Services   int `json:"services"`
	Warnings   int `json:"warnings"`
}

type MapNode struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Ready    bool   `json:"ready"`
	PodCount int    `json:"podCount"`
}

type MapNamespace struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	PodCount      int    `json:"podCount"`
	WorkloadCount int    `json:"workloadCount"`
	ServiceCount  int    `json:"serviceCount"`
	WarningCount  int    `json:"warningCount"`
}

type MapWorkload struct {
	ID        string       `json:"id"`
	Kind      ResourceKind `json:"kind"`
	Namespace string       `json:"namespace"`
	Name      string       `json:"name"`
	Ready     int32        `json:"ready"`
	Desired   int32        `json:"desired"`
	StatusKey string       `json:"statusKey"`
	PodIDs    []string     `json:"podIds"`
}

type MapPod struct {
	ID        string `json:"id"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Node      string `json:"node"`
	Phase     string `json:"phase"`
	Ready     bool   `json:"ready"`
	StatusKey string `json:"statusKey"`
	OwnerKind string `json:"ownerKind"`
	OwnerName string `json:"ownerName"`
	OwnerID   string `json:"ownerId"`
}

type MapService struct {
	ID              string   `json:"id"`
	Namespace       string   `json:"namespace"`
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	Selector        string   `json:"selector"`
	TargetPodIDs    []string `json:"targetPodIds"`
	TargetPodCount  int      `json:"targetPodCount"`
	TargetNamespace string   `json:"targetNamespace"`
}

type MapWarning struct {
	ID             string `json:"id"`
	Reason         string `json:"reason"`
	Message        string `json:"message"`
	Namespace      string `json:"namespace"`
	InvolvedObject string `json:"involvedObject"`
	TargetID       string `json:"targetId"`
	Count          int32  `json:"count"`
	Age            string `json:"age"`
}

type MapClusterResource struct {
	ID        string       `json:"id"`
	Category  string       `json:"category"`
	Kind      ResourceKind `json:"kind"`
	Name      string       `json:"name"`
	Detail    string       `json:"detail"`
	StatusKey string       `json:"statusKey"`
}

func (s *ResourceStore) ClusterMap() (ClusterMap, error) {
	out := ClusterMap{UpdatedAt: time.Now()}
	if s == nil || s.cluster == nil {
		return out, fmt.Errorf("No Kubernetes client is configured.")
	}
	out.Context = s.cluster.ContextName
	out.Cluster = s.cluster.ClusterName
	if out.Cluster == "" {
		out.Cluster = out.Context
	}
	if err := s.readinessError(); err != nil {
		return out, err
	}

	nodes := sortedNodes(s.listNodes())
	namespaces := sortedNamespaces(s.listNamespaces())
	pods := sortedPods(s.listPods(""))
	deployments := sortedDeployments(s.listDeployments(""))
	statefulSets := sortedStatefulSets(s.listStatefulSets(""))
	daemonSets := sortedDaemonSets(s.listDaemonSets(""))
	replicaSets := sortedReplicaSets(s.listReplicaSets(""))
	services := sortedServices(s.listServices(""))
	warnings := sortedWarningEvents(s.recentWarningEvents(mapWarningLimit))
	clusterResources := s.mapClusterResources()
	if len(clusterResources) > mapClusterResourceLimit {
		clusterResources = limitMapClusterResources(clusterResources, mapClusterResourceLimit)
		out.Truncated = true
	}

	out.Counts = MapCounts{
		Nodes:      len(nodes),
		Namespaces: len(namespaces),
		Workloads:  len(deployments) + len(statefulSets) + len(daemonSets),
		Pods:       len(pods),
		Services:   len(services),
		Warnings:   len(warnings),
	}
	out.ClusterResources = clusterResources

	if len(namespaces) > mapNamespaceLimit {
		namespaces = namespaces[:mapNamespaceLimit]
		out.Truncated = true
	}
	if len(pods) > mapPodLimit {
		pods = pods[:mapPodLimit]
		out.Truncated = true
	}
	if len(services) > mapServiceLimit {
		services = services[:mapServiceLimit]
		out.Truncated = true
	}

	podIDs := map[string]bool{}
	podsByNamespace := map[string][]*corev1.Pod{}
	podNamespaceNames := map[string]bool{}
	for _, pod := range pods {
		id := podMapID(pod.Namespace, pod.Name)
		podIDs[id] = true
		podNamespaceNames[pod.Namespace] = true
		podsByNamespace[pod.Namespace] = append(podsByNamespace[pod.Namespace], pod)
	}

	deploymentByReplicaSet := replicaSetDeploymentOwners(replicaSets)
	workloads := mapWorkloads(deployments, statefulSets, daemonSets)
	if len(workloads) > mapWorkloadLimit {
		workloads = workloads[:mapWorkloadLimit]
		out.Truncated = true
	}
	workloadIDs := map[string]bool{}
	for _, workload := range workloads {
		workloadIDs[workload.ID] = true
	}

	workloadByID := map[string]*MapWorkload{}
	for i := range workloads {
		workloadByID[workloads[i].ID] = &workloads[i]
	}

	nodePodCounts := map[string]int{}
	namespaceCounts := map[string]*MapNamespace{}
	for _, namespace := range namespaces {
		namespaceCounts[namespace.Name] = &MapNamespace{ID: namespaceMapID(namespace.Name), Name: namespace.Name}
	}
	for namespace := range podNamespaceNames {
		if namespaceCounts[namespace] == nil && len(namespaceCounts) < mapNamespaceLimit {
			namespaceCounts[namespace] = &MapNamespace{ID: namespaceMapID(namespace), Name: namespace}
		}
	}

	for _, workload := range workloads {
		if ns := namespaceCounts[workload.Namespace]; ns != nil {
			ns.WorkloadCount++
		}
	}

	for _, pod := range pods {
		ready, total, _ := podReady(*pod)
		podMap := MapPod{
			ID:        podMapID(pod.Namespace, pod.Name),
			Namespace: pod.Namespace,
			Name:      pod.Name,
			Node:      pod.Spec.NodeName,
			Phase:     string(pod.Status.Phase),
			Ready:     pod.DeletionTimestamp == nil && pod.Status.Phase == corev1.PodRunning && total > 0 && ready == total,
		}
		podMap.StatusKey = healthKey(podMap.Ready)
		if pod.Status.Phase == corev1.PodPending || pod.Status.Phase == corev1.PodUnknown {
			podMap.StatusKey = "warn"
		}
		if pod.Status.Phase == corev1.PodFailed {
			podMap.StatusKey = "danger"
		}
		owner := resolvedPodOwner(*pod, deploymentByReplicaSet)
		podMap.OwnerKind = owner.Kind
		podMap.OwnerName = owner.Name
		if owner.Kind != "" && owner.Name != "" {
			podMap.OwnerID = workloadMapID(owner.Kind, pod.Namespace, owner.Name)
			if workloadIDs[podMap.OwnerID] {
				if workload := workloadByID[podMap.OwnerID]; workload != nil {
					workload.PodIDs = append(workload.PodIDs, podMap.ID)
				}
			}
		}
		out.Pods = append(out.Pods, podMap)
		if pod.Spec.NodeName != "" {
			nodePodCounts[pod.Spec.NodeName]++
		}
		if ns := namespaceCounts[pod.Namespace]; ns != nil {
			ns.PodCount++
		}
	}

	for _, node := range nodes {
		out.Nodes = append(out.Nodes, MapNode{
			ID:       nodeMapID(node.Name),
			Name:     node.Name,
			Ready:    nodeReady(*node),
			PodCount: nodePodCounts[node.Name],
		})
	}

	for _, service := range services {
		targetIDs := serviceTargetPodIDs(service, podsByNamespace)
		serviceMap := MapService{
			ID:              serviceMapID(service.Namespace, service.Name),
			Namespace:       service.Namespace,
			Name:            service.Name,
			Type:            string(service.Spec.Type),
			Selector:        selectorSummary(service.Spec.Selector),
			TargetPodIDs:    targetIDs,
			TargetPodCount:  len(targetIDs),
			TargetNamespace: service.Namespace,
		}
		out.Services = append(out.Services, serviceMap)
		if ns := namespaceCounts[service.Namespace]; ns != nil {
			ns.ServiceCount++
		}
	}

	for _, warning := range warnings {
		targetID := warningTargetID(warning)
		out.Warnings = append(out.Warnings, MapWarning{
			ID:             eventMapID(warning.Namespace, warning.Name),
			Reason:         warning.Reason,
			Message:        warning.Message,
			Namespace:      warning.Namespace,
			InvolvedObject: warning.InvolvedObject,
			TargetID:       targetID,
			Count:          warning.Count,
			Age:            warning.Age,
		})
		if ns := namespaceCounts[warning.Namespace]; ns != nil {
			ns.WarningCount++
		}
	}

	out.Workloads = workloads
	for _, namespace := range namespaceCounts {
		out.Namespaces = append(out.Namespaces, *namespace)
	}
	sort.SliceStable(out.Namespaces, func(i, j int) bool {
		return strings.ToLower(out.Namespaces[i].Name) < strings.ToLower(out.Namespaces[j].Name)
	})
	return out, nil
}

type podMapOwner struct {
	Kind string
	Name string
}

func mapWorkloads(deployments []*appsv1.Deployment, statefulSets []*appsv1.StatefulSet, daemonSets []*appsv1.DaemonSet) []MapWorkload {
	out := make([]MapWorkload, 0, len(deployments)+len(statefulSets)+len(daemonSets))
	for _, deployment := range deployments {
		desired := int32(0)
		if deployment.Spec.Replicas != nil {
			desired = *deployment.Spec.Replicas
		}
		healthy := deployment.Status.ReadyReplicas == desired && deployment.Status.UpdatedReplicas == desired
		out = append(out, MapWorkload{
			ID:        workloadMapID("Deployment", deployment.Namespace, deployment.Name),
			Kind:      KindDeployments,
			Namespace: deployment.Namespace,
			Name:      deployment.Name,
			Ready:     deployment.Status.ReadyReplicas,
			Desired:   desired,
			StatusKey: healthKey(healthy),
		})
	}
	for _, statefulSet := range statefulSets {
		desired := int32(0)
		if statefulSet.Spec.Replicas != nil {
			desired = *statefulSet.Spec.Replicas
		}
		out = append(out, MapWorkload{
			ID:        workloadMapID("StatefulSet", statefulSet.Namespace, statefulSet.Name),
			Kind:      KindStatefulSet,
			Namespace: statefulSet.Namespace,
			Name:      statefulSet.Name,
			Ready:     statefulSet.Status.ReadyReplicas,
			Desired:   desired,
			StatusKey: healthKey(desired == statefulSet.Status.ReadyReplicas),
		})
	}
	for _, daemonSet := range daemonSets {
		out = append(out, MapWorkload{
			ID:        workloadMapID("DaemonSet", daemonSet.Namespace, daemonSet.Name),
			Kind:      KindDaemonSet,
			Namespace: daemonSet.Namespace,
			Name:      daemonSet.Name,
			Ready:     daemonSet.Status.NumberReady,
			Desired:   daemonSet.Status.DesiredNumberScheduled,
			StatusKey: healthKey(daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return strings.ToLower(out[i].Namespace) < strings.ToLower(out[j].Namespace)
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func replicaSetDeploymentOwners(replicaSets []*appsv1.ReplicaSet) map[string]string {
	out := map[string]string{}
	for _, replicaSet := range replicaSets {
		for _, owner := range replicaSet.OwnerReferences {
			if owner.Kind == "Deployment" && owner.Name != "" {
				out[podKey(replicaSet.Namespace, replicaSet.Name)] = owner.Name
				break
			}
		}
	}
	return out
}

func resolvedPodOwner(pod corev1.Pod, deploymentByReplicaSet map[string]string) podMapOwner {
	for _, owner := range pod.OwnerReferences {
		if owner.Controller != nil && !*owner.Controller {
			continue
		}
		if owner.Kind == "ReplicaSet" {
			if deployment := deploymentByReplicaSet[podKey(pod.Namespace, owner.Name)]; deployment != "" {
				return podMapOwner{Kind: "Deployment", Name: deployment}
			}
		}
		return podMapOwner{Kind: owner.Kind, Name: owner.Name}
	}
	if len(pod.OwnerReferences) > 0 {
		owner := pod.OwnerReferences[0]
		if owner.Kind == "ReplicaSet" {
			if deployment := deploymentByReplicaSet[podKey(pod.Namespace, owner.Name)]; deployment != "" {
				return podMapOwner{Kind: "Deployment", Name: deployment}
			}
		}
		return podMapOwner{Kind: owner.Kind, Name: owner.Name}
	}
	return podMapOwner{}
}

func serviceTargetPodIDs(service *corev1.Service, podsByNamespace map[string][]*corev1.Pod) []string {
	if service == nil || len(service.Spec.Selector) == 0 {
		return nil
	}
	selector := labels.SelectorFromSet(labels.Set(service.Spec.Selector))
	targets := []string{}
	for _, pod := range podsByNamespace[service.Namespace] {
		if selector.Matches(labels.Set(pod.Labels)) {
			targets = append(targets, podMapID(pod.Namespace, pod.Name))
		}
	}
	sort.Strings(targets)
	return targets
}

func selectorSummary(selector map[string]string) string {
	if len(selector) == 0 {
		return ""
	}
	keys := make([]string, 0, len(selector))
	for key := range selector {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+selector[key])
	}
	return strings.Join(parts, ", ")
}

func warningTargetID(warning OverviewEvent) string {
	kind, name, ok := strings.Cut(warning.InvolvedObject, "/")
	if !ok || name == "" {
		return namespaceMapID(warning.Namespace)
	}
	switch kind {
	case "Pod":
		return podMapID(warning.Namespace, name)
	case "Service":
		return serviceMapID(warning.Namespace, name)
	case "Node":
		return nodeMapID(name)
	case "Namespace":
		return namespaceMapID(name)
	case "Deployment", "StatefulSet", "DaemonSet":
		return workloadMapID(kind, warning.Namespace, name)
	default:
		return namespaceMapID(warning.Namespace)
	}
}

func nodeMapID(name string) string {
	return "node:" + name
}

func namespaceMapID(name string) string {
	return "namespace:" + name
}

func workloadMapID(kind, namespace, name string) string {
	return "workload:" + kind + ":" + namespace + ":" + name
}

func podMapID(namespace, name string) string {
	return "pod:" + namespace + ":" + name
}

func serviceMapID(namespace, name string) string {
	return "service:" + namespace + ":" + name
}

func eventMapID(namespace, name string) string {
	return "event:" + namespace + ":" + name
}

func clusterResourceMapID(category, kind, name string) string {
	return "cluster:" + category + ":" + kind + ":" + name
}

func sortedNodes(items []*corev1.Node) []*corev1.Node {
	sort.SliceStable(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items
}

func sortedNamespaces(items []*corev1.Namespace) []*corev1.Namespace {
	sort.SliceStable(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items
}

func sortedPods(items []*corev1.Pod) []*corev1.Pod {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Namespace != items[j].Namespace {
			return strings.ToLower(items[i].Namespace) < strings.ToLower(items[j].Namespace)
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items
}

func sortedDeployments(items []*appsv1.Deployment) []*appsv1.Deployment {
	sort.SliceStable(items, func(i, j int) bool {
		return namespacedLess(items[i].ObjectMeta, items[j].ObjectMeta)
	})
	return items
}

func sortedStatefulSets(items []*appsv1.StatefulSet) []*appsv1.StatefulSet {
	sort.SliceStable(items, func(i, j int) bool {
		return namespacedLess(items[i].ObjectMeta, items[j].ObjectMeta)
	})
	return items
}

func sortedDaemonSets(items []*appsv1.DaemonSet) []*appsv1.DaemonSet {
	sort.SliceStable(items, func(i, j int) bool {
		return namespacedLess(items[i].ObjectMeta, items[j].ObjectMeta)
	})
	return items
}

func sortedReplicaSets(items []*appsv1.ReplicaSet) []*appsv1.ReplicaSet {
	sort.SliceStable(items, func(i, j int) bool {
		return namespacedLess(items[i].ObjectMeta, items[j].ObjectMeta)
	})
	return items
}

func sortedServices(items []*corev1.Service) []*corev1.Service {
	sort.SliceStable(items, func(i, j int) bool {
		return namespacedLess(items[i].ObjectMeta, items[j].ObjectMeta)
	})
	return items
}

func sortedWarningEvents(items []OverviewEvent) []OverviewEvent {
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].LastSeen.After(items[j].LastSeen)
	})
	return items
}

func (s *ResourceStore) mapClusterResources() []MapClusterResource {
	out := []MapClusterResource{}
	for _, pv := range s.listPersistentVolumes() {
		out = append(out, MapClusterResource{
			ID:        clusterResourceMapID("Storage", string(KindPersistentVolumes), pv.Name),
			Category:  "Storage",
			Kind:      KindPersistentVolumes,
			Name:      pv.Name,
			Detail:    string(pv.Status.Phase),
			StatusKey: healthKey(pv.Status.Phase == corev1.VolumeBound),
		})
	}
	for _, storageClass := range s.listStorageClasses() {
		out = append(out, MapClusterResource{
			ID:        clusterResourceMapID("Storage", string(KindStorageClasses), storageClass.Name),
			Category:  "Storage",
			Kind:      KindStorageClasses,
			Name:      storageClass.Name,
			Detail:    storageClass.Provisioner,
			StatusKey: "neutral",
		})
	}
	for _, role := range s.listClusterRoles() {
		out = append(out, MapClusterResource{
			ID:        clusterResourceMapID("RBAC", string(KindClusterRoles), role.Name),
			Category:  "RBAC",
			Kind:      KindClusterRoles,
			Name:      role.Name,
			Detail:    fmt.Sprintf("%d rules", len(role.Rules)),
			StatusKey: "neutral",
		})
	}
	for _, binding := range s.listClusterRoleBindings() {
		out = append(out, MapClusterResource{
			ID:        clusterResourceMapID("RBAC", string(KindClusterRoleBindings), binding.Name),
			Category:  "RBAC",
			Kind:      KindClusterRoleBindings,
			Name:      binding.Name,
			Detail:    binding.RoleRef.Kind + "/" + binding.RoleRef.Name,
			StatusKey: "neutral",
		})
	}
	for _, class := range s.listPriorityClasses() {
		out = append(out, MapClusterResource{
			ID:        clusterResourceMapID("Scheduling", string(KindPriorityClasses), class.Name),
			Category:  "Scheduling",
			Kind:      KindPriorityClasses,
			Name:      class.Name,
			Detail:    fmt.Sprintf("value %d", class.Value),
			StatusKey: "neutral",
		})
	}
	for _, class := range s.listRuntimeClasses() {
		out = append(out, MapClusterResource{
			ID:        clusterResourceMapID("Scheduling", string(KindRuntimeClasses), class.Name),
			Category:  "Scheduling",
			Kind:      KindRuntimeClasses,
			Name:      class.Name,
			Detail:    class.Handler,
			StatusKey: "neutral",
		})
	}
	for _, config := range s.listMutatingWebhookConfigurations() {
		out = append(out, MapClusterResource{
			ID:        clusterResourceMapID("Webhooks", string(KindMutatingWebhookConfigurations), config.Name),
			Category:  "Webhooks",
			Kind:      KindMutatingWebhookConfigurations,
			Name:      config.Name,
			Detail:    fmt.Sprintf("%d webhooks", len(config.Webhooks)),
			StatusKey: "neutral",
		})
	}
	for _, config := range s.listValidatingWebhookConfigurations() {
		out = append(out, MapClusterResource{
			ID:        clusterResourceMapID("Webhooks", string(KindValidatingWebhookConfigurations), config.Name),
			Category:  "Webhooks",
			Kind:      KindValidatingWebhookConfigurations,
			Name:      config.Name,
			Detail:    fmt.Sprintf("%d webhooks", len(config.Webhooks)),
			StatusKey: "neutral",
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return mapClusterResourceLess(out[i], out[j]) })
	return out
}

func limitMapClusterResources(resources []MapClusterResource, limit int) []MapClusterResource {
	if limit <= 0 || len(resources) == 0 {
		return nil
	}
	if len(resources) <= limit {
		return resources
	}
	groups := map[string][]MapClusterResource{}
	categories := []string{}
	for _, resource := range resources {
		if _, ok := groups[resource.Category]; !ok {
			categories = append(categories, resource.Category)
		}
		groups[resource.Category] = append(groups[resource.Category], resource)
	}
	sort.SliceStable(categories, func(i, j int) bool {
		return mapClusterCategoryRank(categories[i]) < mapClusterCategoryRank(categories[j]) ||
			(mapClusterCategoryRank(categories[i]) == mapClusterCategoryRank(categories[j]) && categories[i] < categories[j])
	})

	indexes := map[string]int{}
	out := make([]MapClusterResource, 0, limit)
	for len(out) < limit {
		added := false
		for _, category := range categories {
			index := indexes[category]
			if index >= len(groups[category]) {
				continue
			}
			out = append(out, groups[category][index])
			indexes[category] = index + 1
			added = true
			if len(out) == limit {
				break
			}
		}
		if !added {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return mapClusterResourceLess(out[i], out[j]) })
	return out
}

func mapClusterResourceLess(left, right MapClusterResource) bool {
	if left.Category != right.Category {
		leftRank := mapClusterCategoryRank(left.Category)
		rightRank := mapClusterCategoryRank(right.Category)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return left.Category < right.Category
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	return strings.ToLower(left.Name) < strings.ToLower(right.Name)
}

func mapClusterCategoryRank(category string) int {
	switch category {
	case "Storage":
		return 10
	case "RBAC":
		return 20
	case "Scheduling":
		return 30
	case "Webhooks":
		return 40
	default:
		return 100
	}
}

func namespacedLess(left, right metav1.ObjectMeta) bool {
	if left.Namespace != right.Namespace {
		return strings.ToLower(left.Namespace) < strings.ToLower(right.Namespace)
	}
	return strings.ToLower(left.Name) < strings.ToLower(right.Name)
}
