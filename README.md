# Kubernetto

A small Kubernetes dashboard built as a single Go binary with a server-rendered Data-Star UI.

## Run

```sh
mise dev
```

Then open <http://127.0.0.1:9832>.

The dev task builds the ignored frontend bundle before starting the Go server, so a fresh checkout or worktree can serve the embedded assets. Kubernetto loads every context from your kubeconfig, selecting the active context by default and falling back to in-cluster config when available. You can override the kubeconfig path with `KUBECONFIG`.

```sh
KUBECONFIG=~/.kube/config mise dev
```

You can also set only the local listen port:

```sh
KUBERNETTO_PORT=9833 mise dev
```

By default, Kubernetto refuses non-loopback listen addresses because the
dashboard can read Kubernetes cluster data using your kubeconfig credentials.
If you intentionally need to expose it to your network, opt in explicitly:

```sh
go run ./cmd/kubernetto --addr 0.0.0.0:9832 --allow-remote
```

## Build

```sh
mise run build
```

The build runs Vite first, producing the ignored frontend bundle that is embedded into the Go binary.

## Templates

The UI is built with templ components. After editing `*.templ` files, regenerate the checked-in Go output:

```sh
go generate ./...
```

## Current Scope

- Cluster summary with live Data-Star SSE refresh.
- Context switching across all contexts in the loaded kubeconfig.
- Grouped resource tables for common built-in Kubernetes workloads, storage, networking, security, configuration, and cluster resources.
- Namespace filtering for namespaced resources.
- Client-side signals with server-rendered table fragments.
- Search across visible table cells.
- Shared informer-backed resource cache so page requests render from memory instead of issuing per-user Kubernetes list calls.
