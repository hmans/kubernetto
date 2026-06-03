package kube

import (
	"context"
	"fmt"
	"sort"
	"strings"
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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
)

type ResourceKind string

const (
	KindOverview                        ResourceKind = "overview"
	KindActions                         ResourceKind = "actions"
	KindPods                            ResourceKind = "pods"
	KindDeployments                     ResourceKind = "deployments"
	KindStatefulSet                     ResourceKind = "statefulsets"
	KindDaemonSet                       ResourceKind = "daemonsets"
	KindReplicaSets                     ResourceKind = "replicasets"
	KindJobs                            ResourceKind = "jobs"
	KindCronJobs                        ResourceKind = "cronjobs"
	KindPersistentVolumeClaims          ResourceKind = "persistentvolumeclaims"
	KindPersistentVolumes               ResourceKind = "persistentvolumes"
	KindStorageClasses                  ResourceKind = "storageclasses"
	KindServices                        ResourceKind = "services"
	KindEndpoints                       ResourceKind = "endpoints"
	KindEndpointSlices                  ResourceKind = "endpointslices"
	KindIngresses                       ResourceKind = "ingresses"
	KindIngressClasses                  ResourceKind = "ingressclasses"
	KindNetworkPolicies                 ResourceKind = "networkpolicies"
	KindServiceAccounts                 ResourceKind = "serviceaccounts"
	KindRoles                           ResourceKind = "roles"
	KindRoleBindings                    ResourceKind = "rolebindings"
	KindClusterRoles                    ResourceKind = "clusterroles"
	KindClusterRoleBindings             ResourceKind = "clusterrolebindings"
	KindConfigMaps                      ResourceKind = "configmaps"
	KindSecrets                         ResourceKind = "secrets"
	KindHorizontalPodAutoscalers        ResourceKind = "horizontalpodautoscalers"
	KindPodDisruptionBudgets            ResourceKind = "poddisruptionbudgets"
	KindResourceQuotas                  ResourceKind = "resourcequotas"
	KindLimitRanges                     ResourceKind = "limitranges"
	KindPriorityClasses                 ResourceKind = "priorityclasses"
	KindRuntimeClasses                  ResourceKind = "runtimeclasses"
	KindLeases                          ResourceKind = "leases"
	KindMutatingWebhookConfigurations   ResourceKind = "mutatingwebhookconfigurations"
	KindValidatingWebhookConfigurations ResourceKind = "validatingwebhookconfigurations"
	KindNodes                           ResourceKind = "nodes"
	KindNamespaces                      ResourceKind = "namespaces"
	KindEvents                          ResourceKind = "events"
)

type ResourceDef struct {
	Kind    ResourceKind
	Label   string
	Scope   string
	Group   string
	Default bool
}

type ResourceGroupDef struct {
	ID          string
	Label       string
	DefaultKind ResourceKind
	Kinds       []ResourceKind
}

var ResourceDefs = []ResourceDef{
	{Kind: KindOverview, Label: "Overview", Scope: "cluster", Group: "overview", Default: true},
	{Kind: KindActions, Label: "Issues", Scope: "cluster", Group: "overview"},
	{Kind: KindPods, Label: "Pods", Scope: "namespaced", Group: "workloads", Default: true},
	{Kind: KindDeployments, Label: "Deployments", Scope: "namespaced", Group: "workloads"},
	{Kind: KindStatefulSet, Label: "StatefulSets", Scope: "namespaced", Group: "workloads"},
	{Kind: KindDaemonSet, Label: "DaemonSets", Scope: "namespaced", Group: "workloads"},
	{Kind: KindReplicaSets, Label: "ReplicaSets", Scope: "namespaced", Group: "workloads"},
	{Kind: KindJobs, Label: "Jobs", Scope: "namespaced", Group: "workloads"},
	{Kind: KindCronJobs, Label: "CronJobs", Scope: "namespaced", Group: "workloads"},
	{Kind: KindPersistentVolumeClaims, Label: "PersistentVolumeClaims", Scope: "namespaced", Group: "storage", Default: true},
	{Kind: KindPersistentVolumes, Label: "PersistentVolumes", Scope: "cluster", Group: "storage"},
	{Kind: KindStorageClasses, Label: "StorageClasses", Scope: "cluster", Group: "storage"},
	{Kind: KindServices, Label: "Services", Scope: "namespaced", Group: "network", Default: true},
	{Kind: KindEndpoints, Label: "Endpoints", Scope: "namespaced", Group: "network"},
	{Kind: KindEndpointSlices, Label: "EndpointSlices", Scope: "namespaced", Group: "network"},
	{Kind: KindIngresses, Label: "Ingresses", Scope: "namespaced", Group: "network"},
	{Kind: KindIngressClasses, Label: "IngressClasses", Scope: "cluster", Group: "network"},
	{Kind: KindNetworkPolicies, Label: "NetworkPolicies", Scope: "namespaced", Group: "network"},
	{Kind: KindServiceAccounts, Label: "ServiceAccounts", Scope: "namespaced", Group: "security", Default: true},
	{Kind: KindRoles, Label: "Roles", Scope: "namespaced", Group: "security"},
	{Kind: KindRoleBindings, Label: "RoleBindings", Scope: "namespaced", Group: "security"},
	{Kind: KindClusterRoles, Label: "ClusterRoles", Scope: "cluster", Group: "security"},
	{Kind: KindClusterRoleBindings, Label: "ClusterRoleBindings", Scope: "cluster", Group: "security"},
	{Kind: KindConfigMaps, Label: "ConfigMaps", Scope: "namespaced", Group: "configuration", Default: true},
	{Kind: KindSecrets, Label: "Secrets", Scope: "namespaced", Group: "configuration"},
	{Kind: KindHorizontalPodAutoscalers, Label: "HPAs", Scope: "namespaced", Group: "configuration"},
	{Kind: KindPodDisruptionBudgets, Label: "PodDisruptionBudgets", Scope: "namespaced", Group: "configuration"},
	{Kind: KindResourceQuotas, Label: "ResourceQuotas", Scope: "namespaced", Group: "configuration"},
	{Kind: KindLimitRanges, Label: "LimitRanges", Scope: "namespaced", Group: "configuration"},
	{Kind: KindPriorityClasses, Label: "PriorityClasses", Scope: "cluster", Group: "configuration"},
	{Kind: KindRuntimeClasses, Label: "RuntimeClasses", Scope: "cluster", Group: "configuration"},
	{Kind: KindLeases, Label: "Leases", Scope: "namespaced", Group: "configuration"},
	{Kind: KindMutatingWebhookConfigurations, Label: "MutatingWebhookConfigurations", Scope: "cluster", Group: "configuration"},
	{Kind: KindValidatingWebhookConfigurations, Label: "ValidatingWebhookConfigurations", Scope: "cluster", Group: "configuration"},
	{Kind: KindNodes, Label: "Nodes", Scope: "cluster", Group: "cluster", Default: true},
	{Kind: KindNamespaces, Label: "Namespaces", Scope: "cluster", Group: "cluster"},
	{Kind: KindEvents, Label: "Events", Scope: "namespaced", Group: "cluster"},
}

var ResourceGroups = []ResourceGroupDef{
	{ID: "overview", Label: "Overview", DefaultKind: KindOverview, Kinds: []ResourceKind{KindOverview, KindActions}},
	{ID: "workloads", Label: "Workloads", DefaultKind: KindPods, Kinds: []ResourceKind{KindPods, KindDeployments, KindStatefulSet, KindDaemonSet, KindReplicaSets, KindJobs, KindCronJobs}},
	{ID: "storage", Label: "Storage", DefaultKind: KindPersistentVolumeClaims, Kinds: []ResourceKind{KindPersistentVolumeClaims, KindPersistentVolumes, KindStorageClasses}},
	{ID: "network", Label: "Network", DefaultKind: KindServices, Kinds: []ResourceKind{KindServices, KindEndpoints, KindEndpointSlices, KindIngresses, KindIngressClasses, KindNetworkPolicies}},
	{ID: "security", Label: "Security", DefaultKind: KindServiceAccounts, Kinds: []ResourceKind{KindServiceAccounts, KindRoles, KindRoleBindings, KindClusterRoles, KindClusterRoleBindings}},
	{ID: "configuration", Label: "Configuration", DefaultKind: KindConfigMaps, Kinds: []ResourceKind{KindConfigMaps, KindSecrets, KindHorizontalPodAutoscalers, KindPodDisruptionBudgets, KindResourceQuotas, KindLimitRanges, KindPriorityClasses, KindRuntimeClasses, KindLeases, KindMutatingWebhookConfigurations, KindValidatingWebhookConfigurations}},
	{ID: "cluster", Label: "Cluster", DefaultKind: KindNodes, Kinds: []ResourceKind{KindNodes, KindNamespaces, KindEvents}},
}

type Summary struct {
	ServerVersion string
	Context       string
	Namespace     string
	Nodes         int
	Namespaces    int
	Pods          int
	Deployments   int
	Error         string
	UpdatedAt     time.Time
}

type ClusterOverview struct {
	Error         string
	UpdatedAt     time.Time
	Identity      []DetailField
	Resources     []OverviewMetric
	Stats         []OverviewMetric
	WarningEvents []OverviewEvent
}

type MetricsState struct {
	Available bool
	Message   string
	Source    string
	Window    string
	UpdatedAt time.Time
}

type OverviewMetric struct {
	Label     string
	Value     string
	Detail    string
	StatusKey string
	Kind      ResourceKind
	Ratio     *OverviewRatio
}

type OverviewRatio struct {
	Percent     float64
	Numerator   string
	Denominator string
}

type OverviewEvent struct {
	Name           string
	Type           string
	Reason         string
	Message        string
	Namespace      string
	InvolvedObject string
	Count          int32
	Age            string
	LastSeen       time.Time
}

type Table struct {
	Kind       ResourceKind
	Label      string
	Columns    []string
	Rows       []Row
	Namespace  string
	Query      string
	SortColumn string
	SortOrder  string
	Error      string
	UpdatedAt  time.Time
	Namespaced bool
}

type Row struct {
	Name      string
	Namespace string
	Cluster   string
	Cells     []Cell
	Status    string
	StatusKey string
}

type Cell struct {
	Value     string
	Class     string
	SortValue *int64
}

type ResourceDetail struct {
	Kind        ResourceKind
	Label       string
	Name        string
	Namespace   string
	Status      string
	StatusKey   string
	Age         string
	CreatedAt   time.Time
	UID         string
	Error       string
	YAML        string
	Events      []ResourceEvent
	Fields      []DetailField
	Sections    []DetailSection
	Labels      []DetailField
	Annotations []DetailField
}

type PodUsageOverviewChart struct {
	Metrics MetricsState
	Pods    []PodUsageGraph
}

type PodUsageDetailChart struct {
	Name      string
	Namespace string
	Metrics   MetricsState
	Usage     []UsageGraph
	Samples   []UsageSample
}

type PodUsageGraph struct {
	Name      string
	Namespace string
	CPU       string
	Memory    string
	Samples   []UsageSample
}

type UsageGraph struct {
	Label string
	Value string
}

type UsageSample struct {
	Timestamp time.Time
	CPU       int64
	Memory    int64
}

type ResourceEvent struct {
	Type     string
	Reason   string
	Message  string
	Count    int32
	Age      string
	LastSeen time.Time
}

type DetailSection struct {
	Title  string
	Fields []DetailField
}

type DetailField struct {
	Name  string
	Value string
	Link  *DetailLink
}

type DetailLink struct {
	Resource  ResourceKind
	Namespace string
	Name      string
}

func NormalizeKind(kind string) ResourceKind {
	for _, def := range ResourceDefs {
		if string(def.Kind) == kind {
			return def.Kind
		}
	}
	return KindOverview
}

func (c *Cluster) Summary(ctx context.Context) Summary {
	out := Summary{
		Context:   c.ContextName,
		Namespace: c.Namespace,
		UpdatedAt: time.Now(),
	}
	if c == nil || c.Clientset == nil {
		out.Error = "No Kubernetes client is configured."
		return out
	}

	if version, err := c.Discovery.ServerVersion(); err == nil {
		out.ServerVersion = version.GitVersion
	} else {
		out.Error = err.Error()
	}

	if nodes, err := c.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{}); err == nil {
		out.Nodes = len(nodes.Items)
	}
	if namespaces, err := c.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{}); err == nil {
		out.Namespaces = len(namespaces.Items)
	}
	if pods, err := c.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{}); err == nil {
		out.Pods = len(pods.Items)
	}
	if deployments, err := c.Clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{}); err == nil {
		out.Deployments = len(deployments.Items)
	}

	return out
}

func (c *Cluster) Namespaces(ctx context.Context) ([]string, error) {
	if c == nil || c.Clientset == nil {
		return nil, fmt.Errorf("no Kubernetes client is configured")
	}
	list, err := c.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	namespaces := make([]string, 0, len(list.Items))
	for _, namespace := range list.Items {
		namespaces = append(namespaces, namespace.Name)
	}
	sort.Strings(namespaces)
	return namespaces, nil
}

func (c *Cluster) Table(ctx context.Context, kind ResourceKind, namespace, query string) Table {
	return c.TableWithSort(ctx, kind, namespace, query, "", "")
}

func (c *Cluster) TableWithSort(ctx context.Context, kind ResourceKind, namespace, query, sortColumn, sortOrder string) Table {
	def := resourceDef(kind)
	table := Table{
		Kind:       def.Kind,
		Label:      def.Label,
		Namespace:  namespace,
		Query:      query,
		UpdatedAt:  time.Now(),
		Namespaced: def.Scope == "namespaced",
	}

	if c == nil || c.Clientset == nil {
		table.Error = "No Kubernetes client is configured."
		return table
	}

	switch def.Kind {
	case KindPods:
		table.Columns = []string{"Name", "Namespace", "Ready", "Status", "Restarts", "CPU", "CPU Req", "CPU Limit", "MEM", "MEM Req", "MEM Limit", "Node", "Age"}
		list, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, pod := range list.Items {
			row := podRow(pod, nil)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindDeployments:
		table.Columns = []string{"Name", "Namespace", "Replicas", "CPU", "CPU Req", "CPU Limit", "MEM", "MEM Req", "MEM Limit", "Age"}
		list, err := c.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, deployment := range list.Items {
			row := deploymentRow(deployment, nil)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindStatefulSet:
		table.Columns = []string{"Name", "Namespace", "Replicas", "CPU", "CPU Req", "CPU Limit", "MEM", "MEM Req", "MEM Limit", "Age"}
		list, err := c.Clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, statefulSet := range list.Items {
			row := statefulSetRow(statefulSet, nil)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindDaemonSet:
		table.Columns = []string{"Name", "Namespace", "Replicas", "CPU", "CPU Req", "CPU Limit", "MEM", "MEM Req", "MEM Limit", "Age"}
		list, err := c.Clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, daemonSet := range list.Items {
			row := daemonSetRow(daemonSet, nil)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindReplicaSets:
		table.Columns = []string{"Name", "Namespace", "Replicas", "Age"}
		list, err := c.Clientset.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, replicaSet := range list.Items {
			row := replicaSetRow(replicaSet)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindJobs:
		table.Columns = []string{"Name", "Namespace", "Completions", "Succeeded", "Failed", "Age"}
		list, err := c.Clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, job := range list.Items {
			row := jobRow(job)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindCronJobs:
		table.Columns = []string{"Name", "Namespace", "Schedule", "Suspend", "Active", "Last Schedule", "Age"}
		list, err := c.Clientset.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, cronJob := range list.Items {
			row := cronJobRow(cronJob)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindPersistentVolumeClaims:
		table.Columns = []string{"Name", "Namespace", "Status", "Volume", "Capacity", "StorageClass", "Age"}
		list, err := c.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, pvc := range list.Items {
			row := persistentVolumeClaimRow(pvc)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindPersistentVolumes:
		table.Columns = []string{"Name", "Status", "Capacity", "Access Modes", "Reclaim Policy", "StorageClass", "Claim", "Age"}
		list, err := c.Clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, pv := range list.Items {
			row := persistentVolumeRow(pv)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindStorageClasses:
		table.Columns = []string{"Name", "Provisioner", "Reclaim Policy", "Binding Mode", "Default", "Age"}
		list, err := c.Clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, storageClass := range list.Items {
			row := storageClassRow(storageClass)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindServices:
		table.Columns = []string{"Name", "Namespace", "Type", "Cluster IP", "Ports", "Age"}
		list, err := c.Clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, service := range list.Items {
			row := serviceRow(service)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindEndpoints:
		table.Columns = []string{"Name", "Namespace", "Addresses", "Not Ready", "Ports", "Age"}
		list, err := c.Clientset.CoreV1().Endpoints(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, endpoint := range list.Items {
			row := endpointsRow(endpoint)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindEndpointSlices:
		table.Columns = []string{"Name", "Namespace", "Address Type", "Endpoints", "Ports", "Age"}
		list, err := c.Clientset.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, endpointSlice := range list.Items {
			row := endpointSliceRow(endpointSlice)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindIngresses:
		table.Columns = []string{"Name", "Namespace", "Class", "Hosts", "Address", "Age"}
		list, err := c.Clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, ingress := range list.Items {
			row := ingressRow(ingress)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindIngressClasses:
		table.Columns = []string{"Name", "Controller", "Default", "Age"}
		list, err := c.Clientset.NetworkingV1().IngressClasses().List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, ingressClass := range list.Items {
			row := ingressClassRow(ingressClass)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindNetworkPolicies:
		table.Columns = []string{"Name", "Namespace", "Pod Selector", "Policy Types", "Age"}
		list, err := c.Clientset.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, policy := range list.Items {
			row := networkPolicyRow(policy)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindServiceAccounts:
		table.Columns = []string{"Name", "Namespace", "Secrets", "Image Pull Secrets", "Age"}
		list, err := c.Clientset.CoreV1().ServiceAccounts(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, account := range list.Items {
			row := serviceAccountRow(account)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindRoles:
		table.Columns = []string{"Name", "Namespace", "Rules", "Age"}
		list, err := c.Clientset.RbacV1().Roles(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, role := range list.Items {
			row := roleRow(role)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindRoleBindings:
		table.Columns = []string{"Name", "Namespace", "Role", "Subjects", "Age"}
		list, err := c.Clientset.RbacV1().RoleBindings(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, binding := range list.Items {
			row := roleBindingRow(binding)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindClusterRoles:
		table.Columns = []string{"Name", "Rules", "Age"}
		list, err := c.Clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, role := range list.Items {
			row := clusterRoleRow(role)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindClusterRoleBindings:
		table.Columns = []string{"Name", "Role", "Subjects", "Age"}
		list, err := c.Clientset.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, binding := range list.Items {
			row := clusterRoleBindingRow(binding)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindConfigMaps:
		table.Columns = []string{"Name", "Namespace", "Data", "Age"}
		list, err := c.Clientset.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, configMap := range list.Items {
			row := configMapRow(configMap)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindSecrets:
		table.Columns = []string{"Name", "Namespace", "Type", "Data", "Age"}
		list, err := c.Clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, secret := range list.Items {
			row := secretRow(secret)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindHorizontalPodAutoscalers:
		table.Columns = []string{"Name", "Namespace", "Reference", "Min", "Max", "Replicas", "Age"}
		list, err := c.Clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, hpa := range list.Items {
			row := horizontalPodAutoscalerRow(hpa)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindPodDisruptionBudgets:
		table.Columns = []string{"Name", "Namespace", "Min Available", "Max Unavailable", "Allowed", "Age"}
		list, err := c.Clientset.PolicyV1().PodDisruptionBudgets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, pdb := range list.Items {
			row := podDisruptionBudgetRow(pdb)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindResourceQuotas:
		table.Columns = []string{"Name", "Namespace", "Hard", "Used", "Age"}
		list, err := c.Clientset.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, quota := range list.Items {
			row := resourceQuotaRow(quota)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindLimitRanges:
		table.Columns = []string{"Name", "Namespace", "Limits", "Age"}
		list, err := c.Clientset.CoreV1().LimitRanges(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, limitRange := range list.Items {
			row := limitRangeRow(limitRange)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindPriorityClasses:
		table.Columns = []string{"Name", "Value", "Global Default", "Preemption", "Age"}
		list, err := c.Clientset.SchedulingV1().PriorityClasses().List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, priorityClass := range list.Items {
			row := priorityClassRow(priorityClass)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindRuntimeClasses:
		table.Columns = []string{"Name", "Handler", "Age"}
		list, err := c.Clientset.NodeV1().RuntimeClasses().List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, runtimeClass := range list.Items {
			row := runtimeClassRow(runtimeClass)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindLeases:
		table.Columns = []string{"Name", "Namespace", "Holder", "Renew Time", "Age"}
		list, err := c.Clientset.CoordinationV1().Leases(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, lease := range list.Items {
			row := leaseRow(lease)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindMutatingWebhookConfigurations:
		table.Columns = []string{"Name", "Webhooks", "Age"}
		list, err := c.Clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, config := range list.Items {
			row := mutatingWebhookConfigurationRow(config)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindValidatingWebhookConfigurations:
		table.Columns = []string{"Name", "Webhooks", "Age"}
		list, err := c.Clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, config := range list.Items {
			row := validatingWebhookConfigurationRow(config)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindNodes:
		table.Columns = []string{"Name", "Status", "Roles", "Version", "Internal IP", "Age"}
		list, err := c.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, node := range list.Items {
			row := nodeRow(node)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindNamespaces:
		table.Columns = []string{"Name", "Status", "Age"}
		list, err := c.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, namespace := range list.Items {
			row := namespaceRow(namespace)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindEvents:
		table.Columns = []string{"Name", "Namespace", "Type", "Reason", "Object", "Message", "Age"}
		list, err := c.Clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			table.Error = err.Error()
			return table
		}
		for _, event := range list.Items {
			row := eventRow(event)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	}

	sortTableRows(&table, sortColumn, sortOrder)

	return table
}

func resourceDef(kind ResourceKind) ResourceDef {
	for _, def := range ResourceDefs {
		if def.Kind == kind {
			return def
		}
	}
	return ResourceDefs[0]
}

func podRow(pod corev1.Pod, usage corev1.ResourceList) Row {
	ready := 0
	restarts := int32(0)
	for _, status := range pod.Status.ContainerStatuses {
		if status.Ready {
			ready++
		}
		restarts += status.RestartCount
	}
	total := len(pod.Spec.Containers)
	status := string(pod.Status.Phase)
	if pod.DeletionTimestamp != nil {
		status = "Terminating"
	}
	return Row{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Status:    status,
		StatusKey: healthKey(status == "Running" && ready == total),
		Cells: []Cell{
			{Value: pod.Name, Class: "primary"},
			{Value: pod.Namespace},
			cellWithOptionalSort(fmt.Sprintf("%d/%d", ready, total), ratioSortValue(int64(ready), int64(total))),
			{Value: status, Class: "status " + healthKey(status == "Running" && ready == total)},
			cellWithSortValue(fmt.Sprint(restarts), int64(restarts)),
			resourceListCell(usage, corev1.ResourceCPU),
			containerResourceCell(pod.Spec.Containers, resourceRequests, corev1.ResourceCPU),
			containerResourceCell(pod.Spec.Containers, resourceLimits, corev1.ResourceCPU),
			resourceListCell(usage, corev1.ResourceMemory),
			containerResourceCell(pod.Spec.Containers, resourceRequests, corev1.ResourceMemory),
			containerResourceCell(pod.Spec.Containers, resourceLimits, corev1.ResourceMemory),
			{Value: pod.Spec.NodeName},
			ageCell(pod.CreationTimestamp.Time),
		},
	}
}

func deploymentRow(deployment appsv1.Deployment, usage corev1.ResourceList) Row {
	desired := int32(0)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	healthy := deployment.Status.ReadyReplicas == desired && deployment.Status.UpdatedReplicas == desired && deployment.Status.AvailableReplicas == desired
	return Row{
		Name:      deployment.Name,
		Namespace: deployment.Namespace,
		Status:    fmt.Sprintf("%d/%d", deployment.Status.ReadyReplicas, desired),
		StatusKey: healthKey(healthy),
		Cells: []Cell{
			{Value: deployment.Name, Class: "primary"},
			{Value: deployment.Namespace},
			replicaSummaryCell(deployment.Status.ReadyReplicas, desired, healthKey(healthy), replicaDetail(deployment.Status.UpdatedReplicas, "upd"), replicaDetail(deployment.Status.AvailableReplicas, "avail")),
			resourceListCell(usage, corev1.ResourceCPU),
			containerResourceCell(deployment.Spec.Template.Spec.Containers, resourceRequests, corev1.ResourceCPU),
			containerResourceCell(deployment.Spec.Template.Spec.Containers, resourceLimits, corev1.ResourceCPU),
			resourceListCell(usage, corev1.ResourceMemory),
			containerResourceCell(deployment.Spec.Template.Spec.Containers, resourceRequests, corev1.ResourceMemory),
			containerResourceCell(deployment.Spec.Template.Spec.Containers, resourceLimits, corev1.ResourceMemory),
			ageCell(deployment.CreationTimestamp.Time),
		},
	}
}

func statefulSetRow(statefulSet appsv1.StatefulSet, usage corev1.ResourceList) Row {
	desired := statefulSet.Spec.Replicas
	if desired == nil {
		zero := int32(0)
		desired = &zero
	}
	healthy := statefulSet.Status.ReadyReplicas == *desired
	return Row{
		Name:      statefulSet.Name,
		Namespace: statefulSet.Namespace,
		Status:    fmt.Sprintf("%d/%d", statefulSet.Status.ReadyReplicas, *desired),
		StatusKey: healthKey(healthy),
		Cells: []Cell{
			{Value: statefulSet.Name, Class: "primary"},
			{Value: statefulSet.Namespace},
			replicaSummaryCell(statefulSet.Status.ReadyReplicas, *desired, healthKey(healthy), replicaDetail(statefulSet.Status.Replicas, "cur")),
			resourceListCell(usage, corev1.ResourceCPU),
			containerResourceCell(statefulSet.Spec.Template.Spec.Containers, resourceRequests, corev1.ResourceCPU),
			containerResourceCell(statefulSet.Spec.Template.Spec.Containers, resourceLimits, corev1.ResourceCPU),
			resourceListCell(usage, corev1.ResourceMemory),
			containerResourceCell(statefulSet.Spec.Template.Spec.Containers, resourceRequests, corev1.ResourceMemory),
			containerResourceCell(statefulSet.Spec.Template.Spec.Containers, resourceLimits, corev1.ResourceMemory),
			ageCell(statefulSet.CreationTimestamp.Time),
		},
	}
}

func daemonSetRow(daemonSet appsv1.DaemonSet, usage corev1.ResourceList) Row {
	healthy := daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled && daemonSet.Status.NumberAvailable == daemonSet.Status.DesiredNumberScheduled
	return Row{
		Name:      daemonSet.Name,
		Namespace: daemonSet.Namespace,
		Status:    fmt.Sprintf("%d/%d", daemonSet.Status.NumberReady, daemonSet.Status.DesiredNumberScheduled),
		StatusKey: healthKey(healthy),
		Cells: []Cell{
			{Value: daemonSet.Name, Class: "primary"},
			{Value: daemonSet.Namespace},
			replicaSummaryCell(daemonSet.Status.NumberReady, daemonSet.Status.DesiredNumberScheduled, healthKey(healthy), replicaDetail(daemonSet.Status.NumberAvailable, "avail")),
			resourceListCell(usage, corev1.ResourceCPU),
			containerResourceCell(daemonSet.Spec.Template.Spec.Containers, resourceRequests, corev1.ResourceCPU),
			containerResourceCell(daemonSet.Spec.Template.Spec.Containers, resourceLimits, corev1.ResourceCPU),
			resourceListCell(usage, corev1.ResourceMemory),
			containerResourceCell(daemonSet.Spec.Template.Spec.Containers, resourceRequests, corev1.ResourceMemory),
			containerResourceCell(daemonSet.Spec.Template.Spec.Containers, resourceLimits, corev1.ResourceMemory),
			ageCell(daemonSet.CreationTimestamp.Time),
		},
	}
}

func serviceRow(service corev1.Service) Row {
	return Row{
		Name:      service.Name,
		Namespace: service.Namespace,
		Status:    string(service.Spec.Type),
		StatusKey: "neutral",
		Cells: []Cell{
			{Value: service.Name, Class: "primary"},
			{Value: service.Namespace},
			{Value: string(service.Spec.Type)},
			{Value: service.Spec.ClusterIP},
			{Value: servicePorts(service.Spec.Ports)},
			ageCell(service.CreationTimestamp.Time),
		},
	}
}

func ingressRow(ingress networkingv1.Ingress) Row {
	className := ""
	if ingress.Spec.IngressClassName != nil {
		className = *ingress.Spec.IngressClassName
	}
	return Row{
		Name:      ingress.Name,
		Namespace: ingress.Namespace,
		Status:    "Ingress",
		StatusKey: "neutral",
		Cells: []Cell{
			{Value: ingress.Name, Class: "primary"},
			{Value: ingress.Namespace},
			{Value: className},
			{Value: ingressHosts(ingress.Spec.Rules)},
			{Value: ingressAddresses(ingress.Status.LoadBalancer.Ingress)},
			ageCell(ingress.CreationTimestamp.Time),
		},
	}
}

func nodeRow(node corev1.Node) Row {
	ready := false
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			ready = condition.Status == corev1.ConditionTrue
			break
		}
	}
	return Row{
		Name:      node.Name,
		Status:    mapBool(ready, "Ready", "NotReady"),
		StatusKey: healthKey(ready),
		Cells: []Cell{
			{Value: node.Name, Class: "primary"},
			{Value: mapBool(ready, "Ready", "NotReady"), Class: "status " + healthKey(ready)},
			{Value: nodeRoles(node.Labels)},
			{Value: node.Status.NodeInfo.KubeletVersion},
			{Value: nodeInternalIP(node.Status.Addresses)},
			ageCell(node.CreationTimestamp.Time),
		},
	}
}

func namespaceRow(namespace corev1.Namespace) Row {
	healthy := namespace.Status.Phase == corev1.NamespaceActive
	return Row{
		Name:      namespace.Name,
		Status:    string(namespace.Status.Phase),
		StatusKey: healthKey(healthy),
		Cells: []Cell{
			{Value: namespace.Name, Class: "primary"},
			{Value: string(namespace.Status.Phase), Class: "status " + healthKey(healthy)},
			ageCell(namespace.CreationTimestamp.Time),
		},
	}
}

func replicaSetRow(replicaSet appsv1.ReplicaSet) Row {
	desired := int32(0)
	if replicaSet.Spec.Replicas != nil {
		desired = *replicaSet.Spec.Replicas
	}
	healthy := replicaSet.Status.ReadyReplicas == desired
	return Row{
		Name:      replicaSet.Name,
		Namespace: replicaSet.Namespace,
		Status:    fmt.Sprintf("%d/%d", replicaSet.Status.ReadyReplicas, desired),
		StatusKey: healthKey(healthy),
		Cells: []Cell{
			{Value: replicaSet.Name, Class: "primary"},
			{Value: replicaSet.Namespace},
			replicaSummaryCell(replicaSet.Status.ReadyReplicas, desired, healthKey(healthy), replicaDetail(replicaSet.Status.Replicas, "cur")),
			ageCell(replicaSet.CreationTimestamp.Time),
		},
	}
}

func jobRow(job batchv1.Job) Row {
	desired := int32(1)
	if job.Spec.Completions != nil {
		desired = *job.Spec.Completions
	}
	healthy := job.Status.Succeeded >= desired
	status := fmt.Sprintf("%d/%d", job.Status.Succeeded, desired)
	return Row{
		Name:      job.Name,
		Namespace: job.Namespace,
		Status:    status,
		StatusKey: healthKey(healthy),
		Cells: []Cell{
			{Value: job.Name, Class: "primary"},
			{Value: job.Namespace},
			cellWithClassAndOptionalSort(status, "status "+healthKey(healthy), ratioSortValue(int64(job.Status.Succeeded), int64(desired))),
			cellWithSortValue(fmt.Sprint(job.Status.Succeeded), int64(job.Status.Succeeded)),
			cellWithSortValue(fmt.Sprint(job.Status.Failed), int64(job.Status.Failed)),
			ageCell(job.CreationTimestamp.Time),
		},
	}
}

func cronJobRow(cronJob batchv1.CronJob) Row {
	lastSchedule := ""
	if cronJob.Status.LastScheduleTime != nil {
		lastSchedule = age(cronJob.Status.LastScheduleTime.Time)
	}
	suspended := cronJob.Spec.Suspend != nil && *cronJob.Spec.Suspend
	return Row{
		Name:      cronJob.Name,
		Namespace: cronJob.Namespace,
		Status:    mapBool(suspended, "Suspended", "Active"),
		StatusKey: mapBool(suspended, "warn", "neutral"),
		Cells: []Cell{
			{Value: cronJob.Name, Class: "primary"},
			{Value: cronJob.Namespace},
			{Value: cronJob.Spec.Schedule},
			{Value: fmt.Sprint(suspended)},
			cellWithSortValue(fmt.Sprint(len(cronJob.Status.Active)), int64(len(cronJob.Status.Active))),
			{Value: lastSchedule},
			ageCell(cronJob.CreationTimestamp.Time),
		},
	}
}

func persistentVolumeClaimRow(pvc corev1.PersistentVolumeClaim) Row {
	storage := ""
	if quantity, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
		storage = quantity.String()
	}
	className := ""
	if pvc.Spec.StorageClassName != nil {
		className = *pvc.Spec.StorageClassName
	}
	healthy := pvc.Status.Phase == corev1.ClaimBound
	return Row{
		Name:      pvc.Name,
		Namespace: pvc.Namespace,
		Status:    string(pvc.Status.Phase),
		StatusKey: healthKey(healthy),
		Cells: []Cell{
			{Value: pvc.Name, Class: "primary"},
			{Value: pvc.Namespace},
			{Value: string(pvc.Status.Phase), Class: "status " + healthKey(healthy)},
			{Value: pvc.Spec.VolumeName},
			{Value: storage},
			{Value: className},
			ageCell(pvc.CreationTimestamp.Time),
		},
	}
}

func persistentVolumeRow(pv corev1.PersistentVolume) Row {
	storage := ""
	if quantity, ok := pv.Spec.Capacity[corev1.ResourceStorage]; ok {
		storage = quantity.String()
	}
	claim := ""
	if pv.Spec.ClaimRef != nil {
		claim = namespacedName(pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
	}
	healthy := pv.Status.Phase == corev1.VolumeBound || pv.Status.Phase == corev1.VolumeAvailable
	return Row{
		Name:      pv.Name,
		Status:    string(pv.Status.Phase),
		StatusKey: healthKey(healthy),
		Cells: []Cell{
			{Value: pv.Name, Class: "primary"},
			{Value: string(pv.Status.Phase), Class: "status " + healthKey(healthy)},
			{Value: storage},
			{Value: accessModes(pv.Spec.AccessModes)},
			{Value: string(pv.Spec.PersistentVolumeReclaimPolicy)},
			{Value: pv.Spec.StorageClassName},
			{Value: claim},
			ageCell(pv.CreationTimestamp.Time),
		},
	}
}

func storageClassRow(storageClass storagev1.StorageClass) Row {
	reclaimPolicy := ""
	if storageClass.ReclaimPolicy != nil {
		reclaimPolicy = string(*storageClass.ReclaimPolicy)
	}
	bindingMode := ""
	if storageClass.VolumeBindingMode != nil {
		bindingMode = string(*storageClass.VolumeBindingMode)
	}
	return Row{
		Name:      storageClass.Name,
		Status:    storageClass.Provisioner,
		StatusKey: "neutral",
		Cells: []Cell{
			{Value: storageClass.Name, Class: "primary"},
			{Value: storageClass.Provisioner},
			{Value: reclaimPolicy},
			{Value: bindingMode},
			{Value: fmt.Sprint(isDefaultStorageClass(storageClass.Annotations))},
			ageCell(storageClass.CreationTimestamp.Time),
		},
	}
}

func endpointsRow(endpoint corev1.Endpoints) Row {
	ready := 0
	notReady := 0
	ports := []string{}
	for _, subset := range endpoint.Subsets {
		ready += len(subset.Addresses)
		notReady += len(subset.NotReadyAddresses)
		for _, port := range subset.Ports {
			ports = append(ports, fmt.Sprintf("%d/%s", port.Port, port.Protocol))
		}
	}
	return Row{
		Name:      endpoint.Name,
		Namespace: endpoint.Namespace,
		Status:    fmt.Sprint(ready),
		StatusKey: "neutral",
		Cells: []Cell{
			{Value: endpoint.Name, Class: "primary"},
			{Value: endpoint.Namespace},
			cellWithSortValue(fmt.Sprint(ready), int64(ready)),
			cellWithSortValue(fmt.Sprint(notReady), int64(notReady)),
			{Value: strings.Join(uniqueStrings(ports), ", ")},
			ageCell(endpoint.CreationTimestamp.Time),
		},
	}
}

func endpointSliceRow(endpointSlice discoveryv1.EndpointSlice) Row {
	ports := []string{}
	for _, port := range endpointSlice.Ports {
		if port.Port == nil {
			continue
		}
		protocol := corev1.ProtocolTCP
		if port.Protocol != nil {
			protocol = *port.Protocol
		}
		ports = append(ports, fmt.Sprintf("%d/%s", *port.Port, protocol))
	}
	return Row{
		Name:      endpointSlice.Name,
		Namespace: endpointSlice.Namespace,
		Status:    fmt.Sprint(len(endpointSlice.Endpoints)),
		StatusKey: "neutral",
		Cells: []Cell{
			{Value: endpointSlice.Name, Class: "primary"},
			{Value: endpointSlice.Namespace},
			{Value: string(endpointSlice.AddressType)},
			cellWithSortValue(fmt.Sprint(len(endpointSlice.Endpoints)), int64(len(endpointSlice.Endpoints))),
			{Value: strings.Join(uniqueStrings(ports), ", ")},
			ageCell(endpointSlice.CreationTimestamp.Time),
		},
	}
}

func ingressClassRow(ingressClass networkingv1.IngressClass) Row {
	return Row{
		Name:      ingressClass.Name,
		Status:    ingressClass.Spec.Controller,
		StatusKey: "neutral",
		Cells: []Cell{
			{Value: ingressClass.Name, Class: "primary"},
			{Value: ingressClass.Spec.Controller},
			{Value: fmt.Sprint(ingressClass.Annotations["ingressclass.kubernetes.io/is-default-class"] == "true")},
			ageCell(ingressClass.CreationTimestamp.Time),
		},
	}
}

func networkPolicyRow(policy networkingv1.NetworkPolicy) Row {
	return Row{
		Name:      policy.Name,
		Namespace: policy.Namespace,
		Status:    networkPolicyTypes(policy.Spec.PolicyTypes),
		StatusKey: "neutral",
		Cells: []Cell{
			{Value: policy.Name, Class: "primary"},
			{Value: policy.Namespace},
			{Value: labelSelectorString(&policy.Spec.PodSelector)},
			{Value: networkPolicyTypes(policy.Spec.PolicyTypes)},
			ageCell(policy.CreationTimestamp.Time),
		},
	}
}

func serviceAccountRow(account corev1.ServiceAccount) Row {
	return Row{
		Name:      account.Name,
		Namespace: account.Namespace,
		Status:    "ServiceAccount",
		StatusKey: "neutral",
		Cells: []Cell{
			{Value: account.Name, Class: "primary"},
			{Value: account.Namespace},
			cellWithSortValue(fmt.Sprint(len(account.Secrets)), int64(len(account.Secrets))),
			cellWithSortValue(fmt.Sprint(len(account.ImagePullSecrets)), int64(len(account.ImagePullSecrets))),
			ageCell(account.CreationTimestamp.Time),
		},
	}
}

func roleRow(role rbacv1.Role) Row {
	return Row{
		Name:      role.Name,
		Namespace: role.Namespace,
		Status:    fmt.Sprint(len(role.Rules)),
		StatusKey: "neutral",
		Cells:     []Cell{{Value: role.Name, Class: "primary"}, {Value: role.Namespace}, cellWithSortValue(fmt.Sprint(len(role.Rules)), int64(len(role.Rules))), ageCell(role.CreationTimestamp.Time)},
	}
}

func clusterRoleRow(role rbacv1.ClusterRole) Row {
	return Row{
		Name:      role.Name,
		Status:    fmt.Sprint(len(role.Rules)),
		StatusKey: "neutral",
		Cells:     []Cell{{Value: role.Name, Class: "primary"}, cellWithSortValue(fmt.Sprint(len(role.Rules)), int64(len(role.Rules))), ageCell(role.CreationTimestamp.Time)},
	}
}

func roleBindingRow(binding rbacv1.RoleBinding) Row {
	return Row{
		Name:      binding.Name,
		Namespace: binding.Namespace,
		Status:    binding.RoleRef.Kind + "/" + binding.RoleRef.Name,
		StatusKey: "neutral",
		Cells: []Cell{
			{Value: binding.Name, Class: "primary"},
			{Value: binding.Namespace},
			{Value: binding.RoleRef.Kind + "/" + binding.RoleRef.Name},
			{Value: subjectsValue(binding.Subjects)},
			ageCell(binding.CreationTimestamp.Time),
		},
	}
}

func clusterRoleBindingRow(binding rbacv1.ClusterRoleBinding) Row {
	return Row{
		Name:      binding.Name,
		Status:    binding.RoleRef.Kind + "/" + binding.RoleRef.Name,
		StatusKey: "neutral",
		Cells: []Cell{
			{Value: binding.Name, Class: "primary"},
			{Value: binding.RoleRef.Kind + "/" + binding.RoleRef.Name},
			{Value: subjectsValue(binding.Subjects)},
			ageCell(binding.CreationTimestamp.Time),
		},
	}
}

func configMapRow(configMap corev1.ConfigMap) Row {
	total := len(configMap.Data) + len(configMap.BinaryData)
	return Row{Name: configMap.Name, Namespace: configMap.Namespace, Status: fmt.Sprint(total), StatusKey: "neutral", Cells: []Cell{{Value: configMap.Name, Class: "primary"}, {Value: configMap.Namespace}, cellWithSortValue(fmt.Sprint(total), int64(total)), ageCell(configMap.CreationTimestamp.Time)}}
}

func secretRow(secret corev1.Secret) Row {
	return Row{Name: secret.Name, Namespace: secret.Namespace, Status: string(secret.Type), StatusKey: "neutral", Cells: []Cell{{Value: secret.Name, Class: "primary"}, {Value: secret.Namespace}, {Value: string(secret.Type)}, cellWithSortValue(fmt.Sprint(len(secret.Data)), int64(len(secret.Data))), ageCell(secret.CreationTimestamp.Time)}}
}

func horizontalPodAutoscalerRow(hpa autoscalingv2.HorizontalPodAutoscaler) Row {
	minReplicas := int32(1)
	if hpa.Spec.MinReplicas != nil {
		minReplicas = *hpa.Spec.MinReplicas
	}
	return Row{
		Name:      hpa.Name,
		Namespace: hpa.Namespace,
		Status:    fmt.Sprintf("%d/%d", hpa.Status.CurrentReplicas, hpa.Spec.MaxReplicas),
		StatusKey: "neutral",
		Cells: []Cell{
			{Value: hpa.Name, Class: "primary"},
			{Value: hpa.Namespace},
			{Value: hpa.Spec.ScaleTargetRef.Kind + "/" + hpa.Spec.ScaleTargetRef.Name},
			cellWithSortValue(fmt.Sprint(minReplicas), int64(minReplicas)),
			cellWithSortValue(fmt.Sprint(hpa.Spec.MaxReplicas), int64(hpa.Spec.MaxReplicas)),
			cellWithSortValue(fmt.Sprint(hpa.Status.CurrentReplicas), int64(hpa.Status.CurrentReplicas)),
			ageCell(hpa.CreationTimestamp.Time),
		},
	}
}

func podDisruptionBudgetRow(pdb policyv1.PodDisruptionBudget) Row {
	minAvailable := intOrStringValue(pdb.Spec.MinAvailable)
	maxUnavailable := intOrStringValue(pdb.Spec.MaxUnavailable)
	return Row{
		Name:      pdb.Name,
		Namespace: pdb.Namespace,
		Status:    fmt.Sprint(pdb.Status.DisruptionsAllowed),
		StatusKey: "neutral",
		Cells: []Cell{
			{Value: pdb.Name, Class: "primary"},
			{Value: pdb.Namespace},
			{Value: minAvailable},
			{Value: maxUnavailable},
			cellWithSortValue(fmt.Sprint(pdb.Status.DisruptionsAllowed), int64(pdb.Status.DisruptionsAllowed)),
			ageCell(pdb.CreationTimestamp.Time),
		},
	}
}

func resourceQuotaRow(quota corev1.ResourceQuota) Row {
	return Row{Name: quota.Name, Namespace: quota.Namespace, Status: fmt.Sprint(len(quota.Status.Hard)), StatusKey: "neutral", Cells: []Cell{{Value: quota.Name, Class: "primary"}, {Value: quota.Namespace}, {Value: resourceListSummary(quota.Status.Hard)}, {Value: resourceListSummary(quota.Status.Used)}, ageCell(quota.CreationTimestamp.Time)}}
}

func limitRangeRow(limitRange corev1.LimitRange) Row {
	return Row{Name: limitRange.Name, Namespace: limitRange.Namespace, Status: fmt.Sprint(len(limitRange.Spec.Limits)), StatusKey: "neutral", Cells: []Cell{{Value: limitRange.Name, Class: "primary"}, {Value: limitRange.Namespace}, cellWithSortValue(fmt.Sprint(len(limitRange.Spec.Limits)), int64(len(limitRange.Spec.Limits))), ageCell(limitRange.CreationTimestamp.Time)}}
}

func priorityClassRow(priorityClass schedulingv1.PriorityClass) Row {
	return Row{Name: priorityClass.Name, Status: fmt.Sprint(priorityClass.Value), StatusKey: "neutral", Cells: []Cell{{Value: priorityClass.Name, Class: "primary"}, cellWithSortValue(fmt.Sprint(priorityClass.Value), int64(priorityClass.Value)), {Value: fmt.Sprint(priorityClass.GlobalDefault)}, {Value: preemptionPolicyValue(priorityClass.PreemptionPolicy)}, ageCell(priorityClass.CreationTimestamp.Time)}}
}

func runtimeClassRow(runtimeClass nodev1.RuntimeClass) Row {
	return Row{Name: runtimeClass.Name, Status: runtimeClass.Handler, StatusKey: "neutral", Cells: []Cell{{Value: runtimeClass.Name, Class: "primary"}, {Value: runtimeClass.Handler}, ageCell(runtimeClass.CreationTimestamp.Time)}}
}

func leaseRow(lease coordinationv1.Lease) Row {
	holder := ""
	if lease.Spec.HolderIdentity != nil {
		holder = *lease.Spec.HolderIdentity
	}
	renewTime := ""
	if lease.Spec.RenewTime != nil {
		renewTime = age(lease.Spec.RenewTime.Time)
	}
	return Row{Name: lease.Name, Namespace: lease.Namespace, Status: holder, StatusKey: "neutral", Cells: []Cell{{Value: lease.Name, Class: "primary"}, {Value: lease.Namespace}, {Value: holder}, {Value: renewTime}, ageCell(lease.CreationTimestamp.Time)}}
}

func mutatingWebhookConfigurationRow(config admissionv1.MutatingWebhookConfiguration) Row {
	return Row{Name: config.Name, Status: fmt.Sprint(len(config.Webhooks)), StatusKey: "neutral", Cells: []Cell{{Value: config.Name, Class: "primary"}, cellWithSortValue(fmt.Sprint(len(config.Webhooks)), int64(len(config.Webhooks))), ageCell(config.CreationTimestamp.Time)}}
}

func validatingWebhookConfigurationRow(config admissionv1.ValidatingWebhookConfiguration) Row {
	return Row{Name: config.Name, Status: fmt.Sprint(len(config.Webhooks)), StatusKey: "neutral", Cells: []Cell{{Value: config.Name, Class: "primary"}, cellWithSortValue(fmt.Sprint(len(config.Webhooks)), int64(len(config.Webhooks))), ageCell(config.CreationTimestamp.Time)}}
}

func eventRow(event corev1.Event) Row {
	object := event.InvolvedObject.Kind
	if event.InvolvedObject.Name != "" {
		object += "/" + event.InvolvedObject.Name
	}
	lastSeen := eventTimestamp(event)
	return Row{
		Name:      event.Name,
		Namespace: event.Namespace,
		Status:    event.Type,
		StatusKey: mapBool(strings.EqualFold(event.Type, corev1.EventTypeWarning), "warn", "neutral"),
		Cells: []Cell{
			{Value: event.Name, Class: "primary"},
			{Value: event.Namespace},
			{Value: event.Type, Class: "status " + mapBool(strings.EqualFold(event.Type, corev1.EventTypeWarning), "warn", "neutral")},
			{Value: event.Reason},
			{Value: object},
			{Value: event.Message},
			ageCell(lastSeen),
		},
	}
}

func matches(row Row, query string) bool {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return true
	}
	for _, cell := range row.Cells {
		if strings.Contains(strings.ToLower(cell.Value), query) {
			return true
		}
	}
	return false
}

func FilterTableRows(table *Table, query string) {
	table.Query = query
	if strings.TrimSpace(query) == "" {
		return
	}
	rows := table.Rows[:0]
	for _, row := range table.Rows {
		if matches(row, query) {
			rows = append(rows, row)
		}
	}
	table.Rows = rows
}

func SortTableRows(table *Table, sortColumn, sortOrder string) {
	sortTableRows(table, sortColumn, sortOrder)
}

func sortTableRows(table *Table, sortColumn, sortOrder string) {
	sortColumn, sortOrder = normalizeSort(table.Columns, sortColumn, sortOrder)
	table.SortColumn = sortColumn
	table.SortOrder = sortOrder

	if sortColumn == "" || sortOrder == "" {
		sortRows(table.Rows, table.Namespaced)
		return
	}

	columnIndex := sortColumnIndex(table.Columns, sortColumn)
	sort.SliceStable(table.Rows, func(i, j int) bool {
		cmp := compareCellValues(rowCell(table.Rows[i], columnIndex), rowCell(table.Rows[j], columnIndex))
		if cmp == 0 {
			return compareDefaultRows(table.Rows[i], table.Rows[j], table.Namespaced) < 0
		}
		if sortOrder == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})
}

func normalizeSort(columns []string, sortColumn, sortOrder string) (string, string) {
	if sortOrder != "asc" && sortOrder != "desc" {
		return "", ""
	}
	if sortColumnIndex(columns, sortColumn) == -1 {
		return "", ""
	}
	return sortColumn, sortOrder
}

func sortColumnIndex(columns []string, sortColumn string) int {
	for index, column := range columns {
		if column == sortColumn {
			return index
		}
	}
	return -1
}

func sortRows(rows []Row, namespaced bool) {
	sort.SliceStable(rows, func(i, j int) bool {
		return compareDefaultRows(rows[i], rows[j], namespaced) < 0
	})
}

func compareDefaultRows(left, right Row, namespaced bool) int {
	leftKey := strings.ToLower(strings.Join(rowSortKey(left, namespaced), "/"))
	rightKey := strings.ToLower(strings.Join(rowSortKey(right, namespaced), "/"))
	return strings.Compare(leftKey, rightKey)
}

func rowSortKey(row Row, namespaced bool) []string {
	key := []string{}
	if row.Cluster != "" {
		key = append(key, row.Cluster)
	}
	if namespaced && len(row.Cells) > 1 {
		namespaceIndex := 1
		if row.Cluster != "" {
			namespaceIndex = 2
		}
		if len(row.Cells) > namespaceIndex {
			key = append(key, row.Cells[namespaceIndex].Value)
		}
	}
	key = append(key, row.Name)
	return key
}

func rowCellValue(row Row, columnIndex int) string {
	return rowCell(row, columnIndex).Value
}

func rowCell(row Row, columnIndex int) Cell {
	if columnIndex < 0 || columnIndex >= len(row.Cells) {
		return Cell{}
	}
	return row.Cells[columnIndex]
}

func compareCellValues(left, right Cell) int {
	if left.SortValue != nil && right.SortValue != nil {
		switch {
		case *left.SortValue < *right.SortValue:
			return -1
		case *left.SortValue > *right.SortValue:
			return 1
		default:
			return 0
		}
	}
	return strings.Compare(strings.ToLower(strings.TrimSpace(left.Value)), strings.ToLower(strings.TrimSpace(right.Value)))
}

func healthKey(ok bool) string {
	if ok {
		return "good"
	}
	return "warn"
}

func mapBool(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

type resourceSelector func(corev1.ResourceRequirements) corev1.ResourceList

func resourceRequests(resources corev1.ResourceRequirements) corev1.ResourceList {
	return resources.Requests
}

func resourceLimits(resources corev1.ResourceRequirements) corev1.ResourceList {
	return resources.Limits
}

func cellWithSortValue(value string, sortValue int64) Cell {
	return cellWithOptionalSort(value, ptrInt64(sortValue))
}

func cellWithClassAndSortValue(value, class string, sortValue int64) Cell {
	return cellWithClassAndOptionalSort(value, class, ptrInt64(sortValue))
}

func cellWithOptionalSort(value string, sortValue *int64) Cell {
	return Cell{Value: value, SortValue: sortValue}
}

func cellWithClassAndOptionalSort(value, class string, sortValue *int64) Cell {
	return Cell{Value: value, Class: class, SortValue: sortValue}
}

type replicaSummaryDetail struct {
	value int32
	label string
}

func replicaDetail(value int32, label string) replicaSummaryDetail {
	return replicaSummaryDetail{value: value, label: label}
}

func replicaSummaryCell(ready, desired int32, statusKey string, details ...replicaSummaryDetail) Cell {
	parts := []string{fmt.Sprintf("%d/%d ready", ready, desired)}
	for _, detail := range details {
		if detail.value != desired {
			parts = append(parts, fmt.Sprintf("%s %d", detail.label, detail.value))
		}
	}
	return cellWithClassAndOptionalSort(strings.Join(parts, " · "), "status "+statusKey, ratioSortValue(int64(ready), int64(desired)))
}

func ptrInt64(value int64) *int64 {
	return &value
}

func ratioSortValue(numerator, denominator int64) *int64 {
	if denominator <= 0 {
		return nil
	}
	const scale = 1_000_000
	return ptrInt64(numerator * scale / denominator)
}

func ageCell(t time.Time) Cell {
	value := age(t)
	if t.IsZero() {
		return Cell{Value: value}
	}
	return cellWithSortValue(value, int64(time.Since(t).Seconds()))
}

func containerResourceValue(containers []corev1.Container, selector resourceSelector, name corev1.ResourceName) string {
	return containerResourceCell(containers, selector, name).Value
}

func containerResourceCell(containers []corev1.Container, selector resourceSelector, name corev1.ResourceName) Cell {
	var total resource.Quantity
	for _, container := range containers {
		quantity, ok := selector(container.Resources)[name]
		if ok {
			total.Add(quantity)
		}
	}
	return resourceQuantityCell(name, total)
}

func resourceListValue(values corev1.ResourceList, name corev1.ResourceName) string {
	return resourceListCell(values, name).Value
}

func resourceListCell(values corev1.ResourceList, name corev1.ResourceName) Cell {
	quantity, ok := values[name]
	if !ok {
		return Cell{Value: "-"}
	}
	return resourceQuantityCell(name, quantity)
}

func resourceQuantityValue(name corev1.ResourceName, quantity resource.Quantity) string {
	return resourceQuantityCell(name, quantity).Value
}

func resourceQuantityCell(name corev1.ResourceName, quantity resource.Quantity) Cell {
	if quantity.Sign() == 0 {
		return Cell{Value: "-"}
	}
	switch name {
	case corev1.ResourceCPU:
		return cellWithSortValue(cpuValue(quantity), quantity.MilliValue())
	case corev1.ResourceMemory:
		bytes := quantity.Value()
		return cellWithSortValue(byteValue(bytes), bytes)
	default:
		return Cell{Value: quantity.String()}
	}
}

func cpuValue(quantity resource.Quantity) string {
	milli := quantity.MilliValue()
	if milli%1000 == 0 {
		return fmt.Sprintf("%d", milli/1000)
	}
	return fmt.Sprintf("%dm", milli)
}

func byteValue(bytes int64) string {
	if bytes == 0 {
		return "0"
	}

	units := []struct {
		name  string
		value int64
	}{
		{"Ei", 1 << 60},
		{"Pi", 1 << 50},
		{"Ti", 1 << 40},
		{"Gi", 1 << 30},
		{"Mi", 1 << 20},
		{"Ki", 1 << 10},
	}
	for _, unit := range units {
		if bytes >= unit.value {
			if bytes%unit.value == 0 {
				return fmt.Sprintf("%d%s", bytes/unit.value, unit.name)
			}
			return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", float64(bytes)/float64(unit.value)), "0"), ".") + unit.name
		}
	}
	return fmt.Sprintf("%dB", bytes)
}

func age(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	if d < 0 {
		return "just now"
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		minutes := int(d.Minutes())
		if minutes == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", minutes)
	case d < 48*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}

func servicePorts(ports []corev1.ServicePort) string {
	out := make([]string, 0, len(ports))
	for _, port := range ports {
		value := fmt.Sprintf("%d/%s", port.Port, port.Protocol)
		if port.NodePort != 0 {
			value += fmt.Sprintf(":%d", port.NodePort)
		}
		out = append(out, value)
	}
	return strings.Join(out, ", ")
}

func ingressHosts(rules []networkingv1.IngressRule) string {
	hosts := make([]string, 0, len(rules))
	for _, rule := range rules {
		if rule.Host != "" {
			hosts = append(hosts, rule.Host)
		}
	}
	return strings.Join(hosts, ", ")
}

func ingressAddresses(entries []networkingv1.IngressLoadBalancerIngress) string {
	addresses := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Hostname != "" {
			addresses = append(addresses, entry.Hostname)
		}
		if entry.IP != "" {
			addresses = append(addresses, entry.IP)
		}
	}
	return strings.Join(addresses, ", ")
}

func nodeRoles(labels map[string]string) string {
	roles := make([]string, 0)
	for key := range labels {
		const prefix = "node-role.kubernetes.io/"
		if strings.HasPrefix(key, prefix) {
			role := strings.TrimPrefix(key, prefix)
			if role == "" {
				role = "control-plane"
			}
			roles = append(roles, role)
		}
	}
	sort.Strings(roles)
	if len(roles) == 0 {
		return "worker"
	}
	return strings.Join(roles, ", ")
}

func nodeInternalIP(addresses []corev1.NodeAddress) string {
	for _, address := range addresses {
		if address.Type == corev1.NodeInternalIP {
			return address.Address
		}
	}
	return ""
}

func namespacedName(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "/" + name
}

func accessModes(modes []corev1.PersistentVolumeAccessMode) string {
	values := make([]string, 0, len(modes))
	for _, mode := range modes {
		values = append(values, string(mode))
	}
	return strings.Join(values, ", ")
}

func isDefaultStorageClass(annotations map[string]string) bool {
	return annotations["storageclass.kubernetes.io/is-default-class"] == "true" ||
		annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true"
}

func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func networkPolicyTypes(types []networkingv1.PolicyType) string {
	values := make([]string, 0, len(types))
	for _, value := range types {
		values = append(values, string(value))
	}
	return strings.Join(values, ", ")
}

func labelSelectorString(selector *metav1.LabelSelector) string {
	if selector == nil {
		return ""
	}
	parts := make([]string, 0, len(selector.MatchLabels)+len(selector.MatchExpressions))
	for key, value := range selector.MatchLabels {
		parts = append(parts, key+"="+value)
	}
	for _, expression := range selector.MatchExpressions {
		value := expression.Key + " " + string(expression.Operator)
		if len(expression.Values) > 0 {
			value += " " + strings.Join(expression.Values, ",")
		}
		parts = append(parts, value)
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func subjectsValue(subjects []rbacv1.Subject) string {
	values := make([]string, 0, len(subjects))
	for _, subject := range subjects {
		name := subject.Kind + "/" + subject.Name
		if subject.Namespace != "" {
			name = subject.Kind + "/" + subject.Namespace + "/" + subject.Name
		}
		values = append(values, name)
	}
	sort.Strings(values)
	return strings.Join(values, ", ")
}

func intOrStringValue(value *intstr.IntOrString) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func resourceListSummary(values corev1.ResourceList) string {
	fields := resourceListFields(values)
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, field.Name+"="+field.Value)
	}
	return strings.Join(parts, ", ")
}

func preemptionPolicyValue(value *corev1.PreemptionPolicy) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
