const themeKey = "kubernetto-theme";
const themeOptions = new Set(["auto", "light", "dark"]);
const urlStateKeys = [
  "context",
  "resource",
  "namespace",
  "query",
  "sortColumn",
  "sortOrder",
  "selectedName",
  "selectedNamespace",
  "detailMode",
];
const defaultUrlState = {
  context: "",
  resource: "pods",
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

function savedTheme() {
  try {
    const value = localStorage.getItem(themeKey);
    return themeOptions.has(value) ? value : "auto";
  } catch {
    return "auto";
  }
}

function applyTheme(theme) {
  const nextTheme = themeOptions.has(theme) ? theme : "auto";
  document.documentElement.dataset.theme = nextTheme;
  for (const button of document.querySelectorAll("[data-theme-option]")) {
    button.setAttribute("aria-pressed", String(button.dataset.themeOption === nextTheme));
  }
}

function readUrlState() {
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

function readDOMState() {
  const state = {};
  const context = document.querySelector("#context");
  const namespace = document.querySelector("#namespace");
  const query = document.querySelector("#query");
  const activeResource = document.querySelector("[data-resource-kind][aria-pressed='true']");
  const activeSort = document.querySelector("th[aria-sort='ascending'] [data-sort-column], th[aria-sort='descending'] [data-sort-column]");
  const selectedRow = document.querySelector("tr[data-selected='true'][data-row-name]");
  const activeDetailMode = document.querySelector("[data-detail-mode][aria-selected='true']");

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
  }
  if (activeDetailMode) {
    state.detailMode = activeDetailMode.dataset.detailMode || "overview";
  }

  return normalizeUrlState(state);
}

function normalizeUrlState(state) {
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

function writeUrlState(state, mode = "replace") {
  if (!window.history?.replaceState) {
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
    const historyMethod = mode === "push" && window.history.pushState ? "pushState" : "replaceState";
    window.history[historyMethod](nextState, "", nextURL);
  } else {
    window.history.replaceState(nextState, "", nextURL);
  }
}

function syncUrlState(patch = {}, options = {}) {
  if (restoringHistory) {
    return;
  }
  writeUrlState({ ...readUrlState(), ...readDOMState(), ...patch }, options.mode);
}

function scheduleUrlSync(patch = {}, delay = 0, options = {}) {
  window.clearTimeout(urlSyncTimer);
  urlSyncTimer = window.setTimeout(() => syncUrlState(patch, options), delay);
}

function currentSortState() {
  const state = { ...readUrlState(), ...readDOMState() };
  return { column: state.sortColumn || "", order: state.sortOrder || "" };
}

function nextSortState(column) {
  const current = currentSortState();
  if (current.column !== column) {
    return { sortColumn: column, sortOrder: "asc" };
  }
  if (current.order === "asc") {
    return { sortColumn: column, sortOrder: "desc" };
  }
  return { sortColumn: "", sortOrder: "" };
}

applyTheme(savedTheme());

document.addEventListener("click", (event) => {
  const button = event.target.closest("[data-theme-option]");
  if (!button) {
    return;
  }
  const nextTheme = button.dataset.themeOption;
  if (!themeOptions.has(nextTheme)) {
    return;
  }
  try {
    localStorage.setItem(themeKey, nextTheme);
  } catch {}
  applyTheme(nextTheme);
});

document.addEventListener("click", (event) => {
  const resourceButton = event.target.closest("[data-resource-kind]");
  if (resourceButton) {
    const patch = {
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

  const sortButton = event.target.closest("[data-sort-column]");
  if (sortButton) {
    syncUrlState({
      ...nextSortState(sortButton.dataset.sortColumn || ""),
      selectedName: "",
      selectedNamespace: "",
      detailMode: "overview",
    }, { mode: "push" });
    return;
  }

  const row = event.target.closest("tr[data-row-name]");
  if (row) {
    syncUrlState({
      selectedName: row.dataset.rowName || "",
      selectedNamespace: row.dataset.rowNamespace || "",
      detailMode: "overview",
    }, { mode: "push" });
    return;
  }

  if (event.target.closest("[data-close-detail]")) {
    syncUrlState({ selectedName: "", selectedNamespace: "", detailMode: "overview" }, { mode: "push" });
    return;
  }

  const detailModeButton = event.target.closest("[data-detail-mode]");
  if (detailModeButton) {
    syncUrlState({ detailMode: detailModeButton.dataset.detailMode || "overview" }, { mode: "push" });
    return;
  }

  scheduleUrlSync({}, 50);
});

document.addEventListener("keydown", (event) => {
  if (event.key !== "Enter") {
    return;
  }
  const row = event.target.closest("tr[data-row-name]");
  if (!row) {
    return;
  }
  syncUrlState({
    selectedName: row.dataset.rowName || "",
    selectedNamespace: row.dataset.rowNamespace || "",
    detailMode: "overview",
  }, { mode: "push" });
});

document.addEventListener("change", (event) => {
  if (event.target.matches("#context")) {
    syncUrlState({
      context: event.target.value,
      namespace: "",
      sortColumn: "",
      sortOrder: "",
      selectedName: "",
      selectedNamespace: "",
      detailMode: "overview",
    }, { mode: "push" });
  } else if (event.target.matches("#namespace")) {
    syncUrlState({
      namespace: event.target.value,
      selectedName: "",
      selectedNamespace: "",
      detailMode: "overview",
    }, { mode: "push" });
  }
});

document.addEventListener("input", (event) => {
  if (!event.target.matches("#query")) {
    return;
  }
  window.clearTimeout(querySyncTimer);
  querySyncTimer = window.setTimeout(() => {
    syncUrlState({
      query: event.target.value,
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
