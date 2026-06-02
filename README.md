# Kubernetto

A small Kubernetes dashboard built as a single Go binary with a server-rendered Data-Star UI.

## Run

```sh
go run ./cmd/kubernetto
```

Then open <http://127.0.0.1:9832>.

Kubernetto uses the active kubeconfig by default, falling back to in-cluster config when available. You can override either the listen address or kubeconfig path:

```sh
go run ./cmd/kubernetto --addr 127.0.0.1:9833 --kubeconfig ~/.kube/config
```

You can also set only the local listen port:

```sh
go run ./cmd/kubernetto --port 9833
```

## Build

```sh
go build -o kubernetto ./cmd/kubernetto
```

## Templates

The UI is built with templ components. After editing `*.templ` files, regenerate the checked-in Go output:

```sh
go generate ./...
```

## Current Scope

- Cluster summary with live Data-Star SSE refresh.
- Resource tables for pods, deployments, statefulsets, daemonsets, services, ingresses, nodes, and namespaces.
- Namespace filtering for namespaced resources.
- Client-side signals with server-rendered table fragments.
- Search across visible table cells.
- Shared informer-backed resource cache so page requests render from memory instead of issuing per-user Kubernetes list calls.
