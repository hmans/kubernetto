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
			name:        "css",
			path:        "/assets/app.css",
			contentType: "text/css; charset=utf-8",
			body:        ":root",
		},
		{
			name:        "js",
			path:        "/assets/app.js",
			contentType: "text/javascript; charset=utf-8",
			body:        "const themeKey",
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
			if got := res.Header().Get("Content-Type"); got != tt.contentType {
				t.Fatalf("content type = %q, want %q", got, tt.contentType)
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
