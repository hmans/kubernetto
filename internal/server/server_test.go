package server

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"kubernetto/internal/kube"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestReadSignalsFromQuery(t *testing.T) {
	req := httptest.NewRequest("GET", "/?context=kind-local&clusters=kind-local,prod-west&resource=deployments&namespace=prod&query=api&sortColumn=Name&sortOrder=desc&selectedName=api&selectedNamespace=prod&detailMode=yaml", nil)

	signals := readSignals(req)

	if signals.Context != "kind-local" {
		t.Fatalf("context = %q, want kind-local", signals.Context)
	}
	if signals.Clusters != "kind-local,prod-west" {
		t.Fatalf("clusters = %q, want kind-local,prod-west", signals.Clusters)
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
	if !strings.Contains(body, `src="/assets/datastar.js"`) {
		t.Fatalf("index did not render bundled Datastar asset")
	}
	if strings.Contains(body, `cdn.jsdelivr.net`) {
		t.Fatalf("index rendered external Datastar CDN asset")
	}
	if !strings.Contains(body, "Cluster Overview") {
		t.Fatalf("index did not render cluster overview")
	}
}

func TestRoutesSetTightenedSecurityHeaders(t *testing.T) {
	app := New(nil, context.Background(), nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()

	app.Routes().ServeHTTP(res, req)

	csp := res.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self' 'unsafe-eval'") {
		t.Fatalf("csp script-src = %q, want self with Datastar eval allowance", csp)
	}
	if strings.Contains(csp, "cdn.jsdelivr.net") {
		t.Fatalf("csp still allows jsDelivr: %q", csp)
	}
	if !strings.Contains(csp, "connect-src 'self'") {
		t.Fatalf("csp connect-src = %q, want self", csp)
	}
}

func TestHandleIndexRendersFleetOverviewForMultipleClusters(t *testing.T) {
	app := New([]*kube.Cluster{
		testClusterWithObjects("dev",
			testNode("dev-node", true),
			testPod("api", corev1.PodRunning),
		),
		testClusterWithObjects("prod",
			testNode("prod-node", false),
			testPod("worker", corev1.PodFailed),
			&corev1.Event{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-event", Namespace: "default"},
				Type:       corev1.EventTypeWarning,
				Reason:     "BackOff",
				Message:    "retrying failed pod",
				InvolvedObject: corev1.ObjectReference{
					Kind:      "Pod",
					Name:      "worker",
					Namespace: "default",
				},
				Count: 2,
			},
		),
	}, context.Background(), nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()

	app.handleIndex(res, req)

	body := res.Body.String()
	for _, want := range []string{
		`Fleet Overview`,
		`2 clusters`,
		`class="fleet-cluster-card`,
		`data-fleet-context="dev"`,
		`data-fleet-context="prod"`,
		`data-fleet-issue="true"`,
		`data-fleet-issue-kind="events"`,
		`data-fleet-issue-query="BackOff"`,
		`Investigate warnings`,
		`dev`,
		`prod`,
		`Needs Attention`,
		`BackOff`,
		`retrying failed pod`,
		`Pod/worker`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("fleet overview did not render %q: %s", want, body)
		}
	}
}

func TestOverviewClusterFilterUsesSingleClusterOverview(t *testing.T) {
	app := New([]*kube.Cluster{
		testClusterWithObjects("dev", testNode("dev-node", true), testPod("api", corev1.PodRunning)),
		testClusterWithObjects("prod", testNode("prod-node", true), testPod("worker", corev1.PodRunning)),
	}, context.Background(), nil)
	state := app.state(readSignals(httptest.NewRequest(http.MethodGet, "/?clusters=prod", nil)))

	if state.Signals.Context != "prod" {
		t.Fatalf("context = %q, want prod", state.Signals.Context)
	}
	if len(state.Fleet.Clusters) != 1 || state.Fleet.Clusters[0].Context != "prod" {
		t.Fatalf("fleet clusters = %#v, want one prod cluster", state.Fleet.Clusters)
	}
	if got := detailFieldValue(state.Overview.Identity, "Context"); got != "prod" {
		t.Fatalf("overview context = %q, want prod", got)
	}
}

func TestHandleIndexRendersActionItemsPage(t *testing.T) {
	app := New([]*kube.Cluster{
		testClusterWithObjects("prod",
			testNode("prod-node", false),
			testPod("worker", corev1.PodFailed),
			&corev1.Event{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-event", Namespace: "default"},
				Type:       corev1.EventTypeWarning,
				Reason:     "BackOff",
				Message:    "retrying failed pod",
				InvolvedObject: corev1.ObjectReference{
					Kind:      "Pod",
					Name:      "worker",
					Namespace: "default",
				},
				Count: 2,
			},
		),
	}, context.Background(), nil)
	req := httptest.NewRequest(http.MethodGet, "/?resource=actions", nil)
	res := httptest.NewRecorder()

	app.handleIndex(res, req)

	body := res.Body.String()
	for _, want := range []string{
		`data-signals:resource="&#34;actions&#34;"`,
		`Issues`,
		`data-resource-kind="actions"`,
		`BackOff`,
		`retrying failed pod`,
		`data-action-context="prod"`,
		`data-action-kind="events"`,
		`data-action-query="BackOff"`,
		`data-action-selected-name="worker-event"`,
		`data-action-selected-namespace="default"`,
		`$selectedName = &#34;worker-event&#34;`,
		`data-on-interval__duration.10s="@get(&#39;/ui/table?refresh=auto&#39;)"`,
		`Nodes ready 0/1`,
		`Pods healthy 0/1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("action items page did not render %q: %s", want, body)
		}
	}
	if strings.Contains(body, `id="namespace"`) || strings.Contains(body, `id="query"`) {
		t.Fatalf("action items page rendered table controls: %s", body)
	}
	if strings.Contains(body, `>Refresh<`) {
		t.Fatalf("action items page rendered a manual refresh button: %s", body)
	}
}

func TestActionItemsIgnoreSucceededPods(t *testing.T) {
	app := New([]*kube.Cluster{
		testClusterWithObjects("prod",
			testNode("prod-node", true),
			testPod("completed-job", corev1.PodSucceeded),
		),
	}, context.Background(), nil)
	req := httptest.NewRequest(http.MethodGet, "/?resource=actions", nil)
	res := httptest.NewRecorder()

	app.handleIndex(res, req)

	body := res.Body.String()
	if !strings.Contains(body, "No current issues") {
		t.Fatalf("action items page did not render empty state: %s", body)
	}
	content := body
	if _, after, ok := strings.Cut(body, `id="content-grid"`); ok {
		content = after
	}
	for _, unwanted := range []string{"Pods healthy", "completed-job", "Open pods"} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("action items page rendered non-actionable succeeded pod marker %q: %s", unwanted, content)
		}
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
	for _, want := range []string{"resource-group-button", "Issues", "Workloads", "Storage", "Network", "Security", "Configuration", "Cluster", "EndpointSlices"} {
		if !strings.Contains(body, want) {
			t.Fatalf("grouped nav did not contain %q", want)
		}
	}
	if !strings.Contains(body, `data-resource-group-children="workloads" hidden`) {
		t.Fatalf("inactive workloads children were not hidden")
	}
}

func TestHandleIndexRendersQuickSwitcher(t *testing.T) {
	app := New([]*kube.Cluster{
		testClusterWithObjects("prod", testPod("api", corev1.PodRunning)),
	}, context.Background(), nil)
	req := httptest.NewRequest("GET", "/", nil)
	res := httptest.NewRecorder()

	app.handleIndex(res, req)

	body := res.Body.String()
	for _, want := range []string{
		`id="quick-switcher"`,
		`data-quick-switcher`,
		`data-quick-open`,
		`aria-label="Find anything"`,
		`placeholder="Find anything"`,
		`data-quick-result`,
		`data-quick-resource="pods"`,
		`data-quick-selected-name="api"`,
		`data-quick-context="prod"`,
		`Pods · default · Running`,
		`quick-switcher-meta-token`,
		`quick-switcher-meta-icon icon-[uil--cube]`,
		`quick-switcher-meta-icon icon-[uil--folder-network]`,
		`quick-switcher-meta-icon icon-[lucide--circle-check]`,
		`data-on:click="$context = &#34;prod&#34;`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("quick switcher did not render %q: %s", want, body)
		}
	}
}

func TestHandleIndexOmitsClusterSummaryOnResourcePages(t *testing.T) {
	app := New([]*kube.Cluster{testCluster()}, context.Background(), nil)
	req := httptest.NewRequest(http.MethodGet, "/?resource=pods", nil)
	res := httptest.NewRecorder()

	app.handleIndex(res, req)

	body := res.Body.String()
	if strings.Contains(body, `id="summary"`) {
		t.Fatalf("resource page rendered overview summary cards")
	}
	if strings.Contains(body, `data-on-interval__duration.5s="@get(&#39;/ui/summary&#39;)"`) {
		t.Fatalf("resource page rendered summary auto-refresh interval")
	}
	if !strings.Contains(body, `data-on-interval__duration.5s="@get(&#39;/ui/table?refresh=auto&#39;)"`) {
		t.Fatalf("resource page did not keep table auto-refresh interval")
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
	if !strings.Contains(body, `data: signals {"context":"","clusters":"","resource":"storageclasses","namespace":"","query":"","sortColumn":"","sortOrder":"","selectedName":"fast","selectedNamespace":"","detailMode":"overview"}`) {
		t.Fatalf("table response did not patch normalized signal state:\n%s", body)
	}
	if !strings.Contains(body, "event: datastar-patch-elements") {
		t.Fatalf("table response did not patch elements")
	}
}

func TestHandleTablePatchesPageChromeForNavigation(t *testing.T) {
	app := New([]*kube.Cluster{testCluster()}, context.Background(), nil)
	req := httptest.NewRequest(http.MethodGet, "/ui/table?resource=deployments", nil)
	res := httptest.NewRecorder()

	app.handleTable(res, req)

	body := res.Body.String()
	for _, want := range []string{
		`id="page-title"`,
		`>Deployments</h1>`,
		`id="summary-slot"`,
		`id="resource-controls"`,
		`aria-label="Table search"`,
		`id="query"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("table navigation response did not patch %q:\n%s", want, body)
		}
	}
	for _, unwanted := range []string{
		`id="namespace-picker"`,
		`aria-label="Refresh"`,
	} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("table navigation response patched removed control %q:\n%s", unwanted, body)
		}
	}
}

func TestHandleTableSkipsQuickSwitcherForAutoRefresh(t *testing.T) {
	app := New([]*kube.Cluster{testCluster()}, context.Background(), nil)
	req := httptest.NewRequest(http.MethodGet, "/ui/table?resource=deployments&refresh=auto", nil)
	res := httptest.NewRecorder()

	app.handleTable(res, req)

	body := res.Body.String()
	if strings.Contains(body, `id="quick-switcher"`) {
		t.Fatalf("auto-refresh table response patched quick switcher:\n%s", body)
	}
	if !strings.Contains(body, `id="content-grid"`) {
		t.Fatalf("auto-refresh table response did not patch content:\n%s", body)
	}
}

func TestMultiClusterTableAggregatesRowsWithClusterColumn(t *testing.T) {
	app := New([]*kube.Cluster{
		testClusterWithPod("dev", "api"),
		testClusterWithPod("prod", "worker"),
	}, context.Background(), nil)
	req := httptest.NewRequest(http.MethodGet, "/ui/table?resource=pods", nil)
	signals := readSignals(req)

	state := app.state(signals)

	if state.Signals.Clusters != "" {
		t.Fatalf("clusters signal = %q, want empty no-filter state", state.Signals.Clusters)
	}
	if len(state.Table.Columns) == 0 || state.Table.Columns[0] != "Cluster" {
		t.Fatalf("first table column = %#v, want Cluster", state.Table.Columns)
	}
	if len(state.Table.Rows) != 2 {
		t.Fatalf("rows = %d, want 2: %#v", len(state.Table.Rows), state.Table.Rows)
	}
	seen := map[string]string{}
	for _, row := range state.Table.Rows {
		seen[row.Name] = row.Cluster
		if len(row.Cells) == 0 || row.Cells[0].Value != row.Cluster {
			t.Fatalf("row %q did not expose cluster cell: %#v", row.Name, row.Cells)
		}
	}
	if seen["api"] != "dev" || seen["worker"] != "prod" {
		t.Fatalf("row cluster mapping = %#v, want api/dev and worker/prod", seen)
	}
}

func TestMultiClusterTableDoesNotFilterByClusterOrNodeContext(t *testing.T) {
	devPod := testPod("api", corev1.PodRunning)
	devPod.Spec.NodeName = "dev"
	prodPod := testPod("worker", corev1.PodRunning)
	prodPod.Spec.NodeName = "prod"
	app := New([]*kube.Cluster{
		testClusterWithPods("dev", devPod),
		testClusterWithPods("prod", prodPod),
	}, context.Background(), nil)
	req := httptest.NewRequest(http.MethodGet, "/ui/table?resource=pods&query=prod", nil)

	state := app.state(readSignals(req))

	if len(state.Table.Rows) != 0 {
		t.Fatalf("rows = %d, want 0 because cluster/node context is not searched: %#v", len(state.Table.Rows), state.Table.Rows)
	}
}

func TestMultiClusterTableFiltersByResourceName(t *testing.T) {
	app := New([]*kube.Cluster{
		testClusterWithPod("dev", "api"),
		testClusterWithPod("prod", "worker"),
	}, context.Background(), nil)
	req := httptest.NewRequest(http.MethodGet, "/ui/table?resource=pods&query=worker", nil)

	state := app.state(readSignals(req))

	if len(state.Table.Rows) != 1 {
		t.Fatalf("rows = %d, want 1: %#v", len(state.Table.Rows), state.Table.Rows)
	}
	if state.Table.Rows[0].Name != "worker" || state.Table.Rows[0].Cluster != "prod" {
		t.Fatalf("filtered row = %#v, want prod worker", state.Table.Rows[0])
	}
}

func TestMultiClusterDeploymentTableDoesNotFilterByNamespace(t *testing.T) {
	app := New([]*kube.Cluster{
		testClusterWithObjects("dev",
			testDeployment("alice-backend", "chatto-dev"),
			testDeployment("chatto-hub", "platform"),
		),
		testClusterWithObjects("prod",
			testDeployment("bob-backend", "chatto-dev"),
		),
	}, context.Background(), nil)
	req := httptest.NewRequest(http.MethodGet, "/ui/table?resource=deployments&query=chatto", nil)

	state := app.state(readSignals(req))

	if got, want := tableRowNames(state.Table.Rows), []string{"chatto-hub"}; !equalStringSlices(got, want) {
		t.Fatalf("filtered deployment rows = %#v, want %#v", got, want)
	}
}

func TestMultiClusterTableKeepsClusterColumnWhenOneClusterIsActive(t *testing.T) {
	app := New([]*kube.Cluster{
		testClusterWithPod("dev", "api"),
		testClusterWithPod("prod", "worker"),
	}, context.Background(), nil)
	req := httptest.NewRequest(http.MethodGet, "/ui/table?resource=pods&clusters=prod", nil)

	state := app.state(readSignals(req))

	if state.Signals.Clusters != "prod" {
		t.Fatalf("clusters signal = %q, want prod", state.Signals.Clusters)
	}
	if len(state.Table.Columns) == 0 || state.Table.Columns[0] != "Cluster" {
		t.Fatalf("first table column = %#v, want Cluster", state.Table.Columns)
	}
	if len(state.Table.Rows) != 1 || state.Table.Rows[0].Cluster != "prod" {
		t.Fatalf("rows = %#v, want one prod row", state.Table.Rows)
	}
}

func TestHandleIndexRendersClusterFiltersForMultiClusterTables(t *testing.T) {
	app := New([]*kube.Cluster{
		testClusterWithPod("dev", "api"),
		testClusterWithPod("prod", "worker"),
	}, context.Background(), nil)
	req := httptest.NewRequest(http.MethodGet, "/?resource=pods", nil)
	res := httptest.NewRecorder()

	app.handleIndex(res, req)

	body := res.Body.String()
	for _, want := range []string{
		`id="cluster-filters"`,
		`class="cluster-filter"`,
		`dev`,
		`prod`,
		`data-row-cluster="dev"`,
		`data-row-cluster="prod"`,
		`cluster-context="dev"`,
		`cluster-context="prod"`,
		`data-cluster-context="dev"`,
		`data-cluster-context="prod"`,
		`data-cluster-selection="dev"`,
		`data-cluster-selection="prod"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("multi-cluster table did not render %q: %s", want, body)
		}
	}
	if strings.Contains(body, `class="cluster-filter active"`) {
		t.Fatalf("multi-cluster no-filter state rendered active cluster pill: %s", body)
	}
}

func TestHandleIndexRendersExplicitClusterFilterSelection(t *testing.T) {
	app := New([]*kube.Cluster{
		testClusterWithPod("dev", "api"),
		testClusterWithPod("prod", "worker"),
	}, context.Background(), nil)
	req := httptest.NewRequest(http.MethodGet, "/?resource=pods&clusters=dev", nil)
	res := httptest.NewRecorder()

	app.handleIndex(res, req)

	body := res.Body.String()
	for _, want := range []string{
		`class="cluster-filter active"`,
		`data-cluster-selection=""`,
		`data-cluster-selection="dev,prod"`,
		`data-row-cluster="dev"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("explicit cluster filter did not render %q: %s", want, body)
		}
	}
	if strings.Contains(body, `data-row-cluster="prod"`) {
		t.Fatalf("explicit dev cluster filter rendered prod rows: %s", body)
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
	if !strings.Contains(body, `data-on-interval__duration.5s="@get(&#39;/ui/table?refresh=auto&#39;)"`) {
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
	if strings.Contains(body, `data-on-interval__duration.5s="@get(&#39;/ui/table?refresh=auto&#39;)"`) {
		t.Fatalf("index rendered table auto-refresh interval on overview")
	}
}

func TestHandleIndexRendersLazyOverviewChartShell(t *testing.T) {
	app := New([]*kube.Cluster{testCluster()}, context.Background(), nil)
	req := httptest.NewRequest("GET", "/", nil)
	res := httptest.NewRecorder()

	app.handleIndex(res, req)

	body := res.Body.String()
	if !strings.Contains(body, `id="overview-pod-usage-panel"`) {
		t.Fatalf("index did not render pod usage panel shell")
	}
	if !strings.Contains(body, `/ui/charts/prometheus`) {
		t.Fatalf("index did not render pod usage lazy-load action")
	}
	if strings.Contains(body, `cpu=`) || strings.Contains(body, `memory=`) {
		t.Fatalf("index rendered PromQL query parameters")
	}
	if !strings.Contains(body, `data-on-intersect__once=`) {
		t.Fatalf("index did not render chart viewport-load hook")
	}
	if !strings.Contains(body, "Loading pod usage timelines") {
		t.Fatalf("index did not render pod usage loading state")
	}
}

func TestPodTableRendersLazySparklineCells(t *testing.T) {
	app := New([]*kube.Cluster{testCluster()}, context.Background(), nil)
	req := httptest.NewRequest(http.MethodGet, "/ui/table?resource=pods&namespace=default", nil)
	res := httptest.NewRecorder()

	app.Routes().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	body := res.Body.String()
	for _, want := range []string{
		`<kubernetto-promql-sparkline`,
		`data-ignore-morph`,
		`cluster-context="test"`,
		`pod-namespace="default"`,
		`pod-name="api"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("pod table did not render lazy sparkline marker %q: %s", want, body)
		}
	}
	if strings.Contains(body, `cpu-query=`) || strings.Contains(body, `memory-query=`) {
		t.Fatalf("pod table rendered PromQL sparkline attributes: %s", body)
	}
}

func TestPodTableDimsSucceededRows(t *testing.T) {
	app := New([]*kube.Cluster{
		testClusterWithPods("test",
			testPod("api", corev1.PodRunning),
			testPod("backup", corev1.PodSucceeded),
		),
	}, context.Background(), nil)
	req := httptest.NewRequest(http.MethodGet, "/ui/table?resource=pods&namespace=default", nil)
	res := httptest.NewRecorder()

	app.Routes().ServeHTTP(res, req)

	body := res.Body.String()
	for _, want := range []string{
		`data-row-name="api"`,
		`data-row-name="backup"`,
		`class="terminal-success"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("pod table did not render %q: %s", want, body)
		}
	}
	if count := strings.Count(body, `class="terminal-success"`); count != 1 {
		t.Fatalf("terminal success row count = %d, want 1: %s", count, body)
	}
}

func TestChartEndpointsRenderPatchFragments(t *testing.T) {
	app := New(nil, context.Background(), nil)
	detailQuery := url.Values{
		"panel":     {"pod-usage-detail"},
		"namespace": {"prod"},
		"name":      {"api"},
	}
	tests := []struct {
		path string
		body string
	}{
		{path: "/ui/charts/prometheus", body: "overview-pod-usage-panel"},
		{path: "/ui/charts/prometheus?" + detailQuery.Encode(), body: "detail-pod-usage-prod-api"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			res := httptest.NewRecorder()

			app.Routes().ServeHTTP(res, req)

			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
			}
			if body := res.Body.String(); !strings.Contains(body, tt.body) {
				t.Fatalf("body did not contain %q: %s", tt.body, body)
			}
		})
	}
}

func TestPrometheusPodUsageRangeEndpointReturnsCompressedJSON(t *testing.T) {
	app := New(nil, context.Background(), nil)
	requestBody := `{"pods":[{"namespace":"default","pod":"api"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/prometheus/pod-usage-range", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	res := httptest.NewRecorder()

	app.Routes().ServeHTTP(res, req)

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusServiceUnavailable)
	}
	if got := res.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content encoding = %q, want gzip", got)
	}
	reader, err := gzip.NewReader(res.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read gzip body: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close gzip reader: %v", err)
	}
	var payload kube.PrometheusRangeData
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Available {
		t.Fatalf("payload unexpectedly available: %#v", payload)
	}
	if payload.Message != "No Kubernetes client is configured." {
		t.Fatalf("message = %q", payload.Message)
	}
}

func TestPrometheusPodUsageRangeEndpointRequiresPods(t *testing.T) {
	app := New(nil, context.Background(), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/prometheus/pod-usage-range", strings.NewReader(`{"queries":[{"name":"cpu","query":"up"}]}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	app.Routes().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusBadRequest)
	}
	if body := res.Body.String(); !strings.Contains(body, "at least one pod is required") {
		t.Fatalf("body did not contain pod validation error: %s", body)
	}
}

func TestPrometheusQueryRangeEndpointIsNotRegistered(t *testing.T) {
	app := New(nil, context.Background(), nil)
	req := httptest.NewRequest(http.MethodPost, "/ui/prometheus/query-range", strings.NewReader(`{"queries":[{"name":"cpu","query":"up"}]}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	app.Routes().ServeHTTP(res, req)

	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusMethodNotAllowed)
	}
}

func testCluster() *kube.Cluster {
	return testClusterWithPod("test", "api")
}

func testClusterWithPod(contextName, podName string) *kube.Cluster {
	return testClusterWithPods(contextName, testPod(podName, corev1.PodRunning))
}

func testClusterWithPods(contextName string, pods ...*corev1.Pod) *kube.Cluster {
	objects := make([]runtime.Object, 0, len(pods))
	for _, pod := range pods {
		objects = append(objects, pod)
	}
	return testClusterWithObjects(contextName, objects...)
}

func testClusterWithObjects(contextName string, extraObjects ...runtime.Object) *kube.Cluster {
	objects := []runtime.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	}
	objects = append(objects, extraObjects...)
	clientset := fake.NewSimpleClientset(objects...)
	return &kube.Cluster{
		Clientset:    clientset,
		Discovery:    clientset.Discovery(),
		ContextName:  contextName,
		ClusterName:  contextName,
		Namespace:    "default",
		ConfigSource: "test",
		Current:      contextName == "test" || contextName == "dev",
	}
}

func testNode(name string, ready bool) *corev1.Node {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: status},
			},
		},
	}
}

func testPod(name string, phase corev1.PodPhase) *corev1.Pod {
	ready := phase == corev1.PodRunning
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: name}}},
		Status: corev1.PodStatus{
			Phase: phase,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: name, Ready: ready},
			},
		},
	}
}

func testDeployment(name, namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: name}}},
			},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
	}
}

func int32Ptr(value int32) *int32 {
	return &value
}

func tableRowNames(rows []kube.Row) []string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	return names
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func detailFieldValue(fields []kube.DetailField, name string) string {
	for _, field := range fields {
		if field.Name == name {
			return field.Value
		}
	}
	return ""
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
		{
			name:        "bundled datastar asset",
			path:        "/assets/datastar.js",
			contentType: "text/javascript",
			body:        "Datastar v1.0.2",
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
