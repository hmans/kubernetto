package server

//go:generate go run github.com/a-h/templ/cmd/templ generate

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"io"
	"strings"
	"time"

	"github.com/a-h/templ"

	"kubernetto/internal/kube"
)

type PageState struct {
	Cluster      *kube.Cluster
	Resources    []kube.ResourceDef
	Signals      Signals
	Summary      kube.Summary
	Table        kube.Table
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
		"data-signals:resource":   jsString(state.Signals.Resource),
		"data-signals:namespace":  jsString(state.Signals.Namespace),
		"data-signals:query":      jsString(state.Signals.Query),
		"data-signals:sortColumn": jsString(state.Signals.SortColumn),
		"data-signals:sortOrder":  jsString(state.Signals.SortOrder),
		"data-signals:loading":    "false",
		"data-init":               "@get('/events')",
	}
}

func resourceButtonAttrs(def kube.ResourceDef) templ.Attributes {
	return templ.Attributes{
		"data-indicator:loading": true,
		"data-on:click":          "$resource = '" + string(def.Kind) + "'; $sortColumn = ''; $sortOrder = ''; @get('/ui/table')",
	}
}

func namespaceSelectAttrs() templ.Attributes {
	return templ.Attributes{
		"data-indicator:loading": true,
		"data-bind:namespace":    true,
		"data-on:change":         "@get('/ui/table')",
	}
}

func searchInputAttrs() templ.Attributes {
	return templ.Attributes{
		"data-indicator:loading":        true,
		"data-bind:query":               true,
		"data-on:input__debounce.250ms": "@get('/ui/table')",
	}
}

func refreshButtonAttrs() templ.Attributes {
	return templ.Attributes{
		"data-indicator:loading": true,
		"data-on:click":          "@get('/ui/refresh')",
	}
}

func sortHeaderAttrs(column string) templ.Attributes {
	columnLiteral := jsString(column)
	nextOrder := "$sortColumn == " + columnLiteral + " ? ($sortOrder == 'asc' ? 'desc' : ($sortOrder == 'desc' ? '' : 'asc')) : 'asc'"
	return templ.Attributes{
		"data-indicator:loading": true,
		"data-on:click":          "$sortOrder = " + nextOrder + "; $sortColumn = $sortOrder == '' ? '' : " + columnLiteral + "; @get('/ui/table')",
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

func jsString(value string) string {
	return fmt.Sprintf("%q", value)
}

func progressAttrs() templ.Attributes {
	return templ.Attributes{
		"data-class:active": "$loading",
	}
}
