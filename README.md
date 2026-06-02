# Kubernetto

A small Kubernetes dashboard built as a single Go binary with a server-rendered Data-Star UI.

## Run

```sh
mise run dev
```

Then open <http://127.0.0.1:9832>.

Kubernetto loads every context from your kubeconfig, selecting the active context by default and falling back to in-cluster config when available. You can override either the listen address or kubeconfig path:

```sh
mise run app --addr 127.0.0.1:9833 --kubeconfig ~/.kube/config
```

You can also set only the local listen port:

```sh
mise run app --port 9833
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
- Resource tables for pods, deployments, statefulsets, daemonsets, services, ingresses, nodes, and namespaces.
- Namespace filtering for namespaced resources.
- Client-side signals with server-rendered table fragments.
- Search across visible table cells.
- Shared informer-backed resource cache so page requests render from memory instead of issuing per-user Kubernetes list calls.
