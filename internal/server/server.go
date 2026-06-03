package server

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/starfederation/datastar-go/datastar"

	"kubernetto/internal/kube"
	"kubernetto/internal/server/ui"
)

type Server struct {
	clusters       []*clusterSession
	clustersByName map[string]*clusterSession
	defaultContext string
	ctx            context.Context
	logger         *slog.Logger
	mu             sync.Mutex
}

type clusterSession struct {
	cluster           *kube.Cluster
	store             *kube.ResourceStore
	initialSyncWaited bool
}

func New(clusters []*kube.Cluster, ctx context.Context, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if ctx == nil {
		ctx = context.Background()
	}

	app := &Server{
		clusters:       make([]*clusterSession, 0, len(clusters)),
		clustersByName: make(map[string]*clusterSession, len(clusters)),
		ctx:            ctx,
		logger:         logger,
	}
	for _, cluster := range clusters {
		if cluster == nil || cluster.ContextName == "" {
			continue
		}
		session := &clusterSession{cluster: cluster}
		app.clusters = append(app.clusters, session)
		app.clustersByName[cluster.ContextName] = session
		if app.defaultContext == "" || cluster.Current {
			app.defaultContext = cluster.ContextName
		}
	}
	for _, session := range app.clusters {
		app.ensureSessionStore(session)
	}
	return app
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /assets/", ui.HandleAssets)
	mux.HandleFunc("GET /ui/refresh", s.handleRefresh)
	mux.HandleFunc("GET /ui/summary", s.handleSummary)
	mux.HandleFunc("GET /ui/table", s.handleTable)
	mux.HandleFunc("GET /ui/selection", s.handleSelection)
	mux.HandleFunc("GET /ui/detail", s.handleDetail)
	mux.HandleFunc("GET /ui/charts/prometheus", s.handlePrometheusChart)
	mux.HandleFunc("POST /ui/prometheus/query-range", s.handlePrometheusQueryRange)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	return withSecurityHeaders(mux)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	state := s.state(readSignals(r))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ui.RenderPage(w, state); err != nil {
		s.logger.Error("render index", "error", err)
	}
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	signals := readSignals(r)
	state := s.state(signals)
	sse := datastar.NewSSE(w, r)
	s.patchSignals(sse, state.Signals)
	if r.URL.Query().Get("refresh") != "auto" {
		s.patchElements(sse, "quick switcher", ui.RenderFragment(ui.QuickSwitcherView(state)))
	}
	s.patchElements(sse, "page title", ui.RenderFragment(ui.PageTitleView(state)))
	s.patchElements(sse, "resource nav", ui.RenderFragment(ui.ResourceNavView(state)))
	s.patchElements(sse, "summary slot", ui.RenderFragment(ui.SummarySlotView(state)))
	s.patchElements(sse, "resource controls", ui.RenderFragment(ui.ResourceControlsView(state)))
	s.patchElements(sse, "content", ui.RenderFragment(ui.ContentView(state)))
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	signals := readSignals(r)
	state := s.state(signals)
	sse := datastar.NewSSE(w, r)
	s.patchSignals(sse, state.Signals)
	s.patchElements(sse, "summary slot", ui.RenderFragment(ui.SummarySlotView(state)))
}

func (s *Server) handleTable(w http.ResponseWriter, r *http.Request) {
	signals := readSignals(r)
	state := s.state(signals)
	sse := datastar.NewSSE(w, r)
	s.patchSignals(sse, state.Signals)
	if r.URL.Query().Get("refresh") != "auto" {
		s.patchElements(sse, "quick switcher", ui.RenderFragment(ui.QuickSwitcherView(state)))
	}
	s.patchElements(sse, "page title", ui.RenderFragment(ui.PageTitleView(state)))
	s.patchElements(sse, "resource nav", ui.RenderFragment(ui.ResourceNavView(state)))
	s.patchElements(sse, "summary slot", ui.RenderFragment(ui.SummarySlotView(state)))
	s.patchElements(sse, "resource controls", ui.RenderFragment(ui.ResourceControlsView(state)))
	s.patchElements(sse, "content", ui.RenderFragment(ui.ContentView(state)))
}

func (s *Server) handleSelection(w http.ResponseWriter, r *http.Request) {
	signals := readSignals(r)
	state := s.state(signals)
	sse := datastar.NewSSE(w, r)
	s.patchSignals(sse, state.Signals)
	s.patchElements(sse, "quick switcher", ui.RenderFragment(ui.QuickSwitcherView(state)))
	s.patchElements(sse, "content", ui.RenderFragment(ui.ContentView(state)))
}

func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	signals := readSignals(r)
	state := s.state(signals)
	sse := datastar.NewSSE(w, r)
	s.patchSignals(sse, state.Signals)
	s.patchElements(sse, "detail", ui.RenderFragment(ui.DetailView(state)))
}

func (s *Server) handlePrometheusChart(w http.ResponseWriter, r *http.Request) {
	signals := readSignals(r)
	params := r.URL.Query()
	session := s.session(signals.Context)
	sse := datastar.NewSSE(w, r)
	switch chartPanel(params.Get("panel"), params.Get("view")) {
	case "pod-usage-detail":
		namespace := firstNonEmpty(params.Get("namespace"), signals.SelectedNamespace)
		name := firstNonEmpty(params.Get("name"), signals.SelectedName)
		chart := kube.PodUsageDetailChart{
			Name:      name,
			Namespace: namespace,
			Metrics:   unavailableChartMetrics("No Kubernetes client is configured."),
		}
		if session != nil && session.store != nil && name != "" {
			chart = session.store.PodUsageDetailChart(namespace, name, params.Get("cpu"), params.Get("memory"))
		}
		sse.PatchElements(ui.RenderFragment(ui.DetailPodUsagePanel(chart)))
		return
	}

	chart := kube.PodUsageOverviewChart{Metrics: unavailableChartMetrics("No Kubernetes client is configured.")}
	if session != nil && session.store != nil {
		chart = session.store.PodUsageOverviewChart(params.Get("cpu"), params.Get("memory"), chartLimit(params.Get("limit")))
	}
	sse.PatchElements(ui.RenderFragment(ui.OverviewPodUsagePanel(chart)))
}

type prometheusQueryRangeRequest struct {
	Context       string                      `json:"context"`
	Queries       []kube.PrometheusRangeQuery `json:"queries"`
	WindowSeconds int                         `json:"windowSeconds"`
	StepSeconds   int                         `json:"stepSeconds"`
}

const prometheusQueryRangeMaxQueries = 12

func (s *Server) handlePrometheusQueryRange(w http.ResponseWriter, r *http.Request) {
	var request prometheusQueryRangeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&request); err != nil {
		writeCompressedJSON(w, r, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body"})
		return
	}
	if len(request.Queries) == 0 {
		writeCompressedJSON(w, r, http.StatusBadRequest, map[string]string{"error": "at least one PromQL query is required"})
		return
	}
	if len(request.Queries) > prometheusQueryRangeMaxQueries {
		writeCompressedJSON(w, r, http.StatusBadRequest, map[string]string{"error": "at most twelve PromQL queries can be loaded at once"})
		return
	}

	session := s.session(request.Context)
	if session == nil || session.store == nil {
		writeCompressedJSON(w, r, http.StatusServiceUnavailable, kube.PrometheusRangeData{
			Message:   "No Kubernetes client is configured.",
			Window:    "Last 60 minutes",
			UpdatedAt: time.Now(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	data, err := session.store.PrometheusRangeData(ctx, request.Queries, chartWindow(request.WindowSeconds), chartStep(request.StepSeconds))
	if err != nil {
		data.Message = err.Error()
		writeCompressedJSON(w, r, http.StatusOK, data)
		return
	}
	writeCompressedJSON(w, r, http.StatusOK, data)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) patchSignals(sse *datastar.ServerSentEventGenerator, signals ui.Signals) {
	if err := sse.MarshalAndPatchSignals(signals); err != nil {
		s.logger.Warn("patch datastar signals", "error", err)
	}
}

func (s *Server) patchElements(sse *datastar.ServerSentEventGenerator, label, elements string) {
	if err := sse.PatchElements(elements); err != nil {
		s.logger.Warn("patch datastar elements", "fragment", label, "error", err)
	}
}

func (s *Server) state(signals ui.Signals) ui.PageState {
	kind := kube.NormalizeKind(signals.Resource)
	namespace := signals.Namespace
	detailMode := normalizeDetailMode(signals.DetailMode)
	session := s.session(signals.Context)
	contextName := ""
	if session != nil && session.cluster != nil {
		contextName = session.cluster.ContextName
	}

	def := resourceDef(kind)
	if def.Scope == "cluster" {
		namespace = ""
	}
	selectedNamespace := signals.SelectedNamespace
	if def.Scope == "cluster" {
		selectedNamespace = ""
	}
	if isStandalonePageKind(kind) {
		namespace = ""
		selectedNamespace = ""
		signals.SelectedName = ""
		signals.SelectedNamespace = ""
		signals.SortColumn = ""
		signals.SortOrder = ""
	}
	activeContexts := s.activeContexts(signals.Clusters)
	signals.Clusters = strings.Join(s.selectedContexts(signals.Clusters), ",")
	if isStandalonePageKind(kind) && len(activeContexts) == 1 {
		session = s.session(activeContexts[0])
		if session != nil && session.cluster != nil {
			contextName = session.cluster.ContextName
		}
	}

	summary := kube.Summary{UpdatedAt: time.Now(), Error: "No Kubernetes client is configured."}
	fleet := ui.FleetOverview{UpdatedAt: time.Now()}
	actions := ui.ActionList{UpdatedAt: time.Now(), Error: "No Kubernetes client is configured."}
	overview := kube.ClusterOverview{UpdatedAt: time.Now(), Error: "No Kubernetes client is configured."}
	table := kube.Table{Kind: kind, Label: def.Label, Namespace: namespace, Query: signals.Query, SortColumn: signals.SortColumn, SortOrder: signals.SortOrder, UpdatedAt: time.Now(), Namespaced: def.Scope == "namespaced"}
	detail := kube.ResourceDetail{Kind: kind, Label: def.Label, Name: signals.SelectedName, Namespace: selectedNamespace}
	namespaces := []string{}
	quickItems := []ui.QuickSwitcherItem{}
	namespaceErr := ""

	if session != nil && session.store != nil {
		summary = s.summary(activeContexts)
		fleet = s.fleetOverview(activeContexts)
		actions = s.actionItems(activeContexts)
		overview = session.store.Overview()
		if !isStandalonePageKind(kind) {
			table = s.table(kind, namespace, signals.Query, signals.SortColumn, signals.SortOrder, activeContexts)
			detail = session.store.Detail(kind, selectedNamespace, signals.SelectedName)
		}
		var err error
		namespaces, err = s.namespaces(activeContexts)
		if err != nil {
			namespaceErr = err.Error()
		}
		quickItems = s.quickSwitcherObjectItems(stateQuickSignals(contextName, signals, namespace, table), activeContexts)
	}

	return ui.PageState{
		Cluster:        sessionCluster(session),
		Clusters:       s.clusterList(),
		ActiveContexts: activeContexts,
		Resources:      kube.ResourceDefs,
		QuickItems:     quickItems,
		Signals:        ui.Signals{Context: contextName, Clusters: signals.Clusters, Resource: string(kind), Namespace: namespace, Query: signals.Query, SortColumn: table.SortColumn, SortOrder: table.SortOrder, SelectedName: signals.SelectedName, SelectedNamespace: selectedNamespace, DetailMode: detailMode},
		Summary:        summary,
		Fleet:          fleet,
		Actions:        actions,
		Overview:       overview,
		Table:          table,
		Detail:         detail,
		Namespaces:     namespaces,
		NamespaceErr:   namespaceErr,
	}
}

func stateQuickSignals(contextName string, signals ui.Signals, namespace string, table kube.Table) ui.Signals {
	return ui.Signals{
		Context:           contextName,
		Clusters:          signals.Clusters,
		Resource:          string(table.Kind),
		Namespace:         namespace,
		Query:             signals.Query,
		SortColumn:        table.SortColumn,
		SortOrder:         table.SortOrder,
		SelectedName:      signals.SelectedName,
		SelectedNamespace: signals.SelectedNamespace,
		DetailMode:        signals.DetailMode,
	}
}

func readSignals(r *http.Request) ui.Signals {
	signals := ui.Signals{Resource: string(kube.KindOverview)}
	if err := datastar.ReadSignals(r, &signals); err != nil && !errors.Is(err, http.ErrNoCookie) {
		// Datastar omits signals on plain browser requests. Query parameters keep
		// endpoints easy to hit directly while the UI sends reactive signals.
	}
	q := r.URL.Query()
	if contextName := q.Get("context"); contextName != "" {
		signals.Context = contextName
	}
	if clusters := q.Get("clusters"); clusters != "" {
		signals.Clusters = clusters
	}
	if resource := q.Get("resource"); resource != "" {
		signals.Resource = resource
	}
	if namespace := q.Get("namespace"); namespace != "" {
		signals.Namespace = namespace
	}
	if query := q.Get("query"); query != "" {
		signals.Query = query
	}
	if sortColumn := q.Get("sortColumn"); sortColumn != "" {
		signals.SortColumn = sortColumn
	}
	if sortOrder := q.Get("sortOrder"); sortOrder != "" {
		signals.SortOrder = sortOrder
	}
	if selectedName := q.Get("selectedName"); selectedName != "" {
		signals.SelectedName = selectedName
	}
	if selectedNamespace := q.Get("selectedNamespace"); selectedNamespace != "" {
		signals.SelectedNamespace = selectedNamespace
	}
	if detailMode := q.Get("detailMode"); detailMode != "" {
		signals.DetailMode = detailMode
	}
	if signals.Resource == "" {
		signals.Resource = string(kube.KindOverview)
	}
	signals.DetailMode = normalizeDetailMode(signals.DetailMode)
	return signals
}

func normalizeDetailMode(mode string) string {
	switch mode {
	case "overview", "events", "yaml":
		return mode
	default:
		return "overview"
	}
}

func (s *Server) session(contextName string) *clusterSession {
	if contextName == "" {
		contextName = s.defaultContext
	}
	session := s.ensureStore(contextName)
	s.waitForInitialSync(session)
	return session
}

func (s *Server) ensureStore(contextName string) *clusterSession {
	if contextName == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session := s.clustersByName[contextName]
	if session == nil {
		session = s.clustersByName[s.defaultContext]
	}
	if session == nil {
		return nil
	}
	s.ensureSessionStore(session)
	return session
}

func (s *Server) ensureSessionStore(session *clusterSession) {
	if session == nil || session.store != nil {
		return
	}
	session.store = kube.NewResourceStore(session.cluster, s.logger)
	session.store.Start(s.ctx)
}

func (s *Server) waitForInitialSync(session *clusterSession) {
	if session == nil || session.store == nil {
		return
	}

	s.mu.Lock()
	if session.initialSyncWaited {
		s.mu.Unlock()
		return
	}
	session.initialSyncWaited = true
	store := session.store
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	store.WaitForSync(ctx)
}

func (s *Server) waitForInitialSyncs(sessions []*clusterSession) {
	pending := make([]*clusterSession, 0, len(sessions))
	s.mu.Lock()
	for _, session := range sessions {
		if session == nil || session.store == nil || session.initialSyncWaited {
			continue
		}
		session.initialSyncWaited = true
		pending = append(pending, session)
	}
	s.mu.Unlock()
	if len(pending) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for _, session := range pending {
		wg.Add(1)
		go func(store *kube.ResourceStore) {
			defer wg.Done()
			store.WaitForSync(ctx)
		}(session.store)
	}
	wg.Wait()
}

func (s *Server) sessionsForContexts(contexts []string) []*clusterSession {
	sessions := make([]*clusterSession, 0, len(contexts))
	seen := map[string]bool{}
	for _, contextName := range contexts {
		if seen[contextName] {
			continue
		}
		seen[contextName] = true
		session := s.ensureStore(contextName)
		if session != nil {
			sessions = append(sessions, session)
		}
	}
	return sessions
}

func (s *Server) activeContexts(value string) []string {
	selected := s.selectedContexts(value)
	if len(selected) > 0 {
		return selected
	}
	contexts := s.allContexts()
	if len(contexts) == 0 && s.defaultContext != "" {
		contexts = append(contexts, s.defaultContext)
	}
	return contexts
}

func (s *Server) selectedContexts(value string) []string {
	selected := map[string]bool{}
	for _, contextName := range strings.Split(value, ",") {
		contextName = strings.TrimSpace(contextName)
		if contextName != "" {
			selected[contextName] = true
		}
	}
	if len(selected) == 0 {
		return nil
	}
	contexts := make([]string, 0, len(selected))
	for _, session := range s.clusters {
		if session == nil || session.cluster == nil {
			continue
		}
		if selected[session.cluster.ContextName] {
			contexts = append(contexts, session.cluster.ContextName)
		}
	}
	return contexts
}

func (s *Server) allContexts() []string {
	contexts := make([]string, 0, len(s.clusters))
	for _, session := range s.clusters {
		if session != nil && session.cluster != nil {
			contexts = append(contexts, session.cluster.ContextName)
		}
	}
	return contexts
}

func (s *Server) summary(contexts []string) kube.Summary {
	sessions := s.sessionsForContexts(contexts)
	s.waitForInitialSyncs(sessions)
	out := kube.Summary{UpdatedAt: time.Now()}
	if len(sessions) == 0 {
		out.Error = "No Kubernetes client is configured."
		return out
	}
	if len(sessions) == 1 && sessions[0].store != nil {
		return sessions[0].store.Summary()
	}

	errors := []string{}
	for _, session := range sessions {
		if session.store == nil {
			continue
		}
		summary := session.store.Summary()
		out.Nodes += summary.Nodes
		out.Namespaces += summary.Namespaces
		out.Pods += summary.Pods
		out.Deployments += summary.Deployments
		if summary.Error != "" {
			errors = append(errors, session.cluster.ContextName+": "+summary.Error)
		}
	}
	out.Context = fmt.Sprintf("%d clusters", len(sessions))
	out.ServerVersion = out.Context
	out.Error = strings.Join(errors, "\n")
	return out
}

func (s *Server) fleetOverview(contexts []string) ui.FleetOverview {
	sessions := s.sessionsForContexts(contexts)
	s.waitForInitialSyncs(sessions)
	out := ui.FleetOverview{UpdatedAt: time.Now()}
	for _, session := range sessions {
		if session == nil || session.cluster == nil || session.store == nil {
			continue
		}
		overview := session.store.Overview()
		card := fleetClusterFromOverview(session.cluster.ContextName, overview)
		out.Clusters = append(out.Clusters, card)
		for _, event := range overview.WarningEvents {
			out.WarningEvents = append(out.WarningEvents, ui.FleetWarningEvent{
				Cluster: session.cluster.ContextName,
				Event:   event,
			})
		}
		if overview.UpdatedAt.After(out.UpdatedAt) {
			out.UpdatedAt = overview.UpdatedAt
		}
	}
	sort.SliceStable(out.Clusters, func(i, j int) bool {
		left := fleetStatusRank(out.Clusters[i].StatusKey)
		right := fleetStatusRank(out.Clusters[j].StatusKey)
		if left != right {
			return left < right
		}
		if out.Clusters[i].WarningCount != out.Clusters[j].WarningCount {
			return out.Clusters[i].WarningCount > out.Clusters[j].WarningCount
		}
		return strings.ToLower(out.Clusters[i].Context) < strings.ToLower(out.Clusters[j].Context)
	})
	sort.SliceStable(out.WarningEvents, func(i, j int) bool {
		return out.WarningEvents[i].Event.LastSeen.After(out.WarningEvents[j].Event.LastSeen)
	})
	if len(out.WarningEvents) > 10 {
		out.WarningEvents = out.WarningEvents[:10]
	}
	return out
}

func (s *Server) actionItems(contexts []string) ui.ActionList {
	sessions := s.sessionsForContexts(contexts)
	s.waitForInitialSyncs(sessions)
	out := ui.ActionList{UpdatedAt: time.Now()}
	if len(sessions) == 0 {
		out.Error = "No Kubernetes client is configured."
		return out
	}
	for _, session := range sessions {
		if session == nil || session.cluster == nil || session.store == nil {
			continue
		}
		overview := session.store.Overview()
		if overview.UpdatedAt.After(out.UpdatedAt) {
			out.UpdatedAt = overview.UpdatedAt
		}
		contextName := session.cluster.ContextName
		if overview.Error != "" {
			out.Items = append(out.Items, ui.ActionItem{
				Context:     contextName,
				Title:       "Cluster data unavailable",
				Detail:      overview.Error,
				StatusKey:   "danger",
				TargetKind:  kube.KindOverview,
				ActionLabel: "Open overview",
			})
			continue
		}
		for _, event := range overview.WarningEvents {
			query := firstNonEmpty(event.Reason, event.InvolvedObject)
			out.Items = append(out.Items, ui.ActionItem{
				Context:         contextName,
				Title:           firstNonEmpty(event.Reason, "Warning event"),
				Detail:          eventDetail(event),
				Message:         event.Message,
				Namespace:       event.Namespace,
				Object:          event.InvolvedObject,
				Reason:          event.Reason,
				Age:             event.Age,
				Count:           event.Count,
				StatusKey:       "warn",
				TargetKind:      kube.KindEvents,
				TargetQuery:     query,
				TargetName:      event.Name,
				TargetNamespace: event.Namespace,
				ActionLabel:     "Open events",
				LastSeen:        event.LastSeen,
			})
		}
		for _, metric := range []kube.OverviewMetric{
			overviewMetric(overview.Stats, "Nodes ready"),
			overviewMetric(overview.Stats, "Pods healthy"),
			overviewMetric(overview.Stats, "Workloads ready"),
		} {
			if metric.StatusKey != "warn" && metric.StatusKey != "danger" {
				continue
			}
			out.Items = append(out.Items, ui.ActionItem{
				Context:     contextName,
				Title:       metric.Label + " " + metric.Value,
				Detail:      firstNonEmpty(metric.Detail, "Current readiness is degraded."),
				StatusKey:   metric.StatusKey,
				TargetKind:  metric.Kind,
				ActionLabel: "Open " + strings.ToLower(resourceDef(metric.Kind).Label),
			})
		}
	}
	sort.SliceStable(out.Items, func(i, j int) bool {
		left := fleetStatusRank(out.Items[i].StatusKey)
		right := fleetStatusRank(out.Items[j].StatusKey)
		if left != right {
			return left < right
		}
		if !out.Items[i].LastSeen.Equal(out.Items[j].LastSeen) {
			return out.Items[i].LastSeen.After(out.Items[j].LastSeen)
		}
		if out.Items[i].Context != out.Items[j].Context {
			return strings.ToLower(out.Items[i].Context) < strings.ToLower(out.Items[j].Context)
		}
		return strings.ToLower(out.Items[i].Title) < strings.ToLower(out.Items[j].Title)
	})
	return out
}

func eventDetail(event kube.OverviewEvent) string {
	parts := []string{}
	if event.InvolvedObject != "" {
		parts = append(parts, event.InvolvedObject)
	}
	if event.Namespace != "" {
		parts = append(parts, event.Namespace)
	}
	return strings.Join(parts, " · ")
}

func fleetClusterFromOverview(contextName string, overview kube.ClusterOverview) ui.FleetCluster {
	nodes := overviewMetric(overview.Stats, "Nodes ready")
	pods := overviewMetric(overview.Stats, "Pods healthy")
	workloads := overviewMetric(overview.Stats, "Workloads ready")
	cpu := overviewMetric(overview.Stats, "CPU")
	memory := overviewMetric(overview.Stats, "Memory")
	warnings := overviewMetricInt(overview.Stats, "Warnings")
	card := ui.FleetCluster{
		Context:      contextName,
		Name:         contextName,
		Nodes:        nodes,
		Pods:         pods,
		Workloads:    workloads,
		CPU:          cpu,
		Memory:       memory,
		WarningCount: warnings,
		StatusKey:    "good",
		TopConcern:   "Healthy",
	}
	if clusterName := overviewIdentityValue(overview.Identity, "Cluster"); clusterName != "" {
		card.Name = clusterName
	}
	if overview.Error != "" {
		card.StatusKey = "danger"
		card.Detail = overview.Error
		card.TopConcern = overview.Error
		return card
	}
	if warnings > 0 {
		card.StatusKey = "warn"
		card.TopConcern = warningEventLabel(warnings)
		card.IssueKind = kube.KindEvents
		card.IssueLabel = "Investigate warnings"
		if len(overview.WarningEvents) > 0 {
			card.Detail = overview.WarningEvents[0].Reason
			card.IssueQuery = overview.WarningEvents[0].Reason
			if card.IssueQuery == "" {
				card.IssueQuery = overview.WarningEvents[0].InvolvedObject
			}
		}
		return card
	}
	for _, metric := range []kube.OverviewMetric{nodes, pods, workloads} {
		if metric.StatusKey == "danger" || metric.StatusKey == "warn" {
			card.StatusKey = metric.StatusKey
			card.TopConcern = metric.Label + " " + metric.Value
			card.IssueKind = metric.Kind
			card.IssueLabel = "Inspect " + strings.ToLower(metric.Label)
			if metric.Detail != "" {
				card.Detail = metric.Detail
			}
			return card
		}
	}
	card.Detail = "No recent warnings"
	return card
}

func warningEventLabel(count int) string {
	if count == 1 {
		return "1 recent warning event"
	}
	return fmt.Sprintf("%d recent warning events", count)
}

func overviewMetric(metrics []kube.OverviewMetric, label string) kube.OverviewMetric {
	for _, metric := range metrics {
		if metric.Label == label {
			return metric
		}
	}
	return kube.OverviewMetric{Label: label, Value: "0", StatusKey: "neutral"}
}

func overviewMetricInt(metrics []kube.OverviewMetric, label string) int {
	value, err := strconv.Atoi(overviewMetric(metrics, label).Value)
	if err != nil {
		return 0
	}
	return value
}

func overviewIdentityValue(fields []kube.DetailField, name string) string {
	for _, field := range fields {
		if field.Name == name {
			return field.Value
		}
	}
	return ""
}

func fleetStatusRank(statusKey string) int {
	switch statusKey {
	case "danger":
		return 0
	case "warn":
		return 1
	case "good":
		return 2
	default:
		return 3
	}
}

func isStandalonePageKind(kind kube.ResourceKind) bool {
	return kind == kube.KindOverview || kind == kube.KindActions
}

func (s *Server) table(kind kube.ResourceKind, namespace, query, sortColumn, sortOrder string, contexts []string) kube.Table {
	sessions := s.sessionsForContexts(contexts)
	s.waitForInitialSyncs(sessions)
	def := resourceDef(kind)
	out := kube.Table{
		Kind:       def.Kind,
		Label:      def.Label,
		Namespace:  namespace,
		Query:      query,
		UpdatedAt:  time.Now(),
		Namespaced: def.Scope == "namespaced",
	}
	if len(sessions) == 0 {
		out.Error = "No Kubernetes client is configured."
		return out
	}
	if len(sessions) == 1 && len(s.clusters) <= 1 && sessions[0].store != nil {
		return sessions[0].store.TableWithSort(kind, namespace, query, sortColumn, sortOrder)
	}

	errors := []string{}
	for _, session := range sessions {
		if session.store == nil {
			continue
		}
		table := session.store.TableWithSort(kind, namespace, "", "", "")
		if len(out.Columns) == 0 && len(table.Columns) > 0 {
			out.Columns = append([]string{"Cluster"}, table.Columns...)
		}
		if table.Error != "" {
			errors = append(errors, session.cluster.ContextName+": "+table.Error)
			continue
		}
		for _, row := range table.Rows {
			row.Cluster = session.cluster.ContextName
			row.Cells = append([]kube.Cell{{Value: session.cluster.ContextName, Class: "cluster"}}, row.Cells...)
			out.Rows = append(out.Rows, row)
		}
	}
	if len(out.Columns) == 0 {
		out.Columns = append([]string{"Cluster"}, kube.Table{Kind: kind}.Columns...)
	}
	kube.FilterTableRows(&out, query)
	kube.SortTableRows(&out, sortColumn, sortOrder)
	out.Error = strings.Join(errors, "\n")
	return out
}

const (
	quickSwitcherObjectLimit        = 600
	quickSwitcherObjectPerKindLimit = 60
)

func (s *Server) quickSwitcherObjectItems(signals ui.Signals, contexts []string) []ui.QuickSwitcherItem {
	items := []ui.QuickSwitcherItem{}
	for _, def := range kube.ResourceDefs {
		if def.Kind == kube.KindOverview || def.Kind == kube.KindActions {
			continue
		}
		table := s.table(def.Kind, "", "", "", "", contexts)
		if table.Error != "" {
			continue
		}
		perKind := 0
		for _, row := range table.Rows {
			if row.Name == "" {
				continue
			}
			contextName := signals.Context
			if row.Cluster != "" {
				contextName = row.Cluster
			}
			clusters := signals.Clusters
			if contextName != "" {
				clusters = contextName
			}
			namespace := ""
			if table.Namespaced {
				namespace = row.Namespace
			}
			items = append(items, ui.QuickSwitcherItem{
				Label:             row.Name,
				Meta:              quickSwitcherObjectMeta(table, row),
				MetaTokens:        quickSwitcherObjectMetaTokens(table, row),
				KindLabel:         table.Label,
				Icon:              ui.ResourceIconClass(table.Kind),
				Search:            quickSwitcherObjectSearch(table, row),
				Context:           contextName,
				Clusters:          clusters,
				Resource:          string(table.Kind),
				Namespace:         namespace,
				SelectedName:      row.Name,
				SelectedNamespace: row.Namespace,
				DetailMode:        "overview",
				Endpoint:          "/ui/table",
			})
			perKind++
			if perKind >= quickSwitcherObjectPerKindLimit || len(items) >= quickSwitcherObjectLimit {
				break
			}
		}
		if len(items) >= quickSwitcherObjectLimit {
			break
		}
	}
	return items
}

func quickSwitcherObjectMeta(table kube.Table, row kube.Row) string {
	parts := []string{table.Label}
	if row.Namespace != "" {
		parts = append(parts, row.Namespace)
	}
	if row.Cluster != "" {
		parts = append(parts, row.Cluster)
	}
	if row.Status != "" {
		parts = append(parts, row.Status)
	}
	return strings.Join(parts, " · ")
}

func quickSwitcherObjectMetaTokens(table kube.Table, row kube.Row) []ui.QuickSwitcherMetaToken {
	tokens := []ui.QuickSwitcherMetaToken{{
		Label: table.Label,
		Icon:  ui.ResourceIconClass(table.Kind),
	}}
	if row.Namespace != "" {
		tokens = append(tokens, ui.QuickSwitcherMetaToken{Label: row.Namespace, Icon: ui.ResourceIconClass(kube.KindNamespaces)})
	}
	if row.Cluster != "" {
		tokens = append(tokens, ui.QuickSwitcherMetaToken{Label: row.Cluster, Icon: "icon-[lucide--server]"})
	}
	if row.Status != "" {
		tokens = append(tokens, ui.QuickSwitcherMetaToken{Label: row.Status, Icon: quickSwitcherObjectStatusIcon(row.Status)})
	}
	return tokens
}

func quickSwitcherObjectStatusIcon(status string) string {
	switch strings.ToLower(status) {
	case "running", "active", "bound", "ready", "true", "succeeded", "complete":
		return "icon-[lucide--circle-check]"
	case "pending", "progressing", "terminating":
		return "icon-[lucide--loader]"
	case "failed", "error", "crashloopbackoff":
		return "icon-[lucide--circle-alert]"
	default:
		return "icon-[lucide--activity]"
	}
}

func quickSwitcherObjectSearch(table kube.Table, row kube.Row) string {
	return strings.ToLower(strings.Join([]string{
		row.Name,
		row.Namespace,
		row.Cluster,
		row.Status,
		table.Label,
		string(table.Kind),
	}, " "))
}

func (s *Server) namespaces(contexts []string) ([]string, error) {
	sessions := s.sessionsForContexts(contexts)
	s.waitForInitialSyncs(sessions)
	seen := map[string]bool{}
	namespaces := []string{}
	messages := []string{}
	for _, session := range sessions {
		if session.store == nil {
			continue
		}
		names, err := session.store.Namespaces()
		if err != nil {
			messages = append(messages, session.cluster.ContextName+": "+err.Error())
			continue
		}
		for _, namespace := range names {
			if seen[namespace] {
				continue
			}
			seen[namespace] = true
			namespaces = append(namespaces, namespace)
		}
	}
	sort.Strings(namespaces)
	if len(messages) > 0 {
		return namespaces, errors.New(strings.Join(messages, "\n"))
	}
	return namespaces, nil
}

func (s *Server) clusterList() []*kube.Cluster {
	clusters := make([]*kube.Cluster, 0, len(s.clusters))
	for _, session := range s.clusters {
		clusters = append(clusters, session.cluster)
	}
	return clusters
}

func sessionCluster(session *clusterSession) *kube.Cluster {
	if session == nil {
		return nil
	}
	return session.cluster
}

func unavailableChartMetrics(message string) kube.MetricsState {
	return kube.MetricsState{
		Message:   message,
		Window:    "Last 60 minutes",
		UpdatedAt: time.Now(),
	}
}

func chartLimit(value string) int {
	if value == "" {
		return 8
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 {
		return 8
	}
	if limit > 24 {
		return 24
	}
	return limit
}

func chartWindow(seconds int) time.Duration {
	if seconds <= 0 {
		return 60 * time.Minute
	}
	if seconds < 60 {
		seconds = 60
	}
	if seconds > 6*60*60 {
		seconds = 6 * 60 * 60
	}
	return time.Duration(seconds) * time.Second
}

func chartStep(seconds int) time.Duration {
	if seconds <= 0 {
		return 60 * time.Second
	}
	if seconds < 15 {
		seconds = 15
	}
	if seconds > 5*60 {
		seconds = 5 * 60
	}
	return time.Duration(seconds) * time.Second
}

func chartPanel(panel, legacyView string) string {
	switch panel {
	case "pod-usage-overview", "pod-usage-detail":
		return panel
	}
	if legacyView == "detail" {
		return "pod-usage-detail"
	}
	return "pod-usage-overview"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func writeCompressedJSON(w http.ResponseWriter, r *http.Request, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		http.Error(w, fmt.Sprintf("encode JSON response: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Vary", "Accept-Encoding")
	if strings.Contains(strings.ToLower(r.Header.Get("Accept-Encoding")), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(status)
		gz := gzip.NewWriter(w)
		_, _ = gz.Write(data)
		_ = gz.Close()
		return
	}
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func resourceDef(kind kube.ResourceKind) kube.ResourceDef {
	for _, def := range kube.ResourceDefs {
		if def.Kind == kind {
			return def
		}
	}
	return kube.ResourceDefs[0]
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-eval' https://cdn.jsdelivr.net; style-src 'self'; font-src 'self'; connect-src 'self' https://cdn.jsdelivr.net; img-src 'self' data:")
		next.ServeHTTP(w, r)
	})
}
