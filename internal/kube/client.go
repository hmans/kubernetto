package kube

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

type Cluster struct {
	Clientset      kubernetes.Interface
	Discovery      discovery.DiscoveryInterface
	MetricsClient  metricsclient.Interface
	ContextName    string
	ClusterName    string
	UserName       string
	Namespace      string
	ConfigSource   string
	KubeconfigPath string
}

func NewCluster(kubeconfig string) (*Cluster, error) {
	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBECONFIG")
	}
	if kubeconfig == "" {
		if home, err := os.UserHomeDir(); err == nil {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}

	if kubeconfig != "" {
		if cluster, err := fromKubeconfig(kubeconfig); err == nil {
			return cluster, nil
		}
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		if kubeconfig == "" {
			return nil, fmt.Errorf("no kubeconfig path found and in-cluster config unavailable: %w", err)
		}
		return nil, fmt.Errorf("kubeconfig %q unavailable and in-cluster config unavailable: %w", kubeconfig, err)
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	metricsClient, err := metricsclient.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Cluster{
		Clientset:     clientset,
		Discovery:     clientset.Discovery(),
		MetricsClient: metricsClient,
		ContextName:   "in-cluster",
		ConfigSource:  "in-cluster service account",
	}, nil
}

func fromKubeconfig(kubeconfig string) (*Cluster, error) {
	rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
	raw, err := rules.Load()
	if err != nil {
		return nil, err
	}
	if raw.CurrentContext == "" {
		return nil, errors.New("kubeconfig has no current context")
	}

	overrides := &clientcmd.ConfigOverrides{}
	loader := clientcmd.NewNonInteractiveClientConfig(*raw, raw.CurrentContext, overrides, rules)
	cfg, err := loader.ClientConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	metricsClient, err := metricsclient.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	contextInfo := raw.Contexts[raw.CurrentContext]
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

	return &Cluster{
		Clientset:      clientset,
		Discovery:      clientset.Discovery(),
		MetricsClient:  metricsClient,
		ContextName:    raw.CurrentContext,
		ClusterName:    clusterName,
		UserName:       userName,
		Namespace:      namespace,
		ConfigSource:   "kubeconfig",
		KubeconfigPath: kubeconfig,
	}, nil
}

func CurrentContext(kubeconfig string) (*api.Context, string, error) {
	rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
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
