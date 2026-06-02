package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
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
	app.ensureStore(app.defaultContext)
	return app
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /assets/app.css", ui.HandleStyles)
	mux.HandleFunc("GET /assets/app.js", ui.HandleScript)
	mux.HandleFunc("GET /ui/refresh", s.handleRefresh)
	mux.HandleFunc("GET /ui/summary", s.handleSummary)
	mux.HandleFunc("GET /ui/table", s.handleTable)
	mux.HandleFunc("GET /ui/selection", s.handleSelection)
	mux.HandleFunc("GET /ui/detail", s.handleDetail)
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
	sse.PatchElements(ui.RenderFragment(ui.ResourceNavView(state)))
	sse.PatchElements(ui.RenderFragment(ui.SummaryView(state)))
	sse.PatchElements(ui.RenderFragment(ui.NamespacePickerView(state)))
	sse.PatchElements(ui.RenderFragment(ui.ContentView(state)))
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	signals := readSignals(r)
	state := s.state(signals)
	sse := datastar.NewSSE(w, r)
	sse.PatchElements(ui.RenderFragment(ui.SummaryView(state)))
}

func (s *Server) handleTable(w http.ResponseWriter, r *http.Request) {
	signals := readSignals(r)
	state := s.state(signals)
	sse := datastar.NewSSE(w, r)
	sse.PatchElements(ui.RenderFragment(ui.ResourceNavView(state)))
	sse.PatchElements(ui.RenderFragment(ui.NamespacePickerView(state)))
	sse.PatchElements(ui.RenderFragment(ui.ContentView(state)))
}

func (s *Server) handleSelection(w http.ResponseWriter, r *http.Request) {
	signals := readSignals(r)
	state := s.state(signals)
	sse := datastar.NewSSE(w, r)
	sse.PatchElements(ui.RenderFragment(ui.ContentView(state)))
}

func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	signals := readSignals(r)
	state := s.state(signals)
	sse := datastar.NewSSE(w, r)
	sse.PatchElements(ui.RenderFragment(ui.DetailView(state)))
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
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

	summary := kube.Summary{UpdatedAt: time.Now(), Error: "No Kubernetes client is configured."}
	table := kube.Table{Kind: kind, Label: def.Label, Namespace: namespace, Query: signals.Query, SortColumn: signals.SortColumn, SortOrder: signals.SortOrder, UpdatedAt: time.Now(), Namespaced: def.Scope == "namespaced"}
	detail := kube.ResourceDetail{Kind: kind, Label: def.Label, Name: signals.SelectedName, Namespace: selectedNamespace}
	namespaces := []string{}
	namespaceErr := ""

	if session != nil && session.store != nil {
		summary = session.store.Summary()
		table = session.store.TableWithSort(kind, namespace, signals.Query, signals.SortColumn, signals.SortOrder)
		detail = session.store.Detail(kind, selectedNamespace, signals.SelectedName)
		var err error
		namespaces, err = session.store.Namespaces()
		if err != nil {
			namespaceErr = err.Error()
		}
	}

	return ui.PageState{
		Cluster:      sessionCluster(session),
		Clusters:     s.clusterList(),
		Resources:    kube.ResourceDefs,
		Signals:      ui.Signals{Context: contextName, Resource: string(kind), Namespace: namespace, Query: signals.Query, SortColumn: table.SortColumn, SortOrder: table.SortOrder, SelectedName: signals.SelectedName, SelectedNamespace: selectedNamespace, DetailMode: detailMode},
		Summary:      summary,
		Table:        table,
		Detail:       detail,
		Namespaces:   namespaces,
		NamespaceErr: namespaceErr,
	}
}

func readSignals(r *http.Request) ui.Signals {
	signals := ui.Signals{Resource: string(kube.KindPods)}
	if err := datastar.ReadSignals(r, &signals); err != nil && !errors.Is(err, http.ErrNoCookie) {
		// Datastar omits signals on plain browser requests. Query parameters keep
		// endpoints easy to hit directly while the UI sends reactive signals.
	}
	q := r.URL.Query()
	if contextName := q.Get("context"); contextName != "" {
		signals.Context = contextName
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
		signals.Resource = string(kube.KindPods)
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
	if session.store == nil {
		session.store = kube.NewResourceStore(session.cluster, s.logger)
		session.store.Start(s.ctx)
	}
	return session
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-eval' https://cdn.jsdelivr.net; style-src 'self'; connect-src 'self' https://cdn.jsdelivr.net; img-src 'self' data:")
		next.ServeHTTP(w, r)
	})
}
