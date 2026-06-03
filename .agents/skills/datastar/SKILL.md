---
name: datastar
description: Maintain and debug Kubernetto's Datastar server-rendered UI. Use when Codex works on Datastar attributes, signals, SSE handlers, templ-generated fragments, loading indicators, optimistic UI updates, URL/query-string sync, Datastar dependency upgrades, or responsiveness issues in this repository.
---

# Datastar

Use this skill for Kubernetto's Datastar + Go + templ UI. The app is intentionally server-rendered: keep Datastar unless the user explicitly asks for a different architecture.

## Workflow

1. Inspect the local patterns before editing:
   - `internal/server/ui/render.go` for signal attributes and Datastar action attributes.
   - `internal/server/ui/page.templ` for page layout, Datastar script loading, and fragment roots.
   - `internal/server/server.go` for `datastar.ReadSignals`, `datastar.NewSSE`, signal patches, and element patches.
   - `frontend/src/app.js` for URL sync and small optimistic UI helpers.
   - `frontend/src/app.css` for loading and pending states.
2. For detailed implementation patterns, read `references/kubernetto-patterns.md`.
3. When upgrading Datastar, verify current releases/changelogs from official sources before editing versions. Browser package tags and `datastar-go` module versions can move independently.
4. Keep signal names stable in Go JSON structs and JavaScript URL state. In HTML attributes, use kebab-case for camelCase signal names because browsers lowercase attribute names.
5. Prefer Datastar-native loading behavior:
   - Add `data-indicator:loading` to the element initiating the request.
   - Bind visible UI to `$loading` with `data-show`, `data-class`, or similar reactive attributes.
6. Patch normalized server state back to the client before patching affected elements when handlers can canonicalize resource, namespace, selection, sort, or detail signals.
7. Regenerate and verify:
   - `mise x -- go generate ./...`
   - `mise x -- pnpm build:assets`
   - `mise x -- go test ./...`
   - `mise run build` when preparing a final branch/PR.

## Guardrails

- Do not treat `page_templ.go` or `assets/dist/*` as source of truth. Edit `*.templ`, `frontend/src/*`, and Go sources, then regenerate.
- Do not add a client framework for problems Datastar already handles: partial updates, loading indicators, simple optimistic state, or signal-driven requests.
- Keep SSE fragments rooted by stable IDs so Datastar can merge them predictably.
- Keep URL sync in `frontend/src/app.js` compatible with server-normalized signals.
- Add tests around rendered attributes and SSE event bodies when changing signal names, request handlers, or loading-state hooks.
