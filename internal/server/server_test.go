package server

import (
	"context"
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

	if signals.Resource != "pods" {
		t.Fatalf("resource = %q, want pods", signals.Resource)
	}
	if signals.DetailMode != "overview" {
		t.Fatalf("detailMode = %q, want overview", signals.DetailMode)
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
