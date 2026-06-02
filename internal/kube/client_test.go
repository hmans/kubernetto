package kube

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewClustersReadsEveryKubeconfigContext(t *testing.T) {
	kubeconfig := filepath.Join(t.TempDir(), "config")
	content := `
apiVersion: v1
kind: Config
clusters:
- name: dev-cluster
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
- name: prod-cluster
  cluster:
    server: https://127.0.0.1:7443
    insecure-skip-tls-verify: true
users:
- name: dev-user
  user:
    token: dev-token
- name: prod-user
  user:
    token: prod-token
contexts:
- name: prod
  context:
    cluster: prod-cluster
    user: prod-user
    namespace: platform
- name: dev
  context:
    cluster: dev-cluster
    user: dev-user
current-context: dev
`
	if err := os.WriteFile(kubeconfig, []byte(content), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}

	clusters, err := NewClusters(kubeconfig)
	if err != nil {
		t.Fatalf("NewClusters: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("clusters = %d, want 2", len(clusters))
	}
	if clusters[0].ContextName != "dev" || !clusters[0].Current {
		t.Fatalf("first cluster = %#v, want current dev context", clusters[0])
	}
	if clusters[0].Namespace != "default" {
		t.Fatalf("dev namespace = %q, want default", clusters[0].Namespace)
	}
	if clusters[1].ContextName != "prod" || clusters[1].ClusterName != "prod-cluster" {
		t.Fatalf("second cluster = %#v, want prod context metadata", clusters[1])
	}
	if clusters[1].Namespace != "platform" {
		t.Fatalf("prod namespace = %q, want platform", clusters[1].Namespace)
	}
}
