package ui

//go:generate go run github.com/a-h/templ/cmd/templ generate

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"kubernetto/internal/kube"
)

type PageState struct {
	Cluster        *kube.Cluster
	Clusters       []*kube.Cluster
	ActiveContexts []string
	Resources      []kube.ResourceDef
	QuickItems     []QuickSwitcherItem
	Signals        Signals
	Summary        kube.Summary
	Fleet          FleetOverview
	Actions        ActionList
	Overview       kube.ClusterOverview
	Table          kube.Table
	Detail         kube.ResourceDetail
	Namespaces     []string
	NamespaceErr   string
}

type FleetOverview struct {
	UpdatedAt     time.Time
	Clusters      []FleetCluster
	WarningEvents []FleetWarningEvent
}

type FleetCluster struct {
	Context      string
	Name         string
	StatusKey    string
	Detail       string
	TopConcern   string
	IssueKind    kube.ResourceKind
	IssueQuery   string
	IssueLabel   string
	Nodes        kube.OverviewMetric
	Pods         kube.OverviewMetric
	Workloads    kube.OverviewMetric
	CPU          kube.OverviewMetric
	Memory       kube.OverviewMetric
	WarningCount int
}

type FleetWarningEvent struct {
	Cluster string
	Event   kube.OverviewEvent
}

type ActionList struct {
	Error     string
	UpdatedAt time.Time
	Items     []ActionItem
}

type ActionItem struct {
	Context         string
	Title           string
	Detail          string
	Message         string
	Namespace       string
	Object          string
	Reason          string
	Age             string
	Count           int32
	StatusKey       string
	TargetKind      kube.ResourceKind
	TargetQuery     string
	TargetName      string
	TargetNamespace string
	ActionLabel     string
	LastSeen        time.Time
}

type QuickSwitcherItem struct {
	Label             string
	Meta              string
	MetaTokens        []QuickSwitcherMetaToken
	KindLabel         string
	Icon              string
	Search            string
	Context           string
	Clusters          string
	Resource          string
	ResourceScope     string
	Namespace         string
	Query             string
	SortColumn        string
	SortOrder         string
	SelectedName      string
	SelectedNamespace string
	DetailMode        string
	Endpoint          string
}

type QuickSwitcherMetaToken struct {
	Label string
	Icon  string
}

type ResourceNavGroup struct {
	ID        string
	Label     string
	Default   kube.ResourceDef
	Resources []kube.ResourceDef
}

type Signals struct {
	Context           string `json:"context"`
	Clusters          string `json:"clusters"`
	Resource          string `json:"resource"`
	Namespace         string `json:"namespace"`
	Query             string `json:"query"`
	SortColumn        string `json:"sortColumn"`
	SortOrder         string `json:"sortOrder"`
	SelectedName      string `json:"selectedName"`
	SelectedNamespace string `json:"selectedNamespace"`
	DetailMode        string `json:"detailMode"`
}

func RenderPage(w io.Writer, state PageState) error {
	return Page(state).Render(context.Background(), w)
}

func RenderFragment(component templ.Component) string {
	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		return fmt.Sprintf(`<div class="notice danger">render error: %s</div>`, html.EscapeString(err.Error()))
	}
	return buf.String()
}

func formatClock(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format("15:04:05")
}

func checked(a, b string) string {
	if a == b {
		return "true"
	}
	return "false"
}

func checkedBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func lower(s string) string {
	return strings.ToLower(s)
}

func hasClass(classes, class string) bool {
	for _, value := range strings.Fields(classes) {
		if value == class {
			return true
		}
	}
	return false
}

func hasDetail(state PageState) bool {
	return state.Detail.Name != ""
}

func isOverview(state PageState) bool {
	return state.Signals.Resource == string(kube.KindOverview)
}

func isActionItems(state PageState) bool {
	return state.Signals.Resource == string(kube.KindActions)
}

func isMap(state PageState) bool {
	return state.Signals.Resource == string(kube.KindMap)
}

func isStandalonePage(state PageState) bool {
	return isOverview(state) || isMap(state) || isActionItems(state)
}

func pageTitle(state PageState) string {
	if isOverview(state) {
		return "Cluster Dashboard"
	}
	if isMap(state) {
		return "Map"
	}
	if isActionItems(state) {
		return "Issues"
	}
	return state.Table.Label
}

func contentGridClass(state PageState) string {
	if hasDetail(state) {
		return "content-grid has-detail"
	}
	return "content-grid"
}

func appSignalAttrs(state PageState) templ.Attributes {
	attrs := templ.Attributes{
		"data-signals:context":            signalLiteral(state.Signals.Context),
		"data-signals:clusters":           signalLiteral(state.Signals.Clusters),
		"data-signals:resource":           signalLiteral(state.Signals.Resource),
		"data-signals:namespace":          signalLiteral(state.Signals.Namespace),
		"data-signals:query":              signalLiteral(state.Signals.Query),
		"data-signals:sort-column":        signalLiteral(state.Signals.SortColumn),
		"data-signals:sort-order":         signalLiteral(state.Signals.SortOrder),
		"data-signals:selected-name":      signalLiteral(state.Signals.SelectedName),
		"data-signals:selected-namespace": signalLiteral(state.Signals.SelectedNamespace),
		"data-signals:detail-mode":        signalLiteral(state.Signals.DetailMode),
		"data-signals:loading":            "false",
	}
	if isOverview(state) {
		attrs["data-on-interval__duration.5s"] = "@get('/ui/summary')"
	}
	return attrs
}

func tableAutoRefreshAttrs(state PageState) templ.Attributes {
	attrs := templ.Attributes{
		"data-class:pending": "$loading",
	}
	if isOverview(state) {
		return attrs
	}
	if isMap(state) {
		return attrs
	}
	if isActionItems(state) {
		attrs["data-on-interval__duration.10s"] = "@get('/ui/table?refresh=auto')"
		return attrs
	}
	attrs["data-on-interval__duration.5s"] = "@get('/ui/table?refresh=auto')"
	return attrs
}

func resourceButtonAttrs(def kube.ResourceDef) templ.Attributes {
	return templ.Attributes{
		"data-indicator:loading": true,
		"data-resource-kind":     string(def.Kind),
		"data-resource-scope":    def.Scope,
		"data-on:click":          "$resource = " + signalLiteral(string(def.Kind)) + "; $sortColumn = ''; $sortOrder = ''; $selectedName = ''; $selectedNamespace = ''; $detailMode = 'overview'; @get('/ui/table')",
	}
}

func resourceButtonIconClass(kind kube.ResourceKind) string {
	switch kind {
	case kube.KindOverview:
		return "icon-[uil--dashboard]"
	case kube.KindMap:
		return "icon-[lucide--map]"
	case kube.KindActions:
		return "icon-[lucide--list-checks]"
	case kube.KindPods:
		return "icon-[uil--cube]"
	case kube.KindDeployments:
		return "icon-[uil--rocket]"
	case kube.KindStatefulSet:
		return "icon-[uil--layers]"
	case kube.KindDaemonSet:
		return "icon-[uil--layer-group]"
	case kube.KindReplicaSets:
		return "icon-[uil--copy]"
	case kube.KindJobs:
		return "icon-[uil--check-circle]"
	case kube.KindCronJobs:
		return "icon-[uil--clock]"
	case kube.KindPersistentVolumeClaims:
		return "icon-[uil--database]"
	case kube.KindPersistentVolumes:
		return "icon-[uil--server]"
	case kube.KindStorageClasses:
		return "icon-[uil--archive]"
	case kube.KindServices:
		return "icon-[uil--server-alt]"
	case kube.KindEndpoints:
		return "icon-[uil--sitemap]"
	case kube.KindEndpointSlices:
		return "icon-[uil--share-alt]"
	case kube.KindIngresses:
		return "icon-[uil--globe]"
	case kube.KindIngressClasses:
		return "icon-[uil--compass]"
	case kube.KindNetworkPolicies:
		return "icon-[uil--shield]"
	case kube.KindServiceAccounts:
		return "icon-[uil--user]"
	case kube.KindRoles:
		return "icon-[uil--key-skeleton]"
	case kube.KindRoleBindings:
		return "icon-[uil--link]"
	case kube.KindClusterRoles:
		return "icon-[uil--keyhole-circle]"
	case kube.KindClusterRoleBindings:
		return "icon-[uil--link-h]"
	case kube.KindConfigMaps:
		return "icon-[uil--setting]"
	case kube.KindSecrets:
		return "icon-[uil--lock]"
	case kube.KindHorizontalPodAutoscalers:
		return "icon-[uil--arrows-resize-h]"
	case kube.KindPodDisruptionBudgets:
		return "icon-[uil--shield-exclamation]"
	case kube.KindResourceQuotas:
		return "icon-[uil--chart-pie]"
	case kube.KindLimitRanges:
		return "icon-[uil--sliders-v]"
	case kube.KindPriorityClasses:
		return "icon-[uil--arrow-up]"
	case kube.KindRuntimeClasses:
		return "icon-[uil--processor]"
	case kube.KindLeases:
		return "icon-[uil--file-contract]"
	case kube.KindMutatingWebhookConfigurations, kube.KindValidatingWebhookConfigurations:
		return "icon-[uil--web-grid]"
	case kube.KindNodes:
		return "icon-[uil--server-network]"
	case kube.KindNamespaces:
		return "icon-[uil--folder-network]"
	case kube.KindEvents:
		return "icon-[uil--bolt]"
	default:
		return "icon-[uil--servers]"
	}
}

func ResourceIconClass(kind kube.ResourceKind) string {
	return resourceButtonIconClass(kind)
}

func resourceGroupIconClass(group ResourceNavGroup) string {
	switch group.ID {
	case "overview":
		return "icon-[uil--dashboard]"
	case "workloads":
		return "icon-[uil--clock]"
	case "storage":
		return "icon-[uil--database]"
	case "network":
		return "icon-[uil--desktop]"
	case "security":
		return "icon-[uil--lock]"
	case "configuration":
		return "icon-[uil--setting]"
	case "cluster":
		return "icon-[uil--server-network]"
	case "custom":
		return "icon-[lucide--blocks]"
	default:
		return resourceButtonIconClass(group.Default.Kind)
	}
}

func resourceNavGroups(resources []kube.ResourceDef) []ResourceNavGroup {
	byKind := make(map[kube.ResourceKind]kube.ResourceDef, len(resources))
	for _, resource := range resources {
		byKind[resource.Kind] = resource
	}
	groups := make([]ResourceNavGroup, 0, len(kube.ResourceGroups))
	for _, group := range kube.ResourceGroups {
		navGroup := ResourceNavGroup{ID: group.ID, Label: group.Label, Default: byKind[group.DefaultKind]}
		for _, kind := range group.Kinds {
			if resource, ok := byKind[kind]; ok {
				navGroup.Resources = append(navGroup.Resources, resource)
				if resource.Default || navGroup.Default.Kind == "" {
					navGroup.Default = resource
				}
			}
		}
		if navGroup.Default.Kind != "" {
			groups = append(groups, navGroup)
		}
	}
	customGroup := ResourceNavGroup{ID: "custom", Label: "Custom Resources"}
	for _, resource := range resources {
		if resource.Custom || resource.Group == "custom" {
			customGroup.Resources = append(customGroup.Resources, resource)
			if customGroup.Default.Kind == "" {
				customGroup.Default = resource
			}
		}
	}
	if customGroup.Default.Kind != "" {
		groups = append(groups, customGroup)
	}
	return groups
}

func resourceGroupActive(group ResourceNavGroup, activeResource string) bool {
	for _, resource := range group.Resources {
		if string(resource.Kind) == activeResource {
			return true
		}
	}
	return false
}

func resourceGroupChildren(group ResourceNavGroup) []kube.ResourceDef {
	if group.ID == "overview" && len(group.Resources) <= 1 {
		return nil
	}
	return group.Resources
}

func overviewResourceLinkAttrs(kind kube.ResourceKind) templ.Attributes {
	def := resourceDef(kind)
	return templ.Attributes{
		"type":                   "button",
		"class":                  "overview-link",
		"data-indicator:loading": true,
		"data-resource-kind":     string(def.Kind),
		"data-resource-scope":    def.Scope,
		"data-on:click":          "$resource = " + signalLiteral(string(def.Kind)) + "; $sortColumn = ''; $sortOrder = ''; $selectedName = ''; $selectedNamespace = ''; $detailMode = 'overview'; @get('/ui/table')",
	}
}

func overviewRefreshAttrs() templ.Attributes {
	return templ.Attributes{
		"type":                   "button",
		"data-indicator:loading": true,
		"data-on:click":          "@get('/ui/refresh')",
	}
}

func mapClusterContext(state PageState) string {
	if state.Cluster == nil {
		return ""
	}
	return state.Cluster.ContextName
}

func mapClusterName(state PageState) string {
	if state.Cluster == nil {
		return ""
	}
	if state.Cluster.ClusterName != "" {
		return state.Cluster.ClusterName
	}
	return state.Cluster.ContextName
}

func mapHasMultipleClusterSelection(state PageState) bool {
	return len(selectedClusterContexts(state.Signals.Clusters)) > 1
}

func mapDataEndpoint(state PageState) string {
	contextName := mapClusterContext(state)
	if contextName == "" {
		return "/ui/map-data"
	}
	return "/ui/map-data?context=" + url.QueryEscape(contextName)
}

func mapClusterSelectAttrs(cluster *kube.Cluster, active bool) templ.Attributes {
	return templ.Attributes{
		"type":                   "button",
		"class":                  clusterFilterClass(active),
		"aria-pressed":           checkedBool(active),
		"data-cluster-context":   cluster.ContextName,
		"data-cluster-selection": cluster.ContextName,
		"data-indicator:loading": true,
		"data-on:click":          "$context = " + signalLiteral(cluster.ContextName) + "; $clusters = " + signalLiteral(cluster.ContextName) + "; $resource = 'map'; $namespace = ''; $query = ''; $sortColumn = ''; $sortOrder = ''; $selectedName = ''; $selectedNamespace = ''; $detailMode = 'overview'; @get('/ui/table')",
	}
}

func overviewPodUsageLoadAttrs() templ.Attributes {
	return prometheusChartLoadAttrs(url.Values{
		"panel": {"pod-usage-overview"},
		"limit": {"8"},
	})
}

func detailPodUsageLoadAttrs(namespace, name string) templ.Attributes {
	return prometheusChartLoadAttrs(url.Values{
		"panel":     {"pod-usage-detail"},
		"namespace": {namespace},
		"name":      {name},
	})
}

func prometheusChartLoadAttrs(params url.Values) templ.Attributes {
	return templ.Attributes{
		"data-on-intersect__once": "@get('" + chartURL("/ui/charts/prometheus", params) + "')",
	}
}

func chartURL(path string, params url.Values) string {
	return path + "?" + params.Encode()
}

func hasPodSparklines(state PageState) bool {
	return state.Table.Kind == kube.KindPods
}

func sparklineContext(row kube.Row, state PageState) string {
	if row.Cluster != "" {
		return row.Cluster
	}
	return state.Signals.Context
}

func detailPodUsagePanelID(namespace, name string) string {
	value := "detail-pod-usage"
	if namespace != "" {
		value += "-" + slug(namespace)
	}
	if name != "" {
		value += "-" + slug(name)
	}
	return value
}

func slug(value string) string {
	var out strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			out.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(out.String(), "-")
}

func overviewStatLinkAttrs(metric kube.OverviewMetric) templ.Attributes {
	attrs := overviewResourceLinkAttrs(metric.Kind)
	class := "overview-link overview-stat-card"
	if metric.StatusKey != "" {
		class += " " + metric.StatusKey
	}
	if metric.Ratio != nil {
		class += " has-ratio"
		if boundedPercent(metric.Ratio.Percent) >= 100 {
			class += " full"
		}
	}
	attrs["class"] = class
	return attrs
}

func metricPieStyle(percent float64) string {
	return fmt.Sprintf("--value: %.1f;", boundedPercent(percent))
}

func metricPieDasharray(percent float64) string {
	return fmt.Sprintf("%.1f 100", boundedPercent(percent))
}

func boundedPercent(percent float64) float64 {
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}

func metricPieLabel(percent float64) string {
	return fmt.Sprintf("%.1f%%", percent)
}

func metricPieAria(metric kube.OverviewMetric) string {
	if metric.Ratio == nil {
		return metric.Label
	}
	return fmt.Sprintf("%s: %s of %s, %s", metric.Label, metric.Ratio.Numerator, metric.Ratio.Denominator, metricPieLabel(metric.Ratio.Percent))
}

func namespaceSelectAttrs() templ.Attributes {
	return templ.Attributes{
		"data-indicator:loading": true,
		"data-bind:namespace":    true,
		"data-on:change":         "$selectedName = ''; $selectedNamespace = ''; $detailMode = 'overview'; @get('/ui/table')",
	}
}

func clusterFilterAttrs(cluster *kube.Cluster, state PageState) templ.Attributes {
	active := clusterActive(cluster.ContextName, selectedClusterContexts(state.Signals.Clusters))
	endpoint := "/ui/table"
	if isOverview(state) {
		endpoint = "/ui/refresh"
	}
	return templ.Attributes{
		"type":                   "button",
		"class":                  clusterFilterClass(active),
		"aria-pressed":           checkedBool(active),
		"data-cluster-context":   cluster.ContextName,
		"data-cluster-selection": toggledClusters(cluster.ContextName, state),
		"data-indicator:loading": true,
		"data-on:click":          "$clusters = " + signalLiteral(toggledClusters(cluster.ContextName, state)) + "; $selectedName = ''; $selectedNamespace = ''; $detailMode = 'overview'; @get('" + endpoint + "')",
	}
}

func clusterFilterClass(active bool) string {
	if active {
		return "cluster-filter active"
	}
	return "cluster-filter"
}

func clusterActive(contextName string, activeContexts []string) bool {
	for _, active := range activeContexts {
		if active == contextName {
			return true
		}
	}
	return false
}

func toggledClusters(contextName string, state PageState) string {
	active := map[string]bool{}
	for _, value := range selectedClusterContexts(state.Signals.Clusters) {
		active[value] = true
	}
	active[contextName] = !active[contextName]
	values := []string{}
	for _, cluster := range state.Clusters {
		if cluster != nil && active[cluster.ContextName] {
			values = append(values, cluster.ContextName)
		}
	}
	return strings.Join(values, ",")
}

func selectedClusterContexts(value string) []string {
	values := []string{}
	for _, contextName := range strings.Split(value, ",") {
		contextName = strings.TrimSpace(contextName)
		if contextName != "" {
			values = append(values, contextName)
		}
	}
	return values
}

func fleetClusterCardClass(cluster FleetCluster) string {
	return "fleet-cluster-card " + cluster.StatusKey
}

func fleetClusterOverviewAttrs(cluster FleetCluster) templ.Attributes {
	return templ.Attributes{
		"type":                   "button",
		"class":                  "fleet-card-overview",
		"data-fleet-context":     cluster.Context,
		"data-indicator:loading": true,
		"data-on:click":          "$context = " + signalLiteral(cluster.Context) + "; $clusters = " + signalLiteral(cluster.Context) + "; $resource = 'overview'; $query = ''; $sortColumn = ''; $sortOrder = ''; $selectedName = ''; $selectedNamespace = ''; $detailMode = 'overview'; @get('/ui/refresh')",
	}
}

func fleetClusterIssueAttrs(cluster FleetCluster) templ.Attributes {
	return templ.Attributes{
		"type":                   "button",
		"class":                  "fleet-card-action",
		"data-fleet-context":     cluster.Context,
		"data-fleet-issue":       "true",
		"data-fleet-issue-kind":  string(cluster.IssueKind),
		"data-fleet-issue-query": cluster.IssueQuery,
		"data-indicator:loading": true,
		"data-on:click":          "$context = " + signalLiteral(cluster.Context) + "; $clusters = " + signalLiteral(cluster.Context) + "; $resource = " + signalLiteral(string(cluster.IssueKind)) + "; $namespace = ''; $query = " + signalLiteral(cluster.IssueQuery) + "; $sortColumn = ''; $sortOrder = ''; $selectedName = ''; $selectedNamespace = ''; $detailMode = 'overview'; @get('/ui/table')",
	}
}

func fleetMetricValue(metric kube.OverviewMetric) string {
	if metric.Value == "" {
		return "0"
	}
	return metric.Value
}

func actionItemClass(item ActionItem) string {
	return "action-item " + item.StatusKey
}

func actionItemAttrs(item ActionItem) templ.Attributes {
	endpoint := "/ui/table"
	if item.TargetKind == kube.KindOverview {
		endpoint = "/ui/refresh"
	}
	return templ.Attributes{
		"role":                           "button",
		"tabindex":                       "0",
		"data-action-context":            item.Context,
		"data-action-kind":               string(item.TargetKind),
		"data-action-query":              item.TargetQuery,
		"data-action-selected-name":      item.TargetName,
		"data-action-selected-namespace": item.TargetNamespace,
		"data-indicator:loading":         true,
		"data-on:click":                  "$context = " + signalLiteral(item.Context) + "; $clusters = " + signalLiteral(item.Context) + "; $resource = " + signalLiteral(string(item.TargetKind)) + "; $namespace = ''; $query = " + signalLiteral(item.TargetQuery) + "; $sortColumn = ''; $sortOrder = ''; $selectedName = " + signalLiteral(item.TargetName) + "; $selectedNamespace = " + signalLiteral(item.TargetNamespace) + "; $detailMode = 'overview'; @get('" + endpoint + "')",
		"data-on:keydown__enter":         "$context = " + signalLiteral(item.Context) + "; $clusters = " + signalLiteral(item.Context) + "; $resource = " + signalLiteral(string(item.TargetKind)) + "; $namespace = ''; $query = " + signalLiteral(item.TargetQuery) + "; $sortColumn = ''; $sortOrder = ''; $selectedName = " + signalLiteral(item.TargetName) + "; $selectedNamespace = " + signalLiteral(item.TargetNamespace) + "; $detailMode = 'overview'; @get('" + endpoint + "')",
	}
}

func searchInputAttrs() templ.Attributes {
	return templ.Attributes{
		"data-indicator:loading":        true,
		"data-bind:query":               true,
		"data-on:input__debounce.250ms": "$selectedName = ''; $selectedNamespace = ''; $detailMode = 'overview'; @get('/ui/table')",
	}
}

func refreshButtonAttrs() templ.Attributes {
	return templ.Attributes{
		"data-indicator:loading": true,
		"data-on:click":          "@get('/ui/refresh')",
	}
}

func sortHeaderAttrs(column string) templ.Attributes {
	columnLiteral := signalLiteral(column)
	nextOrder := "$sortColumn == " + columnLiteral + " ? ($sortOrder == 'asc' ? 'desc' : ($sortOrder == 'desc' ? '' : 'asc')) : 'asc'"
	return templ.Attributes{
		"data-indicator:loading": true,
		"data-sort-column":       column,
		"data-on:click":          "$sortOrder = " + nextOrder + "; $sortColumn = $sortOrder == '' ? '' : " + columnLiteral + "; $selectedName = ''; $selectedNamespace = ''; $detailMode = 'overview'; @get('/ui/table')",
	}
}

func sortAria(column, sortColumn, sortOrder string) string {
	if column != sortColumn {
		return "none"
	}
	switch sortOrder {
	case "asc":
		return "ascending"
	case "desc":
		return "descending"
	default:
		return "none"
	}
}

func sortButtonClass(column, sortColumn string) string {
	if column == sortColumn {
		return "sort-heading active"
	}
	return "sort-heading"
}

func sortIndicatorClass(column, sortColumn, sortOrder string) string {
	if column != sortColumn {
		return "sort-indicator"
	}
	if sortOrder == "asc" {
		return "sort-indicator icon-[lucide--arrow-up]"
	}
	if sortOrder == "desc" {
		return "sort-indicator icon-[lucide--arrow-down]"
	}
	return "sort-indicator"
}

func metricsBadgeLabel(state kube.MetricsState) string {
	if state.Available {
		return "metrics on"
	}
	return "metrics off"
}

func metricsBadgeClass(state kube.MetricsState) string {
	if state.Available {
		return "status-pill good"
	}
	return "status-pill neutral"
}

func metricsSourceLabel(state kube.MetricsState) string {
	parts := []string{}
	if state.Source != "" {
		parts = append(parts, state.Source)
	}
	if state.Window != "" {
		parts = append(parts, state.Window)
	}
	return strings.Join(parts, " · ")
}

func cpuSparklinePoints(samples []kube.UsageSample) string {
	return sparklinePoints(samples, func(sample kube.UsageSample) int64 {
		return sample.CPU
	})
}

func memorySparklinePoints(samples []kube.UsageSample) string {
	return sparklinePoints(samples, func(sample kube.UsageSample) int64 {
		return sample.Memory
	})
}

func sparklinePoints(samples []kube.UsageSample, value func(kube.UsageSample) int64) string {
	if len(samples) == 0 {
		return ""
	}
	maxValue := int64(0)
	for _, sample := range samples {
		if current := value(sample); current > maxValue {
			maxValue = current
		}
	}
	if maxValue == 0 {
		return ""
	}
	const width = 120.0
	const height = 36.0
	const pad = 3.0
	if len(samples) == 1 {
		y := sparklineY(value(samples[0]), maxValue, height, pad)
		return fmt.Sprintf("0 %.1f %.1f %.1f", y, width, y)
	}
	points := make([]string, 0, len(samples))
	for i, sample := range samples {
		x := float64(i) * width / float64(len(samples)-1)
		y := sparklineY(value(sample), maxValue, height, pad)
		points = append(points, fmt.Sprintf("%.1f %.1f", x, y))
	}
	return strings.Join(points, " ")
}

func sparklineY(value, maxValue int64, height, pad float64) float64 {
	if value <= 0 || maxValue <= 0 {
		return height - pad
	}
	ratio := float64(value) / float64(maxValue)
	ratio = math.Max(0, math.Min(1, ratio))
	return pad + (1-ratio)*(height-pad*2)
}

func progressAttrs() templ.Attributes {
	return templ.Attributes{
		"data-class:active": "$loading",
	}
}

func rowAttrs(row kube.Row, state PageState) templ.Attributes {
	contextName := state.Signals.Context
	if row.Cluster != "" {
		contextName = row.Cluster
	}
	attrs := templ.Attributes{
		"role":                   "button",
		"tabindex":               "0",
		"aria-selected":          checkedBool(selectedRow(row, state)),
		"data-selected":          checkedBool(selectedRow(row, state)),
		"data-row-name":          row.Name,
		"data-row-namespace":     row.Namespace,
		"data-row-cluster":       row.Cluster,
		"data-indicator:loading": true,
		"data-on:click":          "$context = " + signalLiteral(contextName) + "; $selectedName = " + signalLiteral(row.Name) + "; $selectedNamespace = " + signalLiteral(row.Namespace) + "; $detailMode = 'overview'; @get('/ui/selection')",
		"data-on:keydown__enter": "$context = " + signalLiteral(contextName) + "; $selectedName = " + signalLiteral(row.Name) + "; $selectedNamespace = " + signalLiteral(row.Namespace) + "; $detailMode = 'overview'; @get('/ui/selection')",
	}
	if class := rowClass(row, state); class != "" {
		attrs["class"] = class
	}
	return attrs
}

func closeDetailAttrs() templ.Attributes {
	return templ.Attributes{
		"type":                   "button",
		"title":                  "Close details",
		"data-close-detail":      "true",
		"data-indicator:loading": true,
		"data-on:click":          "$selectedName = ''; $selectedNamespace = ''; $detailMode = 'overview'; @get('/ui/selection')",
	}
}

func detailTabAttrs(mode string, state PageState) templ.Attributes {
	return templ.Attributes{
		"type":                   "button",
		"aria-selected":          checkedBool(state.Signals.DetailMode == mode),
		"data-detail-mode":       mode,
		"data-indicator:loading": true,
		"data-on:click":          "$detailMode = " + signalLiteral(mode) + "; @get('/ui/detail')",
	}
}

func detailFieldLinkHref(field kube.DetailField, signals Signals) string {
	if field.Link == nil {
		return ""
	}
	q := url.Values{}
	if signals.Context != "" {
		q.Set("context", signals.Context)
	}
	if signals.Clusters != "" {
		q.Set("clusters", signals.Clusters)
	}
	q.Set("resource", string(field.Link.Resource))
	if field.Link.Namespace != "" {
		q.Set("namespace", field.Link.Namespace)
		q.Set("selectedNamespace", field.Link.Namespace)
	}
	q.Set("selectedName", field.Link.Name)
	q.Set("detailMode", "overview")
	return "/?" + q.Encode()
}

func detailFieldLinkAttrs(field kube.DetailField, signals Signals) templ.Attributes {
	if field.Link == nil {
		return templ.Attributes{}
	}
	return templ.Attributes{
		"href":  detailFieldLinkHref(field, signals),
		"class": "font-[680] text-[var(--accent-2)] underline decoration-[var(--accent)] underline-offset-2 hover:text-[var(--text-strong)]",
		"title": "Open owner details",
	}
}

func selectedRow(row kube.Row, state PageState) bool {
	return row.Name == state.Signals.SelectedName && row.Namespace == state.Signals.SelectedNamespace && (row.Cluster == "" || row.Cluster == state.Signals.Context)
}

func rowClass(row kube.Row, state PageState) string {
	if terminalSuccessfulRow(row, state) {
		return "terminal-success"
	}
	return ""
}

func terminalSuccessfulRow(row kube.Row, state PageState) bool {
	switch state.Table.Kind {
	case kube.KindPods:
		return strings.EqualFold(row.Status, "Succeeded")
	case kube.KindJobs:
		return row.StatusKey == "good"
	default:
		return false
	}
}

func formatTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

func signalLiteral(value string) string {
	return strconv.Quote(value)
}

func resourceDef(kind kube.ResourceKind) kube.ResourceDef {
	for _, def := range kube.ResourceDefs {
		if def.Kind == kind {
			return def
		}
	}
	return kube.ResourceDefs[0]
}

func normalizeKindForResources(kind string, resources []kube.ResourceDef) kube.ResourceKind {
	for _, def := range resources {
		if string(def.Kind) == kind {
			return def.Kind
		}
	}
	return kube.KindOverview
}

func resourceDefForResources(kind kube.ResourceKind, resources []kube.ResourceDef) kube.ResourceDef {
	for _, def := range resources {
		if def.Kind == kind {
			return def
		}
	}
	return resourceDef(kind)
}

func quickSwitcherItems(state PageState) []QuickSwitcherItem {
	items := []QuickSwitcherItem{}
	items = append(items, quickSwitcherViewItems(state)...)
	items = append(items, quickSwitcherClusterItems(state)...)
	items = append(items, quickSwitcherNamespaceItems(state)...)
	items = append(items, quickSwitcherActionItems(state)...)
	if len(state.QuickItems) > 0 {
		items = append(items, state.QuickItems...)
	} else {
		items = append(items, quickSwitcherRowItems(state)...)
	}
	return items
}

func quickSwitcherViewItems(state PageState) []QuickSwitcherItem {
	items := make([]QuickSwitcherItem, 0, len(state.Resources))
	for _, def := range state.Resources {
		if def.Kind == "" {
			continue
		}
		endpoint := "/ui/table"
		namespace := state.Signals.Namespace
		if def.Scope == "cluster" || def.Kind == kube.KindOverview || def.Kind == kube.KindActions {
			namespace = ""
		}
		if def.Kind == kube.KindOverview {
			endpoint = "/ui/refresh"
		}
		items = append(items, QuickSwitcherItem{
			Label:         def.Label,
			Meta:          resourceQuickSwitcherMeta(def),
			MetaTokens:    resourceQuickSwitcherMetaTokens(def),
			KindLabel:     "View",
			Icon:          resourceButtonIconClass(def.Kind),
			Context:       state.Signals.Context,
			Clusters:      state.Signals.Clusters,
			Resource:      string(def.Kind),
			ResourceScope: def.Scope,
			Namespace:     namespace,
			DetailMode:    "overview",
			Endpoint:      endpoint,
		})
	}
	return items
}

func quickSwitcherClusterItems(state PageState) []QuickSwitcherItem {
	items := []QuickSwitcherItem{}
	for _, cluster := range state.Clusters {
		if cluster == nil || cluster.ContextName == "" {
			continue
		}
		label := cluster.ContextName
		meta := "Cluster"
		if cluster.Current {
			meta = "Current cluster"
		}
		items = append(items, QuickSwitcherItem{
			Label:      label,
			Meta:       meta,
			MetaTokens: []QuickSwitcherMetaToken{{Label: meta, Icon: "icon-[lucide--server]"}},
			KindLabel:  "Cluster",
			Icon:       "icon-[lucide--server]",
			Context:    cluster.ContextName,
			Clusters:   cluster.ContextName,
			Resource:   string(kube.KindOverview),
			DetailMode: "overview",
			Endpoint:   "/ui/refresh",
		})
	}
	return items
}

func quickSwitcherNamespaceItems(state PageState) []QuickSwitcherItem {
	if len(state.Namespaces) == 0 {
		return nil
	}
	resource := normalizeKindForResources(state.Signals.Resource, state.Resources)
	def := resourceDefForResources(resource, state.Resources)
	if def.Scope != "namespaced" || resource == kube.KindActions {
		resource = kube.KindPods
		def = resourceDefForResources(resource, state.Resources)
	}
	items := make([]QuickSwitcherItem, 0, len(state.Namespaces))
	for _, namespace := range state.Namespaces {
		if namespace == "" {
			continue
		}
		items = append(items, QuickSwitcherItem{
			Label:         namespace,
			Meta:          "Namespace",
			MetaTokens:    []QuickSwitcherMetaToken{{Label: "Namespace", Icon: resourceButtonIconClass(kube.KindNamespaces)}},
			KindLabel:     "Namespace",
			Icon:          "icon-[lucide--folder]",
			Search:        strings.ToLower(namespace + " namespace"),
			Context:       state.Signals.Context,
			Clusters:      state.Signals.Clusters,
			Resource:      string(resource),
			ResourceScope: def.Scope,
			Namespace:     namespace,
			DetailMode:    "overview",
			Endpoint:      "/ui/table",
		})
	}
	return items
}

func quickSwitcherActionItems(state PageState) []QuickSwitcherItem {
	items := make([]QuickSwitcherItem, 0, len(state.Actions.Items))
	for _, item := range state.Actions.Items {
		endpoint := "/ui/table"
		if item.TargetKind == kube.KindOverview {
			endpoint = "/ui/refresh"
		}
		items = append(items, QuickSwitcherItem{
			Label:             item.Title,
			Meta:              quickSwitcherActionMeta(item),
			MetaTokens:        quickSwitcherActionMetaTokens(item),
			KindLabel:         "Issue",
			Icon:              "icon-[lucide--circle-alert]",
			Context:           item.Context,
			Clusters:          item.Context,
			Resource:          string(item.TargetKind),
			ResourceScope:     resourceDef(item.TargetKind).Scope,
			Query:             item.TargetQuery,
			SelectedName:      item.TargetName,
			SelectedNamespace: item.TargetNamespace,
			DetailMode:        "overview",
			Endpoint:          endpoint,
		})
	}
	return items
}

func quickSwitcherRowItems(state PageState) []QuickSwitcherItem {
	if isStandalonePage(state) || len(state.Table.Rows) == 0 {
		return nil
	}
	const limit = 80
	items := make([]QuickSwitcherItem, 0, minInt(len(state.Table.Rows), limit))
	for index, row := range state.Table.Rows {
		if index >= limit {
			break
		}
		contextName := state.Signals.Context
		if row.Cluster != "" {
			contextName = row.Cluster
		}
		resourceScope := "cluster"
		if state.Table.Namespaced {
			resourceScope = "namespaced"
		}
		items = append(items, QuickSwitcherItem{
			Label:             row.Name,
			Meta:              quickSwitcherRowMeta(row, state),
			MetaTokens:        quickSwitcherRowMetaTokens(row, state),
			KindLabel:         state.Table.Label,
			Icon:              resourceButtonIconClass(state.Table.Kind),
			Context:           contextName,
			Clusters:          state.Signals.Clusters,
			Resource:          string(state.Table.Kind),
			ResourceScope:     resourceScope,
			Namespace:         state.Table.Namespace,
			Query:             state.Signals.Query,
			SortColumn:        state.Signals.SortColumn,
			SortOrder:         state.Signals.SortOrder,
			SelectedName:      row.Name,
			SelectedNamespace: row.Namespace,
			DetailMode:        "overview",
			Endpoint:          "/ui/selection",
		})
	}
	return items
}

func quickSwitcherItemAttrs(item QuickSwitcherItem, index int) templ.Attributes {
	attrs := templ.Attributes{
		"type":                          "button",
		"role":                          "option",
		"class":                         "quick-switcher-result",
		"tabindex":                      "-1",
		"data-quick-result":             true,
		"data-quick-index":              strconv.Itoa(index),
		"data-quick-label":              item.Label,
		"data-quick-meta":               item.Meta,
		"data-quick-kind":               item.KindLabel,
		"data-quick-search":             quickSwitcherSearchText(item),
		"data-quick-context":            item.Context,
		"data-quick-clusters":           item.Clusters,
		"data-quick-resource":           item.Resource,
		"data-quick-namespace":          item.Namespace,
		"data-quick-query":              item.Query,
		"data-quick-sort-column":        item.SortColumn,
		"data-quick-sort-order":         item.SortOrder,
		"data-quick-selected-name":      item.SelectedName,
		"data-quick-selected-namespace": item.SelectedNamespace,
		"data-quick-detail-mode":        item.DetailMode,
		"data-indicator:loading":        true,
		"data-on:click":                 quickSwitcherClickExpression(item),
		"aria-selected":                 "false",
	}
	if item.Resource != "" {
		attrs["data-resource-kind"] = item.Resource
		scope := item.ResourceScope
		if scope == "" {
			scope = resourceDef(kube.NormalizeKind(item.Resource)).Scope
		}
		attrs["data-resource-scope"] = scope
	}
	return attrs
}

func quickSwitcherClickExpression(item QuickSwitcherItem) string {
	endpoint := item.Endpoint
	if endpoint == "" {
		endpoint = "/ui/table"
	}
	return "$context = " + signalLiteral(item.Context) +
		"; $clusters = " + signalLiteral(item.Clusters) +
		"; $resource = " + signalLiteral(item.Resource) +
		"; $namespace = " + signalLiteral(item.Namespace) +
		"; $query = " + signalLiteral(item.Query) +
		"; $sortColumn = " + signalLiteral(item.SortColumn) +
		"; $sortOrder = " + signalLiteral(item.SortOrder) +
		"; $selectedName = " + signalLiteral(item.SelectedName) +
		"; $selectedNamespace = " + signalLiteral(item.SelectedNamespace) +
		"; $detailMode = " + signalLiteral(firstNonEmptyString(item.DetailMode, "overview")) +
		"; @get('" + endpoint + "')"
}

func resourceQuickSwitcherMeta(def kube.ResourceDef) string {
	if def.Kind == kube.KindOverview {
		return "Dashboard"
	}
	if def.Kind == kube.KindActions {
		return "Current issues"
	}
	if def.Scope == "cluster" {
		return "Cluster-scoped resource"
	}
	return "Namespaced resource"
}

func resourceQuickSwitcherMetaTokens(def kube.ResourceDef) []QuickSwitcherMetaToken {
	return []QuickSwitcherMetaToken{{
		Label: resourceQuickSwitcherMeta(def),
		Icon:  quickSwitcherMetaIconForResourceDef(def),
	}}
}

func quickSwitcherMetaIconForResourceDef(def kube.ResourceDef) string {
	if def.Kind == kube.KindOverview || def.Kind == kube.KindActions {
		return resourceButtonIconClass(def.Kind)
	}
	if def.Scope == "cluster" {
		return "icon-[lucide--server]"
	}
	return resourceButtonIconClass(kube.KindNamespaces)
}

func quickSwitcherActionMeta(item ActionItem) string {
	parts := []string{}
	if item.Context != "" {
		parts = append(parts, item.Context)
	}
	if item.Detail != "" {
		parts = append(parts, item.Detail)
	}
	if item.ActionLabel != "" {
		parts = append(parts, item.ActionLabel)
	}
	return strings.Join(parts, " · ")
}

func quickSwitcherActionMetaTokens(item ActionItem) []QuickSwitcherMetaToken {
	tokens := []QuickSwitcherMetaToken{}
	if item.Context != "" {
		tokens = append(tokens, QuickSwitcherMetaToken{Label: item.Context, Icon: "icon-[lucide--server]"})
	}
	if item.Detail != "" {
		tokens = append(tokens, QuickSwitcherMetaToken{Label: item.Detail, Icon: "icon-[lucide--info]"})
	}
	if item.ActionLabel != "" {
		tokens = append(tokens, QuickSwitcherMetaToken{Label: item.ActionLabel, Icon: "icon-[lucide--circle-alert]"})
	}
	return tokens
}

func quickSwitcherRowMeta(row kube.Row, state PageState) string {
	parts := []string{state.Table.Label}
	if row.Namespace != "" {
		parts = append(parts, row.Namespace)
	}
	if row.Cluster != "" {
		parts = append(parts, row.Cluster)
	}
	if row.Status != "" {
		parts = append(parts, row.Status)
	}
	return strings.Join(parts, " · ")
}

func quickSwitcherRowMetaTokens(row kube.Row, state PageState) []QuickSwitcherMetaToken {
	tokens := []QuickSwitcherMetaToken{{
		Label: state.Table.Label,
		Icon:  resourceButtonIconClass(state.Table.Kind),
	}}
	if row.Namespace != "" {
		tokens = append(tokens, QuickSwitcherMetaToken{Label: row.Namespace, Icon: resourceButtonIconClass(kube.KindNamespaces)})
	}
	if row.Cluster != "" {
		tokens = append(tokens, QuickSwitcherMetaToken{Label: row.Cluster, Icon: "icon-[lucide--server]"})
	}
	if row.Status != "" {
		tokens = append(tokens, QuickSwitcherMetaToken{Label: row.Status, Icon: quickSwitcherStatusIcon(row.Status)})
	}
	return tokens
}

func quickSwitcherMetaTokens(item QuickSwitcherItem) []QuickSwitcherMetaToken {
	if len(item.MetaTokens) > 0 {
		return item.MetaTokens
	}
	if item.Meta == "" {
		return nil
	}
	return []QuickSwitcherMetaToken{{Label: item.Meta, Icon: "icon-[lucide--info]"}}
}

func quickSwitcherStatusIcon(status string) string {
	switch strings.ToLower(status) {
	case "running", "active", "bound", "ready", "true", "succeeded", "complete":
		return "icon-[lucide--circle-check]"
	case "pending", "progressing", "terminating":
		return "icon-[lucide--loader]"
	case "failed", "error", "crashloopbackoff":
		return "icon-[lucide--circle-alert]"
	default:
		return "icon-[lucide--activity]"
	}
}

func quickSwitcherSearchText(item QuickSwitcherItem) string {
	if item.Search != "" {
		return item.Search
	}
	return strings.ToLower(strings.Join([]string{
		item.Label,
		item.Meta,
		item.KindLabel,
		item.Context,
		item.Clusters,
		item.Resource,
		item.Namespace,
		item.Query,
		item.SelectedName,
		item.SelectedNamespace,
	}, " "))
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
