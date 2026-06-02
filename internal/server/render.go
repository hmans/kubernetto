package server

//go:generate go run github.com/a-h/templ/cmd/templ generate

import (
	"bytes"
	"context"
	"encoding/json"
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
		"data-signals:resource":          jsString(state.Signals.Resource),
		"data-signals:namespace":         jsString(state.Signals.Namespace),
		"data-signals:query":             jsString(state.Signals.Query),
		"data-signals:selectedName":      jsString(state.Signals.SelectedName),
		"data-signals:selectedNamespace": jsString(state.Signals.SelectedNamespace),
		"data-signals:detailMode":        jsString(state.Signals.DetailMode),
		"data-signals:loading":           "false",
		"data-init":                      "@get('/events')",
	}
}

func resourceButtonAttrs(def kube.ResourceDef) templ.Attributes {
	return templ.Attributes{
		"data-indicator:loading": true,
		"data-on:click":          "$resource = " + jsString(string(def.Kind)) + "; $selectedName = ''; $selectedNamespace = ''; $detailMode = 'overview'; @get('/ui/table')",
	}
}

func namespaceSelectAttrs() templ.Attributes {
	return templ.Attributes{
		"data-indicator:loading": true,
		"data-bind:namespace":    true,
		"data-on:change":         "$selectedName = ''; $selectedNamespace = ''; $detailMode = 'overview'; @get('/ui/table')",
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
		"data-indicator:loading": true,
		"data-on:click":          "$selectedName = " + jsString(row.Name) + "; $selectedNamespace = " + jsString(row.Namespace) + "; $detailMode = 'overview'; @get('/ui/selection')",
		"data-on:keydown__enter": "$selectedName = " + jsString(row.Name) + "; $selectedNamespace = " + jsString(row.Namespace) + "; $detailMode = 'overview'; @get('/ui/selection')",
	}
}

func closeDetailAttrs() templ.Attributes {
	return templ.Attributes{
		"type":                   "button",
		"title":                  "Close details",
		"data-indicator:loading": true,
		"data-on:click":          "$selectedName = ''; $selectedNamespace = ''; $detailMode = 'overview'; @get('/ui/selection')",
	}
}

func detailTabAttrs(mode string, state PageState) templ.Attributes {
	return templ.Attributes{
		"type":                   "button",
		"aria-selected":          checkedBool(state.Signals.DetailMode == mode),
		"data-indicator:loading": true,
		"data-on:click":          "$detailMode = " + jsString(mode) + "; @get('/ui/detail')",
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

func jsString(value string) string {
	out, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(out)
}
