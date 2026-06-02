package server

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/starfederation/datastar-go/datastar"

	"kubernetto/internal/kube"
)

type Server struct {
	cluster *kube.Cluster
	store   *kube.ResourceStore
	logger  *slog.Logger
}

type Signals struct {
	Resource   string `json:"resource"`
	Namespace  string `json:"namespace"`
	Query      string `json:"query"`
	SortColumn string `json:"sortColumn"`
	SortOrder  string `json:"sortOrder"`
}

func New(cluster *kube.Cluster, store *kube.ResourceStore, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{cluster: cluster, store: store, logger: logger}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /assets/app.css", s.handleStyles)
	mux.HandleFunc("GET /ui/refresh", s.handleRefresh)
	mux.HandleFunc("GET /ui/table", s.handleTable)
	mux.HandleFunc("GET /events", s.handleEvents)
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

func (s *Server) handleTable(w http.ResponseWriter, r *http.Request) {
	signals := readSignals(r)
	state := s.state(signals)
	sse := datastar.NewSSE(w, r)
	sse.PatchElements(renderFragment(resourceNav(state)))
	sse.PatchElements(renderFragment(namespacePicker(state)))
	sse.PatchElements(renderFragment(tableView(state)))
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	sse := datastar.NewSSE(w, r)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			state := s.state(readSignals(r))
			sse.PatchElements(renderFragment(summaryView(state)))
		}
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) state(signals Signals) PageState {
	kind := kube.NormalizeKind(signals.Resource)
	namespace := signals.Namespace

	def := resourceDef(kind)
	if def.Scope == "cluster" {
		namespace = ""
	}

	summary := kube.Summary{UpdatedAt: time.Now(), Error: "No Kubernetes client is configured."}
	table := kube.Table{Kind: kind, Label: def.Label, Namespace: namespace, Query: signals.Query, SortColumn: signals.SortColumn, SortOrder: signals.SortOrder, UpdatedAt: time.Now(), Namespaced: def.Scope == "namespaced"}
	namespaces := []string{}
	namespaceErr := ""

	if s.store != nil {
		summary = s.store.Summary()
		table = s.store.TableWithSort(kind, namespace, signals.Query, signals.SortColumn, signals.SortOrder)
		var err error
		namespaces, err = s.store.Namespaces()
		if err != nil {
			namespaceErr = err.Error()
		}
	}

	return PageState{
		Cluster:      s.cluster,
		Resources:    kube.ResourceDefs,
		Signals:      Signals{Resource: string(kind), Namespace: namespace, Query: signals.Query, SortColumn: table.SortColumn, SortOrder: table.SortOrder},
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
	if signals.Resource == "" {
		signals.Resource = string(kube.KindPods)
	}
	return signals
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
