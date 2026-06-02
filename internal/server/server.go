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

type Signals struct {
	Context   string `json:"context"`
	Resource  string `json:"resource"`
	Namespace string `json:"namespace"`
	Query     string `json:"query"`
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
	mux.HandleFunc("GET /assets/app.css", s.handleStyles)
	mux.HandleFunc("GET /ui/refresh", s.handleRefresh)
	mux.HandleFunc("GET /ui/summary", s.handleSummary)
	mux.HandleFunc("GET /ui/table", s.handleTable)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	return withSecurityHeaders(mux)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	state := s.state(Signals{Resource: string(kube.KindPods)})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := renderPage(w, state); err != nil {
		s.logger.Error("render index", "error", err)
	}
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	signals := readSignals(r)
	state := s.state(signals)
	sse := datastar.NewSSE(w, r)
	sse.PatchElements(renderFragment(resourceNav(state)))
	sse.PatchElements(renderFragment(summaryView(state)))
	sse.PatchElements(renderFragment(namespacePicker(state)))
	sse.PatchElements(renderFragment(tableView(state)))
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	signals := readSignals(r)
	state := s.state(signals)
	sse := datastar.NewSSE(w, r)
	sse.PatchElements(renderFragment(summaryView(state)))
}

func (s *Server) handleTable(w http.ResponseWriter, r *http.Request) {
	signals := readSignals(r)
	state := s.state(signals)
	sse := datastar.NewSSE(w, r)
	sse.PatchElements(renderFragment(resourceNav(state)))
	sse.PatchElements(renderFragment(namespacePicker(state)))
	sse.PatchElements(renderFragment(tableView(state)))
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) state(signals Signals) PageState {
	kind := kube.NormalizeKind(signals.Resource)
	namespace := signals.Namespace
	session := s.session(signals.Context)
	contextName := ""
	if session != nil && session.cluster != nil {
		contextName = session.cluster.ContextName
	}

	def := resourceDef(kind)
	if def.Scope == "cluster" {
		namespace = ""
	}

	summary := kube.Summary{UpdatedAt: time.Now(), Error: "No Kubernetes client is configured."}
	table := kube.Table{Kind: kind, Label: def.Label, Namespace: namespace, Query: signals.Query, UpdatedAt: time.Now(), Namespaced: def.Scope == "namespaced"}
	namespaces := []string{}
	namespaceErr := ""

	if session != nil && session.store != nil {
		summary = session.store.Summary()
		table = session.store.Table(kind, namespace, signals.Query)
		var err error
		namespaces, err = session.store.Namespaces()
		if err != nil {
			namespaceErr = err.Error()
		}
	}

	return PageState{
		Cluster:      sessionCluster(session),
		Clusters:     s.clusterList(),
		Resources:    kube.ResourceDefs,
		Signals:      Signals{Context: contextName, Resource: string(kind), Namespace: namespace, Query: signals.Query},
		Summary:      summary,
		Table:        table,
		Namespaces:   namespaces,
		NamespaceErr: namespaceErr,
	}
}

func readSignals(r *http.Request) Signals {
	signals := Signals{Resource: string(kube.KindPods)}
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
	if signals.Resource == "" {
		signals.Resource = string(kube.KindPods)
	}
	return signals
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
