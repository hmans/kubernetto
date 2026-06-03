# Kubernetto Datastar Patterns

## Core Files

- `internal/server/ui/render.go`: builds Datastar attributes for signals, actions, indicators, and auto-refresh.
- `internal/server/ui/page.templ`: owns the Datastar script tag, fragment roots, and loading overlay markup.
- `internal/server/server.go`: reads request signals, normalizes state, and emits SSE patches.
- `frontend/src/app.js`: mirrors selected UI state into the URL and handles small optimistic updates.
- `frontend/src/app.css`: styles pending/loading states and navigation feedback.

## Signal Rules

The Go signal contract uses camelCase JSON names:

```go
type Signals struct {
    SortColumn        string `json:"sortColumn"`
    SelectedNamespace string `json:"selectedNamespace"`
    DetailMode        string `json:"detailMode"`
}
```

Use kebab-case in `data-signals:*` attributes for any camelCase signal:

```go
templ.Attributes{
    "data-signals:sort-column":        signalLiteral(state.Signals.SortColumn),
    "data-signals:selected-namespace": signalLiteral(state.Signals.SelectedNamespace),
    "data-signals:detail-mode":        signalLiteral(state.Signals.DetailMode),
}
```

Use the camelCase signal names in Datastar expressions and JavaScript because those are the actual signal keys:

```go
"data-on:click": "$sortColumn = ''; $selectedName = ''; @get('/ui/table')"
```

```js
const urlStateKeys = ["sortColumn", "selectedNamespace", "detailMode"];
```

## SSE Handler Pattern

Read signals before creating the SSE stream, normalize them through server state, then patch signals before affected elements.

```go
func (s *Server) handleTable(w http.ResponseWriter, r *http.Request) {
    signals := readSignals(r)
    state := s.state(signals)
    sse := datastar.NewSSE(w, r)
    s.patchSignals(sse, state.Signals)
    s.patchElements(sse, "resource nav", ui.RenderFragment(ui.ResourceNavView(state)))
    s.patchElements(sse, "namespace picker", ui.RenderFragment(ui.NamespacePickerView(state)))
    s.patchElements(sse, "content", ui.RenderFragment(ui.ContentView(state)))
}
```

Use helpers that log patch failures instead of ignoring returned errors:

```go
func (s *Server) patchSignals(sse *datastar.ServerSentEventGenerator, signals ui.Signals) {
    if err := sse.MarshalAndPatchSignals(signals); err != nil {
        s.logger.Warn("patch datastar signals", "error", err)
    }
}
```

Patch only the fragments whose source data changed:

- Summary refresh: signals and `SummaryView`.
- Table/resource changes: signals, resource nav, namespace picker, and content.
- Selection changes: signals and content.
- Detail mode changes: signals and detail.

## Loading State Pattern

Use `data-indicator:loading` on every control that starts a Datastar request:

```go
templ.Attributes{
    "data-indicator:loading": true,
    "data-on:click": "@get('/ui/refresh')",
}
```

React to `$loading` in nearby UI:

```templ
<div id="content-grid" { tableAutoRefreshAttrs(state)... }>
    <div class="view-loading" style="display: none;" data-show="$loading" role="status" aria-live="polite">
        <span class="icon-[lucide--loader-circle]" aria-hidden="true"></span>
        <span>Loading</span>
    </div>
    ...
</div>
```

```go
func tableAutoRefreshAttrs(state PageState) templ.Attributes {
    attrs := templ.Attributes{"data-class:pending": "$loading"}
    if !isOverview(state) {
        attrs["data-on-interval__duration.5s"] = "@get('/ui/table')"
    }
    return attrs
}
```

## Responsiveness Pattern

Use lightweight JavaScript only for behavior that must happen before the SSE response arrives or that Datastar does not own in this app:

- URL query synchronization and browser history.
- Optimistic nav expansion/pressed state before a server patch.
- Theme persistence.

Keep these helpers consistent with server-normalized signals. If server state clears a namespace for cluster-scoped resources, JavaScript URL sync should do the same.

## Dependency Upgrade Checklist

1. Check official release/tag data for Datastar and `datastar-go`.
2. Update the browser script tag in `internal/server/ui/page.templ`.
3. Update `github.com/starfederation/datastar-go` in `go.mod` with `mise x -- go get github.com/starfederation/datastar-go@<version>`.
4. Inspect the upgraded Go module API before using new helper names:
   `mise x -- sh -c 'rg -n "MarshalAndPatchSignals|PatchElements|ReadSignals|WithView" $(go env GOPATH)/pkg/mod/github.com/starfederation/datastar-go@<version>'`
5. Run generation, asset build, tests, and `mise run build`.

## Test Targets

For signal/loading changes, prefer focused server tests:

- Initial page contains the pinned Datastar script URL.
- Initial page renders kebab-case `data-signals:*` attributes.
- Content fragments render `data-class:pending="$loading"` and `data-show="$loading"`.
- SSE handlers emit `event: datastar-patch-signals` with normalized JSON before element patches.
- Cluster-scoped resources clear namespace and selected namespace signals.

For frontend helper changes, build assets with `mise x -- pnpm build:assets` and smoke-test rendered HTML or a local browser session when available.
