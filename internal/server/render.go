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
		"data-signals:context":          signalLiteral(state.Signals.Context),
		"data-signals:resource":         signalLiteral(state.Signals.Resource),
		"data-signals:namespace":        signalLiteral(state.Signals.Namespace),
		"data-signals:query":            signalLiteral(state.Signals.Query),
		"data-signals:loading":          "false",
		"data-on-interval__duration.5s": "@get('/ui/summary')",
	}
}

func resourceButtonAttrs(def kube.ResourceDef) templ.Attributes {
	return templ.Attributes{
		"data-indicator:loading": true,
		"data-on:click":          "$resource = '" + string(def.Kind) + "'; @get('/ui/table')",
	}
}

func namespaceSelectAttrs() templ.Attributes {
	return templ.Attributes{
		"data-indicator:loading": true,
		"data-bind:namespace":    true,
		"data-on:change":         "@get('/ui/table')",
	}
}

func contextSelectAttrs() templ.Attributes {
	return templ.Attributes{
		"data-indicator:loading": true,
		"data-bind:context":      true,
		"data-on:change":         "$namespace = ''; @get('/ui/refresh')",
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

func progressAttrs() templ.Attributes {
	return templ.Attributes{
		"data-class:active": "$loading",
	}
}

func signalLiteral(value string) string {
	return strconv.Quote(value)
}
