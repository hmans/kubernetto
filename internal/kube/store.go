package kube

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/informers"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	networkinglisters "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

const cacheWarmingMessage = "Kubernetes cache is warming up."

type ResourceStore struct {
	cluster *Cluster
	logger  *slog.Logger

	factory informers.SharedInformerFactory
	synced  []cache.InformerSynced
	ready   atomic.Bool
	started atomic.Bool

	mu            sync.RWMutex
	serverVersion string
	versionError  string
	errorMessage  string
	podUsage      map[string]corev1.ResourceList

	pods         corelisters.PodLister
	events       corelisters.EventLister
	services     corelisters.ServiceLister
	nodes        corelisters.NodeLister
	namespaces   corelisters.NamespaceLister
	deployments  appslisters.DeploymentLister
	statefulSets appslisters.StatefulSetLister
	daemonSets   appslisters.DaemonSetLister
	ingresses    networkinglisters.IngressLister
}

func NewResourceStore(cluster *Cluster, logger *slog.Logger) *ResourceStore {
	if logger == nil {
		logger = slog.Default()
	}

	store := &ResourceStore{
		cluster: cluster,
		logger:  logger,
	}
	if cluster == nil || cluster.Clientset == nil {
		store.setError("No Kubernetes client is configured.")
		return store
	}

	store.factory = informers.NewSharedInformerFactory(cluster.Clientset, 0)
	store.podUsage = map[string]corev1.ResourceList{}

	pods := store.factory.Core().V1().Pods()
	events := store.factory.Core().V1().Events()
	services := store.factory.Core().V1().Services()
	nodes := store.factory.Core().V1().Nodes()
	namespaces := store.factory.Core().V1().Namespaces()
	deployments := store.factory.Apps().V1().Deployments()
	statefulSets := store.factory.Apps().V1().StatefulSets()
	daemonSets := store.factory.Apps().V1().DaemonSets()
	ingresses := store.factory.Networking().V1().Ingresses()

	store.pods = pods.Lister()
	store.events = events.Lister()
	store.services = services.Lister()
	store.nodes = nodes.Lister()
	store.namespaces = namespaces.Lister()
	store.deployments = deployments.Lister()
	store.statefulSets = statefulSets.Lister()
	store.daemonSets = daemonSets.Lister()
	store.ingresses = ingresses.Lister()

	store.synced = []cache.InformerSynced{
		pods.Informer().HasSynced,
		services.Informer().HasSynced,
		nodes.Informer().HasSynced,
		namespaces.Informer().HasSynced,
		deployments.Informer().HasSynced,
		statefulSets.Informer().HasSynced,
		daemonSets.Informer().HasSynced,
		ingresses.Informer().HasSynced,
	}

	return store
}

func (s *ResourceStore) Start(ctx context.Context) {
	if s == nil || s.factory == nil {
		return
	}
	if !s.started.CompareAndSwap(false, true) {
		return
	}

	if s.cluster.Discovery != nil {
		go s.loadServerVersion(ctx)
	}
	if s.cluster.MetricsClient != nil {
		go s.pollPodMetrics(ctx, s.cluster.MetricsClient)
	}

	s.factory.Start(ctx.Done())
	go func() {
		if cache.WaitForCacheSync(ctx.Done(), s.synced...) {
			s.ready.Store(true)
			s.setError("")
			s.logger.Info("kubernetes resource cache synced")
			return
		}
		if ctx.Err() == nil {
			s.setError("Kubernetes cache failed to sync.")
			s.logger.Warn("kubernetes resource cache failed to sync")
		}
	}()
}

func (s *ResourceStore) WaitForSync(ctx context.Context) bool {
	if s == nil {
		return false
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		if s.ready.Load() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

func (s *ResourceStore) Summary() Summary {
	out := Summary{UpdatedAt: time.Now()}
	if s == nil || s.cluster == nil {
		out.Error = "No Kubernetes client is configured."
		return out
	}
	out.Context = s.cluster.ContextName
	out.Namespace = s.cluster.Namespace
	out.ServerVersion = s.serverVersionValue()
	if versionErr := s.serverVersionError(); versionErr != "" {
		out.Error = versionErr
	}

	if err := s.currentError(); err != "" {
		out.Error = err
		return out
	}
	if !s.ready.Load() {
		out.Error = cacheWarmingMessage
		return out
	}

	out.Nodes = len(s.listNodes())
	out.Namespaces = len(s.listNamespaces())
	out.Pods = len(s.listPods(""))
	out.Deployments = len(s.listDeployments(""))
	return out
}

func (s *ResourceStore) Namespaces() ([]string, error) {
	if err := s.readinessError(); err != nil {
		return nil, err
	}
	namespaces := s.listNamespaces()
	names := make([]string, 0, len(namespaces))
	for _, namespace := range namespaces {
		names = append(names, namespace.Name)
	}
	sort.Strings(names)
	return names, nil
}

func (s *ResourceStore) Table(kind ResourceKind, namespace, query string) Table {
	return s.TableWithSort(kind, namespace, query, "", "")
}

func (s *ResourceStore) TableWithSort(kind ResourceKind, namespace, query, sortColumn, sortOrder string) Table {
	def := resourceDef(kind)
	table := Table{
		Kind:       def.Kind,
		Label:      def.Label,
		Namespace:  namespace,
		Query:      query,
		UpdatedAt:  time.Now(),
		Namespaced: def.Scope == "namespaced",
	}

	if err := s.readinessError(); err != nil {
		table.Error = err.Error()
		return table
	}

	switch def.Kind {
	case KindPods:
		table.Columns = []string{"Name", "Namespace", "Ready", "Status", "Restarts", "CPU", "CPU Req", "CPU Limit", "MEM", "MEM Req", "MEM Limit", "Node", "Age"}
		for _, pod := range s.listPods(namespace) {
			row := podRow(*pod, s.podUsageFor(pod.Namespace, pod.Name))
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindDeployments:
		table.Columns = []string{"Name", "Namespace", "Ready", "Up-to-date", "Available", "CPU", "CPU Req", "CPU Limit", "MEM", "MEM Req", "MEM Limit", "Age"}
		for _, deployment := range s.listDeployments(namespace) {
			row := deploymentRow(*deployment, s.podUsageForSelector(deployment.Namespace, deployment.Spec.Selector))
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindStatefulSet:
		table.Columns = []string{"Name", "Namespace", "Ready", "Replicas", "CPU", "CPU Req", "CPU Limit", "MEM", "MEM Req", "MEM Limit", "Age"}
		for _, statefulSet := range s.listStatefulSets(namespace) {
			row := statefulSetRow(*statefulSet, s.podUsageForSelector(statefulSet.Namespace, statefulSet.Spec.Selector))
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindDaemonSet:
		table.Columns = []string{"Name", "Namespace", "Desired", "Ready", "Available", "CPU", "CPU Req", "CPU Limit", "MEM", "MEM Req", "MEM Limit", "Age"}
		for _, daemonSet := range s.listDaemonSets(namespace) {
			row := daemonSetRow(*daemonSet, s.podUsageForSelector(daemonSet.Namespace, daemonSet.Spec.Selector))
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindServices:
		table.Columns = []string{"Name", "Namespace", "Type", "Cluster IP", "Ports", "Age"}
		for _, service := range s.listServices(namespace) {
			row := serviceRow(*service)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindIngresses:
		table.Columns = []string{"Name", "Namespace", "Class", "Hosts", "Address", "Age"}
		for _, ingress := range s.listIngresses(namespace) {
			row := ingressRow(*ingress)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindNodes:
		table.Columns = []string{"Name", "Status", "Roles", "Version", "Internal IP", "Age"}
		for _, node := range s.listNodes() {
			row := nodeRow(*node)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	case KindNamespaces:
		table.Columns = []string{"Name", "Status", "Age"}
		for _, namespace := range s.listNamespaces() {
			row := namespaceRow(*namespace)
			if matches(row, query) {
				table.Rows = append(table.Rows, row)
			}
		}
	}

	sortTableRows(&table, sortColumn, sortOrder)
	return table
}

func (s *ResourceStore) loadServerVersion(ctx context.Context) {
	versionCh := make(chan *version.Info, 1)
	errCh := make(chan error, 1)
	go func() {
		versionInfo, err := s.cluster.Discovery.ServerVersion()
		if err != nil {
			errCh <- err
			return
		}
		versionCh <- versionInfo
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		s.setVersionError(err.Error())
	case versionInfo := <-versionCh:
		s.mu.Lock()
		s.serverVersion = versionInfo.GitVersion
		s.mu.Unlock()
	}
}

func (s *ResourceStore) pollPodMetrics(ctx context.Context, metricsClient metricsclient.Interface) {
	metricsAPIUnavailableLogged := false
	if err := s.loadPodMetrics(ctx, metricsClient); err != nil && ctx.Err() == nil {
		s.logPodMetricsError(err, &metricsAPIUnavailableLogged)
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.loadPodMetrics(ctx, metricsClient); err != nil && ctx.Err() == nil {
				s.logPodMetricsError(err, &metricsAPIUnavailableLogged)
			}
		}
	}
}

func (s *ResourceStore) loadPodMetrics(ctx context.Context, metricsClient metricsclient.Interface) error {
	if s == nil || metricsClient == nil {
		return nil
	}
	list, err := metricsClient.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	podUsage := make(map[string]corev1.ResourceList, len(list.Items))
	for _, metrics := range list.Items {
		usage := corev1.ResourceList{}
		for _, container := range metrics.Containers {
			addResourceList(usage, container.Usage)
		}
		if len(usage) > 0 {
			podUsage[podKey(metrics.Namespace, metrics.Name)] = usage
		}
	}
	s.setPodUsage(podUsage)
	return nil
}

func (s *ResourceStore) logPodMetricsError(err error, metricsAPIUnavailableLogged *bool) {
	if podMetricsAPIUnavailable(err) {
		if metricsAPIUnavailableLogged == nil || !*metricsAPIUnavailableLogged {
			s.logger.Info("pod metrics API unavailable; usage columns will remain empty", "error", err)
		}
		if metricsAPIUnavailableLogged != nil {
			*metricsAPIUnavailableLogged = true
		}
		return
	}
	s.logger.Warn("pod metrics unavailable", "error", err)
}

func podMetricsAPIUnavailable(err error) bool {
	return apierrors.IsNotFound(err)
}

func (s *ResourceStore) readinessError() error {
	if s == nil || s.cluster == nil || s.cluster.Clientset == nil {
		return fmt.Errorf("No Kubernetes client is configured.")
	}
	if err := s.currentError(); err != "" {
		return fmt.Errorf("%s", err)
	}
	if !s.ready.Load() {
		return fmt.Errorf("%s", cacheWarmingMessage)
	}
	return nil
}

func (s *ResourceStore) currentError() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.errorMessage
}

func (s *ResourceStore) setError(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errorMessage = message
}

func (s *ResourceStore) setVersionError(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.versionError = message
}

func (s *ResourceStore) setPodUsage(podUsage map[string]corev1.ResourceList) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.podUsage = podUsage
}

func (s *ResourceStore) serverVersionValue() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.serverVersion
}

func (s *ResourceStore) serverVersionError() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.versionError
}

func (s *ResourceStore) podUsageFor(namespace, name string) corev1.ResourceList {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return copyResourceList(s.podUsage[podKey(namespace, name)])
}

func (s *ResourceStore) podUsageForSelector(namespace string, selector *metav1.LabelSelector) corev1.ResourceList {
	if selector == nil {
		return nil
	}
	labelSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil || labels.MatchesNothing(labelSelector) {
		return nil
	}

	total := corev1.ResourceList{}
	for _, pod := range s.listPods(namespace) {
		if labelSelector.Matches(labels.Set(pod.Labels)) {
			addResourceList(total, s.podUsageFor(pod.Namespace, pod.Name))
		}
	}
	if len(total) == 0 {
		return nil
	}
	return total
}

func podKey(namespace, name string) string {
	return namespace + "/" + name
}

func copyResourceList(values corev1.ResourceList) corev1.ResourceList {
	if len(values) == 0 {
		return nil
	}
	out := make(corev1.ResourceList, len(values))
	addResourceList(out, values)
	return out
}

func addResourceList(total, values corev1.ResourceList) {
	for name, quantity := range values {
		if quantity.Sign() == 0 {
			continue
		}
		current := total[name]
		current.Add(quantity)
		total[name] = current
	}
}

func (s *ResourceStore) listPods(namespace string) []*corev1.Pod {
	if namespace != "" {
		items, _ := s.pods.Pods(namespace).List(labels.Everything())
		return items
	}
	items, _ := s.pods.List(labels.Everything())
	return items
}

func (s *ResourceStore) listServices(namespace string) []*corev1.Service {
	if namespace != "" {
		items, _ := s.services.Services(namespace).List(labels.Everything())
		return items
	}
	items, _ := s.services.List(labels.Everything())
	return items
}

func (s *ResourceStore) listNodes() []*corev1.Node {
	items, _ := s.nodes.List(labels.Everything())
	return items
}

func (s *ResourceStore) listNamespaces() []*corev1.Namespace {
	items, _ := s.namespaces.List(labels.Everything())
	return items
}

func (s *ResourceStore) listDeployments(namespace string) []*appsv1.Deployment {
	if namespace != "" {
		items, _ := s.deployments.Deployments(namespace).List(labels.Everything())
		return items
	}
	items, _ := s.deployments.List(labels.Everything())
	return items
}

func (s *ResourceStore) listStatefulSets(namespace string) []*appsv1.StatefulSet {
	if namespace != "" {
		items, _ := s.statefulSets.StatefulSets(namespace).List(labels.Everything())
		return items
	}
	items, _ := s.statefulSets.List(labels.Everything())
	return items
}

func (s *ResourceStore) listDaemonSets(namespace string) []*appsv1.DaemonSet {
	if namespace != "" {
		items, _ := s.daemonSets.DaemonSets(namespace).List(labels.Everything())
		return items
	}
	items, _ := s.daemonSets.List(labels.Everything())
	return items
}

func (s *ResourceStore) listIngresses(namespace string) []*networkingv1.Ingress {
	if namespace != "" {
		items, _ := s.ingresses.Ingresses(namespace).List(labels.Everything())
		return items
	}
	items, _ := s.ingresses.List(labels.Everything())
	return items
}
