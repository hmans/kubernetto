package server

//go:generate go run github.com/a-h/templ/cmd/templ generate

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"io"
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
	Table        kube.Table
	Detail       kube.ResourceDetail
	Namespaces   []string
	NamespaceErr string
}

func summaryView(state PageState) templ.Component {
	return SummaryView(state)
}

func namespacePicker(state PageState) templ.Component {
	return NamespacePickerView(state)
}

func resourceNav(state PageState) templ.Component {
	return ResourceNavView(state)
}

func tableView(state PageState) templ.Component {
	return TableView(state)
}

func detailView(state PageState) templ.Component {
	return DetailView(state)
}

func renderPage(w io.Writer, state PageState) error {
	return Page(state).Render(context.Background(), w)
}

func renderFragment(component templ.Component) string {
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

func resourceButtonAttrs(def kube.ResourceDef) templ.Attributes {
	return templ.Attributes{
		"data-indicator:loading": true,
		"data-resource-kind":     string(def.Kind),
		"data-resource-scope":    def.Scope,
		"data-on:click":          "$resource = " + signalLiteral(string(def.Kind)) + "; $sortColumn = ''; $sortOrder = ''; $selectedName = ''; $selectedNamespace = ''; $detailMode = 'overview'; @get('/ui/table')",
	}
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

func sortIndicator(column, sortColumn, sortOrder string) string {
	if column != sortColumn {
		return ""
	}
	if sortOrder == "asc" {
		return "↑"
	}
	if sortOrder == "desc" {
		return "↓"
	}
	return ""
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
