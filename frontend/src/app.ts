import "./app.css";
import "./charts";

const themeKey = "kubernetto-theme";
const themeOptions = new Set(["auto", "light", "dark"]);
const urlStateKeys = [
  "context",
  "clusters",
  "resource",
  "namespace",
  "query",
  "sortColumn",
  "sortOrder",
  "selectedName",
  "selectedNamespace",
  "detailMode",
] as const;
type UrlStateKey = typeof urlStateKeys[number];
type UrlState = Record<UrlStateKey, string>;
type UrlStatePatch = Partial<UrlState>;
type UrlSyncOptions = {
  mode?: "replace" | "push";
};

const defaultUrlState: UrlState = {
  context: "",
  clusters: "",
  resource: "overview",
  namespace: "",
  query: "",
  sortColumn: "",
  sortOrder: "",
  selectedName: "",
  selectedNamespace: "",
  detailMode: "overview",
};
let urlSyncTimer = 0;
let querySyncTimer = 0;
let restoringHistory = false;

function savedTheme(): string {
  try {
    const value = localStorage.getItem(themeKey);
    return value && themeOptions.has(value) ? value : "auto";
  } catch {
    return "auto";
  }
}

function applyTheme(theme: string) {
  const nextTheme = themeOptions.has(theme) ? theme : "auto";
  document.documentElement.dataset.theme = nextTheme;
  for (const button of document.querySelectorAll<HTMLElement>("[data-theme-option]")) {
    button.setAttribute("aria-pressed", String(button.dataset.themeOption === nextTheme));
  }
}

function readUrlState(): UrlState {
  const params = new URLSearchParams(window.location.search);
  const state = { ...defaultUrlState };
  for (const key of urlStateKeys) {
    const value = params.get(key);
    if (value !== null) {
      state[key] = value;
    }
  }
  return normalizeUrlState(state);
}

function readDOMState(): UrlStatePatch {
  const state: UrlStatePatch = {};
  const context = document.querySelector<HTMLSelectElement>("#context");
  const namespace = document.querySelector<HTMLSelectElement>("#namespace");
  const query = document.querySelector<HTMLInputElement>("#query");
  const clusters = activeClusterSelection();
  const activeResource = activeResourceButton();
  const activeSort = document.querySelector<HTMLElement>("th[aria-sort='ascending'] [data-sort-column], th[aria-sort='descending'] [data-sort-column]");
  const selectedRow = document.querySelector<HTMLElement>("tr[data-selected='true'][data-row-name]");
  const activeDetailMode = document.querySelector<HTMLElement>("[data-detail-mode][aria-selected='true']");

  if (context) {
    state.context = context.value;
  }
  if (namespace && !namespace.disabled) {
    state.namespace = namespace.value;
  } else if (namespace && namespace.disabled) {
    state.namespace = "";
  }
  if (query) {
    state.query = query.value;
  }
  if (clusters !== null) {
    state.clusters = clusters;
  }
  if (activeResource) {
    state.resource = activeResource.dataset.resourceKind || "";
  }
  if (activeSort) {
    state.sortColumn = activeSort.dataset.sortColumn || "";
    state.sortOrder = activeSort.closest("th")?.getAttribute("aria-sort") === "descending" ? "desc" : "asc";
  } else {
    state.sortColumn = "";
    state.sortOrder = "";
  }
  if (selectedRow) {
    state.selectedName = selectedRow.dataset.rowName || "";
    state.selectedNamespace = selectedRow.dataset.rowNamespace || "";
    if (selectedRow.dataset.rowCluster) {
      state.context = selectedRow.dataset.rowCluster;
    }
  }
  if (activeDetailMode) {
    state.detailMode = activeDetailMode.dataset.detailMode || "overview";
  }

  return normalizeUrlState(state);
}

function activeClusterSelection(): string | null {
  const buttons = Array.from(document.querySelectorAll<HTMLElement>("[data-cluster-context]"));
  if (!buttons.length) {
    return null;
  }
  const active = buttons
    .filter((button) => button.getAttribute("aria-pressed") === "true")
    .map((button) => button.dataset.clusterContext || "")
    .filter(Boolean);
  return clusterSelectionUrlValue(active.join(","));
}

function activeResourceButton(): HTMLElement | null {
  return document.querySelector<HTMLElement>(".resource-child-button[data-resource-kind][aria-pressed='true']")
    || document.querySelector<HTMLElement>(".resource-group-button[data-resource-kind][aria-pressed='true']");
}

function clusterSelectionUrlValue(value: string): string {
  const selected = String(value || "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
  if (!selected.length) {
    return "";
  }
  return selected.join(",");
}

function normalizeUrlState(state: UrlStatePatch): UrlState {
  const next = { ...defaultUrlState, ...state };
  if (next.sortOrder !== "asc" && next.sortOrder !== "desc") {
    next.sortColumn = "";
    next.sortOrder = "";
  }
  if (!next.sortColumn) {
    next.sortOrder = "";
  }
  if (next.detailMode !== "events" && next.detailMode !== "yaml") {
    next.detailMode = "overview";
  }
  if (!next.selectedName) {
    next.selectedNamespace = "";
    next.detailMode = "overview";
  }
  if (!next.resource) {
    next.resource = defaultUrlState.resource;
  }
  return next;
}

function writeUrlState(state: UrlStatePatch, mode: "replace" | "push" = "replace") {
  if (!window.history.replaceState) {
    return;
  }
  const nextState = normalizeUrlState(state);
  const url = new URL(window.location.href);
  for (const key of urlStateKeys) {
    url.searchParams.delete(key);
  }
  for (const key of urlStateKeys) {
    const value = nextState[key] || "";
    if (value && !(key === "detailMode" && value === defaultUrlState.detailMode)) {
      url.searchParams.set(key, value);
    }
  }
  const nextURL = url.pathname + url.search + url.hash;
  if (nextURL !== window.location.pathname + window.location.search + window.location.hash) {
    const historyMethod = mode === "push" ? "pushState" : "replaceState";
    window.history[historyMethod](nextState, "", nextURL);
  } else {
    window.history.replaceState(nextState, "", nextURL);
  }
}

function syncUrlState(patch: UrlStatePatch = {}, options: UrlSyncOptions = {}) {
  if (restoringHistory) {
    return;
  }
  writeUrlState({ ...readUrlState(), ...readDOMState(), ...patch }, options.mode);
}

function scheduleUrlSync(patch: UrlStatePatch = {}, delay = 0, options: UrlSyncOptions = {}) {
  window.clearTimeout(urlSyncTimer);
  urlSyncTimer = window.setTimeout(() => syncUrlState(patch, options), delay);
}

function currentSortState(): { column: string; order: string } {
  const state = { ...readUrlState(), ...readDOMState() };
  return { column: state.sortColumn || "", order: state.sortOrder || "" };
}

function nextSortState(column: string): Pick<UrlState, "sortColumn" | "sortOrder"> {
  const current = currentSortState();
  if (current.column !== column) {
    return { sortColumn: column, sortOrder: "asc" };
  }
  if (current.order === "asc") {
    return { sortColumn: column, sortOrder: "desc" };
  }
  return { sortColumn: "", sortOrder: "" };
}

function eventElement(event: Event): Element | null {
  return event.target instanceof Element ? event.target : null;
}

function isEditableShortcutTarget(target: Element | null): boolean {
  if (!target) {
    return false;
  }
  if (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement || target instanceof HTMLSelectElement) {
    return true;
  }
  return Boolean(target.closest("[contenteditable=''], [contenteditable='true']"));
}

function focusSearchInput() {
  const query = document.querySelector<HTMLInputElement>("#query");
  if (!query) {
    return;
  }
  query.focus();
}

function applyOptimisticResourceNav(resourceButton: HTMLElement) {
  const nav = resourceButton.closest("#resource-nav");
  const group = resourceButton.closest("[data-resource-group]");
  const resourceKind = resourceButton.dataset.resourceKind || defaultUrlState.resource;
  if (!nav || !group) {
    return;
  }

  for (const button of nav.querySelectorAll<HTMLElement>(".resource-group-button, .resource-child-button")) {
    button.setAttribute("aria-pressed", "false");
  }
  for (const children of nav.querySelectorAll<HTMLElement>("[data-resource-group-children]")) {
    children.hidden = true;
  }

  const groupButton = group.querySelector<HTMLElement>(".resource-group-button");
  const children = group.querySelector<HTMLElement>("[data-resource-group-children]");
  groupButton?.setAttribute("aria-pressed", "true");
  if (children) {
    children.hidden = false;
  }
  for (const button of nav.querySelectorAll<HTMLElement>(`[data-resource-kind="${CSS.escape(resourceKind)}"]`)) {
    if (button.closest("[data-resource-group]") === group) {
      button.setAttribute("aria-pressed", "true");
    }
  }
}

applyTheme(savedTheme());

document.addEventListener("click", (event) => {
  const target = eventElement(event);
  const button = target?.closest<HTMLElement>("[data-theme-option]");
  if (!button) {
    return;
  }
  const nextTheme = button.dataset.themeOption;
  if (!nextTheme) {
    return;
  }
  if (!themeOptions.has(nextTheme)) {
    return;
  }
  try {
    localStorage.setItem(themeKey, nextTheme);
  } catch {}
  applyTheme(nextTheme);
});

document.addEventListener("click", (event) => {
  const target = eventElement(event);
  if (!target) {
    return;
  }
  const fleetButton = target.closest<HTMLElement>("[data-fleet-context]");
  if (fleetButton) {
    const context = fleetButton.dataset.fleetContext || "";
    if (fleetButton.dataset.fleetIssue === "true") {
      syncUrlState({
        context,
        clusters: context,
        resource: fleetButton.dataset.fleetIssueKind || defaultUrlState.resource,
        namespace: "",
        query: fleetButton.dataset.fleetIssueQuery || "",
        sortColumn: "",
        sortOrder: "",
        selectedName: "",
        selectedNamespace: "",
        detailMode: "overview",
      }, { mode: "push" });
    } else {
      syncUrlState({
        context,
        clusters: context,
        resource: defaultUrlState.resource,
        namespace: "",
        query: "",
        sortColumn: "",
        sortOrder: "",
        selectedName: "",
        selectedNamespace: "",
        detailMode: "overview",
      }, { mode: "push" });
    }
    return;
  }

  const resourceButton = target.closest<HTMLElement>("[data-resource-kind]");
  if (resourceButton) {
    applyOptimisticResourceNav(resourceButton);
    const patch: UrlStatePatch = {
      resource: resourceButton.dataset.resourceKind || defaultUrlState.resource,
      sortColumn: "",
      sortOrder: "",
      selectedName: "",
      selectedNamespace: "",
      detailMode: "overview",
    };
    if (resourceButton.dataset.resourceScope === "cluster") {
      patch.namespace = "";
    }
    syncUrlState({ ...patch }, { mode: "push" });
    return;
  }

  const clusterButton = target.closest<HTMLElement>("[data-cluster-context]");
  if (clusterButton) {
    syncUrlState({
      clusters: clusterSelectionUrlValue(clusterButton.dataset.clusterSelection || ""),
      selectedName: "",
      selectedNamespace: "",
      detailMode: "overview",
    }, { mode: "push" });
    return;
  }

  const sortButton = target.closest<HTMLElement>("[data-sort-column]");
  if (sortButton) {
    syncUrlState({
      ...nextSortState(sortButton.dataset.sortColumn || ""),
      selectedName: "",
      selectedNamespace: "",
      detailMode: "overview",
    }, { mode: "push" });
    return;
  }

  const row = target.closest<HTMLElement>("tr[data-row-name]");
  if (row) {
    const patch: UrlStatePatch = {
      selectedName: row.dataset.rowName || "",
      selectedNamespace: row.dataset.rowNamespace || "",
      detailMode: "overview",
    };
    if (row.dataset.rowCluster) {
      patch.context = row.dataset.rowCluster;
    }
    syncUrlState(patch, { mode: "push" });
    return;
  }

  if (target.closest("[data-close-detail]")) {
    syncUrlState({ selectedName: "", selectedNamespace: "", detailMode: "overview" }, { mode: "push" });
    return;
  }

  const detailModeButton = target.closest<HTMLElement>("[data-detail-mode]");
  if (detailModeButton) {
    syncUrlState({ detailMode: detailModeButton.dataset.detailMode || "overview" }, { mode: "push" });
    return;
  }

  scheduleUrlSync({}, 50);
});

document.addEventListener("keydown", (event) => {
  const target = eventElement(event);
  if (event.key === "/" && !event.altKey && !event.ctrlKey && !event.metaKey && !isEditableShortcutTarget(target)) {
    const query = document.querySelector<HTMLInputElement>("#query");
    if (query) {
      event.preventDefault();
      focusSearchInput();
    }
    return;
  }

  if (event.key !== "Enter") {
    return;
  }
  const row = target?.closest<HTMLElement>("tr[data-row-name]");
  if (!row) {
    return;
  }
  const patch: UrlStatePatch = {
    selectedName: row.dataset.rowName || "",
    selectedNamespace: row.dataset.rowNamespace || "",
    detailMode: "overview",
  };
  if (row.dataset.rowCluster) {
    patch.context = row.dataset.rowCluster;
  }
  syncUrlState(patch, { mode: "push" });
});

document.addEventListener("change", (event) => {
  const target = event.target;
  if (!(target instanceof HTMLSelectElement)) {
    return;
  }
  if (target.matches("#context")) {
    syncUrlState({
      context: target.value,
      namespace: "",
      sortColumn: "",
      sortOrder: "",
      selectedName: "",
      selectedNamespace: "",
      detailMode: "overview",
    }, { mode: "push" });
  } else if (target.matches("#namespace")) {
    syncUrlState({
      namespace: target.value,
      selectedName: "",
      selectedNamespace: "",
      detailMode: "overview",
    }, { mode: "push" });
  }
});

document.addEventListener("input", (event) => {
  const target = event.target;
  if (!(target instanceof HTMLInputElement) || !target.matches("#query")) {
    return;
  }
  window.clearTimeout(querySyncTimer);
  querySyncTimer = window.setTimeout(() => {
    syncUrlState({
      query: target.value,
      selectedName: "",
      selectedNamespace: "",
      detailMode: "overview",
    }, { mode: "push" });
  }, 250);
});

window.addEventListener("popstate", () => {
  restoringHistory = true;
  window.clearTimeout(urlSyncTimer);
  window.clearTimeout(querySyncTimer);
  window.location.reload();
});

document.addEventListener("DOMContentLoaded", () => {
  applyTheme(savedTheme());
  syncUrlState();

  const app = document.querySelector(".app");
  if (app) {
    const observer = new MutationObserver(() => scheduleUrlSync({}, 50));
    observer.observe(app, {
      attributes: true,
      childList: true,
      subtree: true,
      attributeFilter: ["aria-pressed", "aria-selected", "aria-sort", "data-selected", "disabled"],
    });
  }
});
