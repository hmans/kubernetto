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
	Cluster      *kube.Cluster
	Clusters     []*kube.Cluster
	Resources    []kube.ResourceDef
	Signals      Signals
	Summary      kube.Summary
	Overview     kube.ClusterOverview
	Table        kube.Table
	Detail       kube.ResourceDetail
	Namespaces   []string
	NamespaceErr string
}

type Signals struct {
	Context           string `json:"context"`
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

func contentGridClass(state PageState) string {
	if hasDetail(state) {
		return "content-grid has-detail"
	}
	return "content-grid"
}

func appSignalAttrs(state PageState) templ.Attributes {
	return templ.Attributes{
		"data-signals:context":           signalLiteral(state.Signals.Context),
		"data-signals:resource":          signalLiteral(state.Signals.Resource),
		"data-signals:namespace":         signalLiteral(state.Signals.Namespace),
		"data-signals:query":             signalLiteral(state.Signals.Query),
		"data-signals:sortColumn":        signalLiteral(state.Signals.SortColumn),
		"data-signals:sortOrder":         signalLiteral(state.Signals.SortOrder),
		"data-signals:selectedName":      signalLiteral(state.Signals.SelectedName),
		"data-signals:selectedNamespace": signalLiteral(state.Signals.SelectedNamespace),
		"data-signals:detailMode":        signalLiteral(state.Signals.DetailMode),
		"data-signals:loading":           "false",
		"data-on-interval__duration.5s":  "@get('/ui/summary')",
	}
}

func tableAutoRefreshAttrs(state PageState) templ.Attributes {
	if isOverview(state) {
		return templ.Attributes{}
	}
	return templ.Attributes{
		"data-on-interval__duration.5s": "@get('/ui/table')",
	}
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
	case kube.KindPods:
		return "icon-[uil--cube]"
	case kube.KindDeployments:
		return "icon-[uil--rocket]"
	case kube.KindStatefulSet:
		return "icon-[uil--layers]"
	case kube.KindDaemonSet:
		return "icon-[uil--layer-group]"
	case kube.KindServices:
		return "icon-[uil--server-alt]"
	case kube.KindIngresses:
		return "icon-[uil--globe]"
	case kube.KindNodes:
		return "icon-[uil--server-network]"
	case kube.KindNamespaces:
		return "icon-[uil--folder-network]"
	default:
		return "icon-[uil--servers]"
	}
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

func overviewPodUsageLoadAttrs() templ.Attributes {
	return prometheusChartLoadAttrs(url.Values{
		"panel":  {"pod-usage-overview"},
		"limit":  {"8"},
		"cpu":    {kube.PodCPUQuery()},
		"memory": {kube.PodMemoryQuery()},
	})
}

func detailPodUsageLoadAttrs(namespace, name string) templ.Attributes {
	return prometheusChartLoadAttrs(url.Values{
		"panel":     {"pod-usage-detail"},
		"namespace": {namespace},
		"name":      {name},
		"cpu":       {kube.PodCPUQueryFor(namespace, name)},
		"memory":    {kube.PodMemoryQueryFor(namespace, name)},
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

func contextSelectAttrs() templ.Attributes {
	return templ.Attributes{
		"data-indicator:loading": true,
		"data-bind:context":      true,
		"data-on:change":         "$namespace = ''; $sortColumn = ''; $sortOrder = ''; $selectedName = ''; $selectedNamespace = ''; $detailMode = 'overview'; @get('/ui/refresh')",
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
	return templ.Attributes{
		"role":                   "button",
		"tabindex":               "0",
		"aria-selected":          checkedBool(selectedRow(row, state)),
		"data-selected":          checkedBool(selectedRow(row, state)),
		"data-row-name":          row.Name,
		"data-row-namespace":     row.Namespace,
		"data-indicator:loading": true,
		"data-on:click":          "$selectedName = " + signalLiteral(row.Name) + "; $selectedNamespace = " + signalLiteral(row.Namespace) + "; $detailMode = 'overview'; @get('/ui/selection')",
		"data-on:keydown__enter": "$selectedName = " + signalLiteral(row.Name) + "; $selectedNamespace = " + signalLiteral(row.Namespace) + "; $detailMode = 'overview'; @get('/ui/selection')",
	}
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

func selectedRow(row kube.Row, state PageState) bool {
	return row.Name == state.Signals.SelectedName && row.Namespace == state.Signals.SelectedNamespace
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
