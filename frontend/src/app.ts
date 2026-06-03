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
let quickActiveIndex = 0;

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

function actionItemUrlPatch(actionItem: HTMLElement): UrlStatePatch {
  const context = actionItem.dataset.actionContext || "";
  return {
    context,
    clusters: context,
    resource: actionItem.dataset.actionKind || defaultUrlState.resource,
    namespace: "",
    query: actionItem.dataset.actionQuery || "",
    sortColumn: "",
    sortOrder: "",
    selectedName: actionItem.dataset.actionSelectedName || "",
    selectedNamespace: actionItem.dataset.actionSelectedNamespace || "",
    detailMode: "overview",
  };
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

function quickSwitcher(): HTMLElement | null {
  return document.querySelector<HTMLElement>("[data-quick-switcher]");
}

function quickInput(): HTMLInputElement | null {
  return document.querySelector<HTMLInputElement>("[data-quick-input]");
}

function quickResults(): HTMLElement[] {
  return Array.from(document.querySelectorAll<HTMLElement>("[data-quick-result]"));
}

function visibleQuickResults(): HTMLElement[] {
  return quickResults().filter((result) => !result.hidden);
}

function quickText(value: string | undefined): string {
  return String(value || "").trim().toLowerCase();
}

function quickFieldScore(field: string, term: string, scores: {
  exact: number;
  prefix: number;
  contains: number;
}): number {
  if (!field || !term) {
    return 0;
  }
  if (field === term) {
    return scores.exact;
  }
  if (field.startsWith(term)) {
    return scores.prefix;
  }
  if (field.includes(term)) {
    return scores.contains;
  }
  return 0;
}

function quickMatchScore(result: HTMLElement, terms: string[]): number {
  if (!terms.length) {
    return 0;
  }

  const label = quickText(result.dataset.quickLabel);
  const meta = quickText(result.dataset.quickMeta);
  const kind = quickText(result.dataset.quickKind);
  const context = quickText(result.dataset.quickContext);
  const clusters = quickText(result.dataset.quickClusters);
  const resource = quickText(result.dataset.quickResource);
  const namespace = quickText(result.dataset.quickNamespace);
  const selectedName = quickText(result.dataset.quickSelectedName);
  const selectedNamespace = quickText(result.dataset.quickSelectedNamespace);
  const search = quickText(result.dataset.quickSearch);
  let total = 0;

  for (const term of terms) {
    const termScore = Math.max(
      quickFieldScore(label, term, { exact: 140, prefix: 115, contains: 85 }),
      quickFieldScore(selectedName, term, { exact: 130, prefix: 110, contains: 80 }),
      quickFieldScore(namespace, term, { exact: 95, prefix: 75, contains: 45 }),
      quickFieldScore(selectedNamespace, term, { exact: 90, prefix: 70, contains: 42 }),
      quickFieldScore(kind, term, { exact: 80, prefix: 62, contains: 38 }),
      quickFieldScore(resource, term, { exact: 76, prefix: 58, contains: 34 }),
      quickFieldScore(meta, term, { exact: 44, prefix: 36, contains: 24 }),
      quickFieldScore(context, term, { exact: 24, prefix: 18, contains: 10 }),
      quickFieldScore(clusters, term, { exact: 22, prefix: 16, contains: 8 }),
      search.includes(term) ? 4 : 0,
    );
    if (termScore === 0) {
      return -1;
    }
    total += termScore;
  }

  return total;
}

function quickResultUrlPatch(result: HTMLElement): UrlStatePatch {
  return {
    context: result.dataset.quickContext || "",
    clusters: result.dataset.quickClusters || "",
    resource: result.dataset.quickResource || defaultUrlState.resource,
    namespace: result.dataset.quickNamespace || "",
    query: result.dataset.quickQuery || "",
    sortColumn: result.dataset.quickSortColumn || "",
    sortOrder: result.dataset.quickSortOrder || "",
    selectedName: result.dataset.quickSelectedName || "",
    selectedNamespace: result.dataset.quickSelectedNamespace || "",
    detailMode: result.dataset.quickDetailMode || "overview",
  };
}

function setQuickSelected(index: number) {
  const visible = visibleQuickResults();
  if (!visible.length) {
    quickActiveIndex = 0;
    for (const result of quickResults()) {
      result.setAttribute("aria-selected", "false");
    }
    return;
  }
  quickActiveIndex = ((index % visible.length) + visible.length) % visible.length;
  for (const result of quickResults()) {
    result.setAttribute("aria-selected", "false");
  }
  visible[quickActiveIndex]?.setAttribute("aria-selected", "true");
  visible[quickActiveIndex]?.scrollIntoView?.({ block: "nearest" });
}

function updateQuickResults() {
  const input = quickInput();
  const container = document.querySelector<HTMLElement>("[data-quick-results]");
  const terms = String(input?.value || "")
    .trim()
    .toLowerCase()
    .split(/\s+/)
    .filter(Boolean);
  let visibleCount = 0;
  const scored = quickResults().map((result, index) => ({
    result,
    index,
    score: quickMatchScore(result, terms),
  }));
  const sorted = [...scored].sort((left, right) => {
    const scoreDelta = right.score - left.score;
    if (scoreDelta !== 0) {
      return scoreDelta;
    }
    return Number(left.result.dataset.quickIndex || left.index) - Number(right.result.dataset.quickIndex || right.index);
  });

  for (const item of sorted) {
    const matched = terms.length === 0 || item.score >= 0;
    item.result.hidden = !matched;
    if (matched) {
      visibleCount += 1;
    }
    container?.append(item.result);
  }

  const count = document.querySelector<HTMLElement>("[data-quick-count]");
  if (count) {
    count.textContent = `${visibleCount} result${visibleCount === 1 ? "" : "s"}`;
  }
  const empty = document.querySelector<HTMLElement>("[data-quick-empty]");
  if (empty) {
    empty.hidden = visibleCount !== 0;
  }
  setQuickSelected(0);
}

function openQuickSwitcher() {
  const palette = quickSwitcher();
  const input = quickInput();
  if (!palette || !input) {
    return;
  }
  palette.hidden = false;
  input.value = "";
  updateQuickResults();
  window.setTimeout(() => input.focus(), 0);
}

function closeQuickSwitcher() {
  const palette = quickSwitcher();
  if (!palette || palette.hidden) {
    return;
  }
  palette.hidden = true;
}

function quickSwitcherOpen(): boolean {
  const palette = quickSwitcher();
  return Boolean(palette && !palette.hidden);
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
  if (target.closest("[data-quick-open]")) {
    openQuickSwitcher();
    return;
  }
  if (target.closest("[data-quick-close]")) {
    closeQuickSwitcher();
    return;
  }
  const quickResult = target.closest<HTMLElement>("[data-quick-result]");
  if (quickResult) {
    syncUrlState(quickResultUrlPatch(quickResult), { mode: "push" });
    closeQuickSwitcher();
    return;
  }
  const actionButton = target.closest<HTMLElement>("[data-action-context]");
  if (actionButton) {
    syncUrlState(actionItemUrlPatch(actionButton), { mode: "push" });
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
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
    event.preventDefault();
    openQuickSwitcher();
    return;
  }
  if (quickSwitcherOpen()) {
    if (event.key === "Escape") {
      event.preventDefault();
      closeQuickSwitcher();
      return;
    }
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setQuickSelected(quickActiveIndex + 1);
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      setQuickSelected(quickActiveIndex - 1);
      return;
    }
    if (event.key === "Home") {
      event.preventDefault();
      setQuickSelected(0);
      return;
    }
    if (event.key === "End") {
      event.preventDefault();
      setQuickSelected(visibleQuickResults().length - 1);
      return;
    }
    if (event.key === "Enter") {
      const result = visibleQuickResults()[quickActiveIndex];
      if (result) {
        event.preventDefault();
        result.click();
      }
      return;
    }
  }
  if (event.key !== "Enter") {
    return;
  }
  const target = eventElement(event);
  const actionButton = target?.closest<HTMLElement>("[data-action-context]");
  if (actionButton) {
    syncUrlState(actionItemUrlPatch(actionButton), { mode: "push" });
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
  if (target instanceof HTMLInputElement && target.matches("[data-quick-input]")) {
    updateQuickResults();
    return;
  }
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
  updateQuickResults();
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
