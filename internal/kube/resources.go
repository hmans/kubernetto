package kube

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ResourceKind string

const (
	KindOverview    ResourceKind = "overview"
	KindPods        ResourceKind = "pods"
	KindDeployments ResourceKind = "deployments"
	KindStatefulSet ResourceKind = "statefulsets"
	KindDaemonSet   ResourceKind = "daemonsets"
	KindServices    ResourceKind = "services"
	KindIngresses   ResourceKind = "ingresses"
	KindNodes       ResourceKind = "nodes"
	KindNamespaces  ResourceKind = "namespaces"
)

type ResourceDef struct {
	Kind  ResourceKind
	Label string
	Scope string
}

var ResourceDefs = []ResourceDef{
	{Kind: KindOverview, Label: "Overview", Scope: "cluster"},
	{Kind: KindPods, Label: "Pods", Scope: "namespaced"},
	{Kind: KindDeployments, Label: "Deployments", Scope: "namespaced"},
	{Kind: KindStatefulSet, Label: "StatefulSets", Scope: "namespaced"},
	{Kind: KindDaemonSet, Label: "DaemonSets", Scope: "namespaced"},
	{Kind: KindServices, Label: "Services", Scope: "namespaced"},
	{Kind: KindIngresses, Label: "Ingresses", Scope: "namespaced"},
	{Kind: KindNodes, Label: "Nodes", Scope: "cluster"},
	{Kind: KindNamespaces, Label: "Namespaces", Scope: "cluster"},
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
		table.Columns = []string{"Name", "Namespace", "Ready", "Up-to-date", "Available", "CPU", "CPU Req", "CPU Limit", "MEM", "MEM Req", "MEM Limit", "Age"}
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
		table.Columns = []string{"Name", "Namespace", "Ready", "Replicas", "CPU", "CPU Req", "CPU Limit", "MEM", "MEM Req", "MEM Limit", "Age"}
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
		table.Columns = []string{"Name", "Namespace", "Desired", "Ready", "Available", "CPU", "CPU Req", "CPU Limit", "MEM", "MEM Req", "MEM Limit", "Age"}
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
	healthy := deployment.Status.ReadyReplicas == desired && deployment.Status.UpdatedReplicas == desired
	return Row{
		Name:      deployment.Name,
		Namespace: deployment.Namespace,
		Status:    fmt.Sprintf("%d/%d", deployment.Status.ReadyReplicas, desired),
		StatusKey: healthKey(healthy),
		Cells: []Cell{
			{Value: deployment.Name, Class: "primary"},
			{Value: deployment.Namespace},
			cellWithClassAndOptionalSort(fmt.Sprintf("%d/%d", deployment.Status.ReadyReplicas, desired), "status "+healthKey(healthy), ratioSortValue(int64(deployment.Status.ReadyReplicas), int64(desired))),
			cellWithSortValue(fmt.Sprint(deployment.Status.UpdatedReplicas), int64(deployment.Status.UpdatedReplicas)),
			cellWithSortValue(fmt.Sprint(deployment.Status.AvailableReplicas), int64(deployment.Status.AvailableReplicas)),
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
			cellWithClassAndOptionalSort(fmt.Sprintf("%d/%d", statefulSet.Status.ReadyReplicas, *desired), "status "+healthKey(healthy), ratioSortValue(int64(statefulSet.Status.ReadyReplicas), int64(*desired))),
			cellWithSortValue(fmt.Sprint(statefulSet.Status.Replicas), int64(statefulSet.Status.Replicas)),
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
	healthy := daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled
	return Row{
		Name:      daemonSet.Name,
		Namespace: daemonSet.Namespace,
		Status:    fmt.Sprintf("%d/%d", daemonSet.Status.NumberReady, daemonSet.Status.DesiredNumberScheduled),
		StatusKey: healthKey(healthy),
		Cells: []Cell{
			{Value: daemonSet.Name, Class: "primary"},
			{Value: daemonSet.Namespace},
			cellWithSortValue(fmt.Sprint(daemonSet.Status.DesiredNumberScheduled), int64(daemonSet.Status.DesiredNumberScheduled)),
			cellWithClassAndSortValue(fmt.Sprint(daemonSet.Status.NumberReady), "status "+healthKey(healthy), int64(daemonSet.Status.NumberReady)),
			cellWithSortValue(fmt.Sprint(daemonSet.Status.NumberAvailable), int64(daemonSet.Status.NumberAvailable)),
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
	if namespaced && len(row.Cells) > 1 {
		return []string{row.Cells[1].Value, row.Name}
	}
	return []string{row.Name}
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
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
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
