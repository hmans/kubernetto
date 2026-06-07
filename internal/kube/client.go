package kube

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

type Cluster struct {
	Clientset      kubernetes.Interface
	DynamicClient  dynamic.Interface
	Discovery      discovery.DiscoveryInterface
	MetricsClient  metricsclient.Interface
	ContextName    string
	ClusterName    string
	UserName       string
	Namespace      string
	ConfigSource   string
	KubeconfigPath string
	Current        bool
}

func NewCluster(kubeconfig string) (*Cluster, error) {
	clusters, err := NewClusters(kubeconfig)
	if err != nil {
		return nil, err
	}
	return clusters[0], nil
}

func NewClusters(kubeconfig string) ([]*Cluster, error) {
	rules, source := kubeconfigLoadingRules(kubeconfig)
	if clusters, err := fromKubeconfig(rules, source); err == nil {
		return clusters, nil
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		if source == "" {
			return nil, fmt.Errorf("no kubeconfig path found and in-cluster config unavailable: %w", err)
		}
		return nil, fmt.Errorf("kubeconfig %q unavailable and in-cluster config unavailable: %w", source, err)
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	metricsClient, err := metricsclient.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return []*Cluster{{
		Clientset:     clientset,
		DynamicClient: dynamicClient,
		Discovery:     clientset.Discovery(),
		MetricsClient: metricsClient,
		ContextName:   "in-cluster",
		ConfigSource:  "in-cluster service account",
		Current:       true,
	}}, nil
}

func fromKubeconfig(rules *clientcmd.ClientConfigLoadingRules, source string) ([]*Cluster, error) {
	raw, err := rules.Load()
	if err != nil {
		return nil, err
	}
	if len(raw.Contexts) == 0 {
		return nil, errors.New("kubeconfig has no contexts")
	}

	contextNames := orderedContextNames(raw)
	clusters := make([]*Cluster, 0, len(contextNames))
	for _, contextName := range contextNames {
		overrides := &clientcmd.ConfigOverrides{}
		loader := clientcmd.NewNonInteractiveClientConfig(*raw, contextName, overrides, rules)
		cfg, err := loader.ClientConfig()
		if err != nil {
			return nil, err
		}

		clientset, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			return nil, err
		}
		dynamicClient, err := dynamic.NewForConfig(cfg)
		if err != nil {
			return nil, err
		}
		metricsClient, err := metricsclient.NewForConfig(cfg)
		if err != nil {
			return nil, err
		}

		contextInfo := raw.Contexts[contextName]
		namespace := "default"
		clusterName := ""
		userName := ""
		if contextInfo != nil {
			namespace = contextInfo.Namespace
			if namespace == "" {
				namespace = "default"
			}
			clusterName = contextInfo.Cluster
			userName = contextInfo.AuthInfo
		}

		clusters = append(clusters, &Cluster{
			Clientset:      clientset,
			DynamicClient:  dynamicClient,
			Discovery:      clientset.Discovery(),
			MetricsClient:  metricsClient,
			ContextName:    contextName,
			ClusterName:    clusterName,
			UserName:       userName,
			Namespace:      namespace,
			ConfigSource:   "kubeconfig",
			KubeconfigPath: source,
			Current:        contextName == raw.CurrentContext,
		})
	}

	return clusters, nil
}

func CurrentContext(kubeconfig string) (*api.Context, string, error) {
	rules, _ := kubeconfigLoadingRules(kubeconfig)
	raw, err := rules.Load()
	if err != nil {
		return nil, "", err
	}
	ctx := raw.Contexts[raw.CurrentContext]
	if ctx == nil {
		return nil, raw.CurrentContext, errors.New("current context not found")
	}
	return ctx, raw.CurrentContext, nil
}

func kubeconfigLoadingRules(kubeconfig string) (*clientcmd.ClientConfigLoadingRules, string) {
	if kubeconfig != "" {
		return &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}, kubeconfig
	}
	if env := os.Getenv(clientcmd.RecommendedConfigPathEnvVar); env != "" {
		return clientcmd.NewDefaultClientConfigLoadingRules(), env
	}
	homeFile := ""
	if home, err := os.UserHomeDir(); err == nil {
		homeFile = filepath.Join(home, ".kube", "config")
	}
	return clientcmd.NewDefaultClientConfigLoadingRules(), homeFile
}

func orderedContextNames(raw *api.Config) []string {
	contextNames := make([]string, 0, len(raw.Contexts))
	for name := range raw.Contexts {
		if name != raw.CurrentContext {
			contextNames = append(contextNames, name)
		}
	}
	sort.Strings(contextNames)
	if raw.CurrentContext != "" {
		contextNames = append([]string{raw.CurrentContext}, contextNames...)
	}
	return contextNames
}
