package server

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"strings"
	"time"

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

type component struct {
	Name string
	Data any
}

func summaryView(state PageState) component {
	return component{Name: "summary", Data: state}
}

func namespacePicker(state PageState) component {
	return component{Name: "namespacePicker", Data: state}
}

func resourceNav(state PageState) component {
	return component{Name: "resourceNav", Data: state}
}

func tableView(state PageState) component {
	return component{Name: "table", Data: state}
}

func renderPage(w io.Writer, state PageState) error {
	return templates.ExecuteTemplate(w, "page", state)
}

func renderFragment(c component) string {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, c.Name, c.Data); err != nil {
		return fmt.Sprintf(`<div class="notice danger">render error: %s</div>`, template.HTMLEscapeString(err.Error()))
	}
	return buf.String()
}

func formatClock(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format("15:04:05")
}

func selected(a, b string) string {
	if a == b {
		return "selected"
	}
	return ""
}

func checked(a, b string) string {
	if a == b {
		return "true"
	}
	return "false"
}

func disabled(disabled bool) string {
	if disabled {
		return "disabled"
	}
	return ""
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

var templates = template.Must(template.New("ui").Funcs(template.FuncMap{
	"clock":    formatClock,
	"selected": selected,
	"checked":  checked,
	"disabled": disabled,
	"lower":    lower,
	"hasClass": hasClass,
}).Parse(pageTemplate))

const pageTemplate = `{{ define "page" -}}
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Kubernetto</title>
  <script type="module" src="https://cdn.jsdelivr.net/gh/starfederation/datastar@1.0.0/bundles/datastar.js"></script>
  <style>
    :root {
      color-scheme: light;
      --bg: #f5f7f8;
      --panel: #ffffff;
      --panel-2: #eef2f5;
      --text: #172026;
      --muted: #687781;
      --line: #dce3e8;
      --accent: #0d9488;
      --accent-2: #155e75;
      --danger: #b42318;
      --warn: #b45309;
      --good: #087443;
      --shadow: 0 12px 34px rgba(23, 32, 38, .08);
    }
    * { box-sizing: border-box; }
    html, body { min-height: 100%; }
    body {
      margin: 0;
      background: var(--bg);
      color: var(--text);
      font: 14px/1.45 ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      letter-spacing: 0;
    }
    button, input, select { font: inherit; }
    .app {
      min-height: 100vh;
      display: grid;
      grid-template-columns: 248px minmax(0, 1fr);
    }
    .side {
      background: #15252d;
      color: #e8f1f4;
      padding: 22px 16px;
      display: flex;
      flex-direction: column;
      gap: 22px;
    }
    .brand { display: flex; align-items: center; gap: 10px; font-weight: 760; font-size: 18px; }
    .brand-mark {
      width: 32px; height: 32px; border-radius: 7px; display: grid; place-items: center;
      background: #0d9488; color: white; font-weight: 800;
    }
    .context { color: #a8bac2; font-size: 12px; display: grid; gap: 5px; }
    .context strong { color: #ffffff; font-size: 13px; overflow-wrap: anywhere; }
    .nav { display: grid; gap: 5px; }
    .nav button {
      width: 100%;
      border: 0;
      border-radius: 7px;
      background: transparent;
      color: #cfe0e5;
      display: flex;
      justify-content: space-between;
      align-items: center;
      min-height: 36px;
      padding: 8px 10px;
      cursor: pointer;
      text-align: left;
    }
    .nav button:hover, .nav button[aria-pressed="true"] { background: rgba(255,255,255,.09); color: #fff; }
    .scope { color: #89a0aa; font-size: 11px; text-transform: uppercase; }
    .main { min-width: 0; padding: 20px 24px 28px; display: grid; gap: 18px; align-content: start; }
    .topbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
    .topbar h1 { margin: 0; font-size: 22px; line-height: 1.1; }
    .updated { color: var(--muted); font-size: 12px; white-space: nowrap; }
    .summary {
      display: grid;
      grid-template-columns: repeat(4, minmax(130px, 1fr));
      gap: 12px;
    }
    .metric {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 14px;
      box-shadow: var(--shadow);
      min-width: 0;
    }
    .metric .label { color: var(--muted); font-size: 12px; }
    .metric .value { font-weight: 760; font-size: 24px; margin-top: 4px; overflow-wrap: anywhere; }
    .toolbar {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      box-shadow: var(--shadow);
      display: grid;
      grid-template-columns: minmax(160px, 260px) minmax(180px, 1fr) auto;
      gap: 10px;
      padding: 12px;
      align-items: end;
    }
    .field { display: grid; gap: 5px; min-width: 0; }
    .field label { color: var(--muted); font-size: 12px; }
    .field input, .field select {
      width: 100%;
      height: 36px;
      border: 1px solid var(--line);
      border-radius: 7px;
      background: #fff;
      color: var(--text);
      padding: 0 10px;
      outline: none;
    }
    .field input:focus, .field select:focus { border-color: var(--accent); box-shadow: 0 0 0 3px rgba(13,148,136,.14); }
    .icon-button {
      height: 36px;
      width: 38px;
      align-self: end;
      border: 1px solid var(--line);
      border-radius: 7px;
      background: var(--panel-2);
      color: var(--text);
      cursor: pointer;
    }
    .icon-button:hover { border-color: var(--accent); color: var(--accent-2); }
    .table-panel {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      box-shadow: var(--shadow);
      min-width: 0;
      overflow: hidden;
    }
    .table-head {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: center;
      padding: 14px 16px;
      border-bottom: 1px solid var(--line);
    }
    .table-head h2 { margin: 0; font-size: 16px; line-height: 1.2; }
    .table-wrap { overflow: auto; }
    table { width: 100%; border-collapse: collapse; min-width: 760px; }
    th, td { padding: 10px 12px; border-bottom: 1px solid var(--line); text-align: left; white-space: nowrap; }
    th { color: var(--muted); font-size: 12px; background: #f9fbfc; font-weight: 680; }
    td { color: #25323a; }
    tr:hover td { background: #fbfcfd; }
    .primary { font-weight: 690; color: var(--text); }
    td.status { color: inherit; background: transparent; }
    td.status > span {
      display: inline-flex;
      align-items: center;
      min-height: 24px;
      padding: 2px 8px;
      border-radius: 999px;
      font-size: 12px;
      font-weight: 680;
    }
    td.status.good > span { background: #ddf7ec; color: var(--good); }
    td.status.warn > span { background: #fff4dd; color: var(--warn); }
    td.status.neutral > span { background: #e9f1f5; color: var(--accent-2); }
    .notice {
      padding: 14px 16px;
      border-radius: 8px;
      border: 1px solid var(--line);
      background: #fff;
      color: var(--muted);
    }
    .danger { border-color: #f3b8b2; background: #fff1f0; color: var(--danger); }
    .empty { padding: 34px 16px; color: var(--muted); text-align: center; }
    @media (max-width: 860px) {
      .app { grid-template-columns: 1fr; }
      .side { position: static; }
      .summary { grid-template-columns: repeat(2, minmax(0, 1fr)); }
      .toolbar { grid-template-columns: 1fr; }
      .topbar { align-items: flex-start; flex-direction: column; }
    }
  </style>
</head>
<body>
  <div class="app"
       data-signals:resource="'{{ .Signals.Resource }}'"
       data-signals:namespace="'{{ .Signals.Namespace }}'"
       data-signals:query="'{{ .Signals.Query }}'"
       data-init="@get('/events')">
    <aside class="side">
      <div class="brand"><div class="brand-mark">K</div><div>Kubernetto</div></div>
      <div class="context">
        <span>Context</span>
        <strong>{{ if .Cluster }}{{ .Cluster.ContextName }}{{ else }}unconfigured{{ end }}</strong>
        <span>{{ if .Cluster }}{{ .Cluster.ConfigSource }}{{ else }}kubeconfig unavailable{{ end }}</span>
      </div>
      {{ template "resourceNav" . }}
    </aside>
    <main class="main">
      <div class="topbar">
        <h1>Cluster Dashboard</h1>
        <div class="updated">Live summary every 5s</div>
      </div>
      {{ template "summary" . }}
      <section class="toolbar" aria-label="Table controls">
        {{ template "namespacePicker" . }}
        <div class="field">
          <label for="query">Search</label>
          <input id="query" type="search" placeholder="Filter visible fields" data-bind:query data-on:input__debounce.250ms="@get('/ui/table')">
        </div>
        <button class="icon-button" type="button" title="Refresh" data-on:click="@get('/ui/refresh')">↻</button>
      </section>
      {{ template "table" . }}
    </main>
  </div>
</body>
</html>
{{- end }}

{{ define "summary" -}}
<section id="summary" class="summary" aria-label="Cluster summary">
  {{ if .Summary.Error }}<div class="notice danger" style="grid-column: 1 / -1">{{ .Summary.Error }}</div>{{ end }}
  <div class="metric"><div class="label">Version</div><div class="value">{{ if .Summary.ServerVersion }}{{ .Summary.ServerVersion }}{{ else }}unknown{{ end }}</div></div>
  <div class="metric"><div class="label">Nodes</div><div class="value">{{ .Summary.Nodes }}</div></div>
  <div class="metric"><div class="label">Pods</div><div class="value">{{ .Summary.Pods }}</div></div>
  <div class="metric"><div class="label">Deployments</div><div class="value">{{ .Summary.Deployments }}</div></div>
</section>
{{- end }}

{{ define "resourceNav" -}}
<nav id="resource-nav" class="nav" aria-label="Resources">
  {{ range .Resources }}
    <button type="button"
            aria-pressed="{{ checked $.Signals.Resource (printf "%s" .Kind) }}"
            data-on:click="$resource = '{{ .Kind }}'; @get('/ui/table')">
      <span>{{ .Label }}</span>
      <span class="scope">{{ .Scope }}</span>
    </button>
  {{ end }}
</nav>
{{- end }}

{{ define "namespacePicker" -}}
<div id="namespace-picker" class="field">
  <label for="namespace">Namespace</label>
  <select id="namespace" data-bind:namespace data-on:change="@get('/ui/table')" {{ disabled (not .Table.Namespaced) }}>
    <option value="">All namespaces</option>
    {{ range .Namespaces }}
      <option value="{{ . }}" {{ selected $.Signals.Namespace . }}>{{ . }}</option>
    {{ end }}
  </select>
  {{ if .NamespaceErr }}<span class="updated">{{ .NamespaceErr }}</span>{{ end }}
</div>
{{- end }}

{{ define "table" -}}
<section id="table-panel" class="table-panel" aria-live="polite">
  <div class="table-head">
    <h2>{{ .Table.Label }}</h2>
    <div class="updated">{{ len .Table.Rows }} rows · updated {{ clock .Table.UpdatedAt }}</div>
  </div>
  {{ if .Table.Error }}
    <div class="notice danger">{{ .Table.Error }}</div>
  {{ else if not .Table.Rows }}
    <div class="empty">No {{ lower .Table.Label }} found.</div>
  {{ else }}
    <div class="table-wrap">
      <table>
        <thead>
          <tr>{{ range .Table.Columns }}<th scope="col">{{ . }}</th>{{ end }}</tr>
        </thead>
        <tbody>
          {{ range .Table.Rows }}
            <tr>{{ range .Cells }}<td class="{{ .Class }}">{{ if hasClass .Class "status" }}<span>{{ .Value }}</span>{{ else }}{{ .Value }}{{ end }}</td>{{ end }}</tr>
          {{ end }}
        </tbody>
      </table>
    </div>
  {{ end }}
</section>
{{- end }}
`
