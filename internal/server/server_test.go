package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadSignalsFromQuery(t *testing.T) {
	req := httptest.NewRequest("GET", "/?context=kind-local&resource=deployments&namespace=prod&query=api&sortColumn=Name&sortOrder=desc&selectedName=api&selectedNamespace=prod&detailMode=yaml", nil)

	signals := readSignals(req)

	if signals.Context != "kind-local" {
		t.Fatalf("context = %q, want kind-local", signals.Context)
	}
	if signals.Resource != "deployments" {
		t.Fatalf("resource = %q, want deployments", signals.Resource)
	}
	if signals.Namespace != "prod" {
		t.Fatalf("namespace = %q, want prod", signals.Namespace)
	}
	if signals.Query != "api" {
		t.Fatalf("query = %q, want api", signals.Query)
	}
	if signals.SortColumn != "Name" || signals.SortOrder != "desc" {
		t.Fatalf("sort = %q/%q, want Name/desc", signals.SortColumn, signals.SortOrder)
	}
	if signals.SelectedName != "api" || signals.SelectedNamespace != "prod" {
		t.Fatalf("selection = %q/%q, want api/prod", signals.SelectedName, signals.SelectedNamespace)
	}
	if signals.DetailMode != "yaml" {
		t.Fatalf("detailMode = %q, want yaml", signals.DetailMode)
	}
}

func TestReadSignalsDefaults(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)

	signals := readSignals(req)

	if signals.Resource != "overview" {
		t.Fatalf("resource = %q, want overview", signals.Resource)
	}
	if signals.DetailMode != "overview" {
		t.Fatalf("detailMode = %q, want overview", signals.DetailMode)
	}
}

func TestHandleIndexDefaultsToClusterOverview(t *testing.T) {
	app := New(nil, context.Background(), nil)
	req := httptest.NewRequest("GET", "/", nil)
	res := httptest.NewRecorder()

	app.handleIndex(res, req)

	body := res.Body.String()
	if !strings.Contains(body, `data-signals:resource="&#34;overview&#34;"`) {
		t.Fatalf("index did not render overview resource signal")
	}
	if !strings.Contains(body, `datastar@v1.0.2`) {
		t.Fatalf("index did not render version-locked Datastar asset")
	}
	if !strings.Contains(body, "Cluster Overview") {
		t.Fatalf("index did not render cluster overview")
	}
}

func TestHandleIndexUsesQueryState(t *testing.T) {
	app := New(nil, context.Background(), nil)
	req := httptest.NewRequest("GET", "/?resource=deployments&query=api", nil)
	res := httptest.NewRecorder()

	app.handleIndex(res, req)

	body := res.Body.String()
	if !strings.Contains(body, `data-signals:resource="&#34;deployments&#34;"`) {
		t.Fatalf("index did not render requested resource signal")
	}
	if !strings.Contains(body, `value="api"`) {
		t.Fatalf("index did not render requested query value")
	}
}

func TestHandleIndexRendersGroupedResourceNav(t *testing.T) {
	app := New(nil, context.Background(), nil)
	req := httptest.NewRequest("GET", "/?resource=services", nil)
	res := httptest.NewRecorder()

	app.handleIndex(res, req)

	body := res.Body.String()
	for _, want := range []string{"resource-group-button", "Workloads", "Storage", "Network", "Security", "Configuration", "Cluster", "EndpointSlices"} {
		if !strings.Contains(body, want) {
			t.Fatalf("grouped nav did not contain %q", want)
		}
	}
	if !strings.Contains(body, `data-resource-group-children="workloads" hidden`) {
		t.Fatalf("inactive workloads children were not hidden")
	}
}

func TestStateClearsNamespaceForClusterScopedExpandedResources(t *testing.T) {
	app := New(nil, context.Background(), nil)
	req := httptest.NewRequest("GET", "/?resource=storageclasses&namespace=prod&selectedName=fast&selectedNamespace=prod", nil)

	state := app.state(readSignals(req))

	if state.Signals.Namespace != "" {
		t.Fatalf("namespace = %q, want cleared for cluster-scoped resource", state.Signals.Namespace)
	}
	if state.Signals.SelectedNamespace != "" {
		t.Fatalf("selected namespace = %q, want cleared for cluster-scoped resource", state.Signals.SelectedNamespace)
	}
}

func TestHandleTablePatchesNormalizedSignals(t *testing.T) {
	app := New(nil, context.Background(), nil)
	req := httptest.NewRequest("GET", "/ui/table?resource=storageclasses&namespace=prod&selectedName=fast&selectedNamespace=prod", nil)
	res := httptest.NewRecorder()

	app.handleTable(res, req)

	body := res.Body.String()
	if !strings.Contains(body, "event: datastar-patch-signals") {
		t.Fatalf("table response did not patch signals")
	}
	if !strings.Contains(body, `data: signals {"context":"","resource":"storageclasses","namespace":"","query":"","sortColumn":"","sortOrder":"","selectedName":"fast","selectedNamespace":"","detailMode":"overview"}`) {
		t.Fatalf("table response did not patch normalized signal state:\n%s", body)
	}
	if !strings.Contains(body, "event: datastar-patch-elements") {
		t.Fatalf("table response did not patch elements")
	}
}

func TestHandleIndexRendersTableAutoRefresh(t *testing.T) {
	app := New(nil, context.Background(), nil)
	req := httptest.NewRequest("GET", "/?resource=pods", nil)
	res := httptest.NewRecorder()

	app.handleIndex(res, req)

	body := res.Body.String()
	if !strings.Contains(body, `id="content-grid"`) {
		t.Fatalf("index did not render content grid")
	}
	if !strings.Contains(body, `data-on-interval__duration.5s="@get(&#39;/ui/table&#39;)"`) {
		t.Fatalf("index did not render table auto-refresh interval")
	}
	if !strings.Contains(body, `data-class:pending="$loading"`) || !strings.Contains(body, `data-show="$loading"`) {
		t.Fatalf("index did not render loading state hooks")
	}
}

func TestHandleIndexDoesNotAutoRefreshOverviewAsTable(t *testing.T) {
	app := New(nil, context.Background(), nil)
	req := httptest.NewRequest("GET", "/", nil)
	res := httptest.NewRecorder()

	app.handleIndex(res, req)

	body := res.Body.String()
	if strings.Contains(body, `data-on-interval__duration.5s="@get(&#39;/ui/table&#39;)"`) {
		t.Fatalf("index rendered table auto-refresh interval on overview")
	}
}

func TestAssetEndpoints(t *testing.T) {
	app := New(nil, context.Background(), nil)
	tests := []struct {
		name        string
		path        string
		contentType string
		body        string
	}{
		{
			name:        "tracked asset",
			path:        "/assets/README.txt",
			contentType: "text/plain",
			body:        "Generated frontend bundles",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			res := httptest.NewRecorder()

			app.Routes().ServeHTTP(res, req)

			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
			}
			if got := res.Header().Get("Content-Type"); !strings.HasPrefix(got, tt.contentType) {
				t.Fatalf("content type = %q, want prefix %q", got, tt.contentType)
			}
			if got := res.Header().Get("Cache-Control"); got != "no-cache" {
				t.Fatalf("cache control = %q, want no-cache", got)
			}
			if body := res.Body.String(); !strings.Contains(body, tt.body) {
				t.Fatalf("body did not contain %q", tt.body)
			}
		})
	}
}
