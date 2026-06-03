import "./app.css";

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
];
const defaultUrlState = {
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
const sparklineCacheTTL = 60 * 1000;
const sparklineMaxConcurrentBatches = 2;
const sparklineMaxPodBatchItems = 8;
const sparklineMaxQueryBatchItems = 3;
const sparklineWindowSeconds = 3600;
const sparklineStepSeconds = 180;
let sparklineActiveBatches = 0;
let sparklineBatchTimer = 0;
const sparklineBatchQueue = [];
const sparklineCache = new Map();
const sparklineInflight = new Map();

class KubernettoPromqlSparkline extends HTMLElement {
  constructor() {
    super();
    this.abortController = null;
    this.loaded = false;
    this.loading = false;
    this.observer = null;
  }

  connectedCallback() {
    if (!this.hasChildNodes()) {
      this.renderPlaceholder();
    }
    if (this.loaded || this.loading) {
      return;
    }
    if (!("IntersectionObserver" in window)) {
      this.load();
      return;
    }
    this.observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) {
        this.observer?.disconnect();
        this.observer = null;
        this.load();
      }
    }, { rootMargin: "120px" });
    this.observer.observe(this);
  }

  disconnectedCallback() {
    this.observer?.disconnect();
    this.observer = null;
    if (!this.loaded) {
      this.abortController?.abort();
    }
  }

  renderPlaceholder() {
    this.innerHTML = `<div class="sparkline-placeholder" title="Waiting for metrics" aria-hidden="true"></div>`;
  }

  renderEmpty(message = "No samples", kind = "empty") {
    this.innerHTML = `<span class="sparkline-empty ${kind}" title="${escapeHTML(message)}" aria-label="${escapeHTML(message)}">-</span>`;
  }

  async load() {
    if (this.loaded || this.loading) {
      return;
    }
    this.loading = true;
    this.abortController = new AbortController();
    try {
      const data = await loadSparklineData({
        context: this.getAttribute("cluster-context") || "",
        namespace: this.getAttribute("pod-namespace") || "",
        pod: this.getAttribute("pod-name") || "",
        cpuQuery: this.getAttribute("cpu-query") || "",
        memoryQuery: this.getAttribute("memory-query") || "",
        windowSeconds: sparklineWindowSeconds,
        stepSeconds: sparklineStepSeconds,
      }, this.abortController.signal);
      this.loaded = true;
      if (!data.available) {
        this.renderEmpty(data.message || "Prometheus data unavailable", "error");
        return;
      }
      this.renderSparkline(data.series || []);
    } catch (error) {
      if (error.name !== "AbortError") {
        this.loaded = true;
        this.renderEmpty(error.message || "Sparkline failed to load", "error");
      }
    } finally {
      this.loading = false;
      this.abortController = null;
    }
  }

  renderSparkline(series) {
    const cpuSamples = seriesSamples(series, "cpu");
    const memorySamples = seriesSamples(series, "memory");
    const domain = sparklineDomain(cpuSamples, memorySamples);
    const cpuScale = sparklineScale(cpuSamples, domain);
    const memoryScale = sparklineScale(memorySamples, domain);
    const cpuPoints = sparklinePoints(cpuSamples, domain, cpuScale);
    const memoryPoints = sparklinePoints(memorySamples, domain, memoryScale);
    if (!cpuPoints && !memoryPoints) {
      this.renderEmpty("No Prometheus samples");
      return;
    }
    this.innerHTML = `
      <div class="sparkline-frame">
        <svg class="table-sparkline" viewBox="0 0 120 36" preserveAspectRatio="none" role="img" aria-label="CPU and memory usage sparkline">
          ${memoryPoints ? `<polyline class="timeline-line memory" points="${memoryPoints}"></polyline>` : ""}
          ${cpuPoints ? `<polyline class="timeline-line cpu" points="${cpuPoints}"></polyline>` : ""}
          <line class="sparkline-cursor" x1="0" y1="3" x2="0" y2="33"></line>
          <circle class="sparkline-point memory" r="2.3" cx="0" cy="0"></circle>
          <circle class="sparkline-point cpu" r="2.3" cx="0" cy="0"></circle>
        </svg>
        <span class="sparkline-tooltip" role="status"></span>
      </div>
    `;
    this.bindSparklineHover({ cpuSamples, memorySamples, domain, cpuScale, memoryScale });
  }

  bindSparklineHover(data) {
    const frame = this.querySelector(".sparkline-frame");
    const svg = this.querySelector(".table-sparkline");
    const cursor = this.querySelector(".sparkline-cursor");
    const cpuPoint = this.querySelector(".sparkline-point.cpu");
    const memoryPoint = this.querySelector(".sparkline-point.memory");
    const tooltip = this.querySelector(".sparkline-tooltip");
    if (!frame || !svg || !cursor || !cpuPoint || !memoryPoint || !tooltip) {
      return;
    }

    const move = (event) => {
      const rect = svg.getBoundingClientRect();
      if (rect.width <= 0) {
        return;
      }
      const ratio = Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width));
      const x = ratio * 120;
      const timestamp = data.domain.start + ratio * Math.max(1, data.domain.end - data.domain.start);
      const cpu = nearestSample(data.cpuSamples, timestamp);
      const memory = nearestSample(data.memorySamples, timestamp);
      cursor.setAttribute("x1", x.toFixed(1));
      cursor.setAttribute("x2", x.toFixed(1));
      updateSparklinePoint(cpuPoint, cpu, data.domain, data.cpuScale);
      updateSparklinePoint(memoryPoint, memory, data.domain, data.memoryScale);
      tooltip.innerHTML = sparklineTooltipHTML(cpu, memory);
      tooltip.style.left = `${Math.max(6, Math.min(rect.width - 6, ratio * rect.width))}px`;
      frame.classList.add("active");
    };
    const leave = () => {
      frame.classList.remove("active");
    };
    frame.addEventListener("pointermove", move);
    frame.addEventListener("pointerleave", leave);
    frame.addEventListener("focusin", () => {
      move({ clientX: svg.getBoundingClientRect().left + svg.getBoundingClientRect().width });
    });
    frame.addEventListener("focusout", leave);
  }
}

function loadSparklineData(request, signal) {
  const key = sparklineCacheKey(request);
  const cached = sparklineCache.get(key);
  if (cached && Date.now() - cached.updatedAt < sparklineCacheTTL) {
    return withAbort(Promise.resolve(cached.data), signal);
  }

  const inflight = sparklineInflight.get(key);
  if (inflight) {
    return withAbort(inflight, signal);
  }

  const promise = enqueueSparklineBatch(request, key)
    .finally(() => {
      sparklineInflight.delete(key);
    });
  sparklineInflight.set(key, promise);
  return withAbort(promise, signal);
}

function enqueueSparklineBatch(request, key) {
  return new Promise((resolve, reject) => {
    sparklineBatchQueue.push({ request, key, resolve, reject });
    scheduleSparklineBatchDrain();
  });
}

function scheduleSparklineBatchDrain() {
  if (sparklineBatchTimer) {
    return;
  }
  sparklineBatchTimer = window.setTimeout(() => {
    sparklineBatchTimer = 0;
    drainSparklineBatches();
  }, 25);
}

function drainSparklineBatches() {
  while (sparklineActiveBatches < sparklineMaxConcurrentBatches && sparklineBatchQueue.length > 0) {
    const batch = nextSparklineBatch();
    sparklineActiveBatches += 1;
    runSparklineBatch(batch)
      .finally(() => {
        sparklineActiveBatches -= 1;
        if (sparklineBatchQueue.length > 0) {
          scheduleSparklineBatchDrain();
        }
      });
  }
}

function nextSparklineBatch() {
  const first = sparklineBatchQueue.shift();
  const batch = [first];
  const limit = sparklineBatchLimit(first.request);
  for (let index = 0; index < sparklineBatchQueue.length && batch.length < limit;) {
    const item = sparklineBatchQueue[index];
    if (sparklineBatchCompatible(first.request, item.request)) {
      batch.push(item);
      sparklineBatchQueue.splice(index, 1);
    } else {
      index += 1;
    }
  }
  return batch;
}

async function runSparklineBatch(batch) {
  if (batch.every((item) => canUsePodBatch(item.request))) {
    await runPodSparklineBatch(batch);
    return;
  }
  await runQuerySparklineBatch(batch);
}

async function runPodSparklineBatch(batch) {
  try {
    const first = batch[0].request;
    const response = await fetch("/ui/prometheus/query-range", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        context: first.context,
        windowSeconds: first.windowSeconds,
        stepSeconds: first.stepSeconds,
        queries: [
          { name: "cpu", query: podBatchCPUQuery(batch.map((item) => item.request)) },
          { name: "memory", query: podBatchMemoryQuery(batch.map((item) => item.request)) },
        ],
      }),
    });
    const data = await response.json();
    if (!response.ok || !data.available) {
      throw new Error(data.message || data.error || "Prometheus data unavailable");
    }

    const seriesByPod = podBatchSeriesByPod(data.series || []);
    for (const item of batch) {
      const itemData = {
        ...data,
        series: seriesByPod.get(podBatchKey(item.request.namespace, item.request.pod)) || [],
      };
      sparklineCache.set(item.key, { data: itemData, updatedAt: Date.now() });
      item.resolve(itemData);
    }
  } catch (error) {
    for (const item of batch) {
      item.reject(error);
    }
  }
}

async function runQuerySparklineBatch(batch) {
  try {
    const first = batch[0].request;
    const response = await fetch("/ui/prometheus/query-range", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        context: first.context,
        windowSeconds: first.windowSeconds,
        stepSeconds: first.stepSeconds,
        queries: batch.flatMap((item, index) => [
          { name: `${index}:cpu`, query: item.request.cpuQuery },
          { name: `${index}:memory`, query: item.request.memoryQuery },
        ]),
      }),
    });
    const data = await response.json();
    if (!response.ok || !data.available) {
      throw new Error(data.message || data.error || "Prometheus data unavailable");
    }

    for (const [index, item] of batch.entries()) {
      const prefix = `${index}:`;
      const itemData = {
        ...data,
        series: (data.series || [])
          .filter((series) => series.name?.startsWith(prefix))
          .map((series) => ({ ...series, name: series.name.slice(prefix.length) })),
      };
      sparklineCache.set(item.key, { data: itemData, updatedAt: Date.now() });
      item.resolve(itemData);
    }
  } catch (error) {
    for (const item of batch) {
      item.reject(error);
    }
  }
}

function sparklineBatchCompatible(a, b) {
  return a.context === b.context &&
    a.windowSeconds === b.windowSeconds &&
    a.stepSeconds === b.stepSeconds &&
    canUsePodBatch(a) === canUsePodBatch(b);
}

function sparklineBatchLimit(request) {
  return canUsePodBatch(request) ? sparklineMaxPodBatchItems : sparklineMaxQueryBatchItems;
}

function sparklineCacheKey(request) {
  if (canUsePodBatch(request)) {
    return JSON.stringify({
      context: request.context,
      namespace: request.namespace,
      pod: request.pod,
      windowSeconds: request.windowSeconds,
      stepSeconds: request.stepSeconds,
    });
  }
  return JSON.stringify(request);
}

function canUsePodBatch(request) {
  return Boolean(request.namespace && request.pod);
}

function podBatchCPUQuery(requests) {
  return `sum by (namespace, pod) (${podBatchSelectors(requests, "container_cpu_usage_seconds_total")
    .map((selector) => `rate(${selector}[5m])`)
    .join(" or ")})`;
}

function podBatchMemoryQuery(requests) {
  return `sum by (namespace, pod) (${podBatchSelectors(requests, "container_memory_working_set_bytes").join(" or ")})`;
}

function podBatchSelectors(requests, metric) {
  return [...podBatchPodsByNamespace(requests).entries()]
    .map(([namespace, pods]) => `${metric}{namespace="${escapePrometheusLabelValue(namespace)}",pod=~"${podBatchPodRegex(pods)}",container!="",image!=""}`);
}

function podBatchPodsByNamespace(requests) {
  const podsByNamespace = new Map();
  for (const request of requests) {
    if (!podsByNamespace.has(request.namespace)) {
      podsByNamespace.set(request.namespace, new Set());
    }
    podsByNamespace.get(request.namespace).add(request.pod);
  }
  return new Map([...podsByNamespace.entries()].map(([namespace, pods]) => [namespace, [...pods].sort()]));
}

function podBatchPodRegex(pods) {
  return escapePrometheusLabelValue(pods.map(escapeRegexLiteral).join("|"));
}

function podBatchSeriesByPod(series) {
  const byPod = new Map();
  for (const item of series) {
    const key = podBatchKey(item.metric?.namespace || "", item.metric?.pod || "");
    if (!byPod.has(key)) {
      byPod.set(key, []);
    }
    byPod.get(key).push(item);
  }
  return byPod;
}

function podBatchKey(namespace, pod) {
  return `${namespace}\u0000${pod}`;
}

function escapeRegexLiteral(value) {
  return String(value).replace(/[\\^$.*+?()[\]{}|]/g, "\\$&");
}

function escapePrometheusLabelValue(value) {
  return String(value)
    .replaceAll("\\", "\\\\")
    .replaceAll("\n", "\\n")
    .replaceAll('"', '\\"');
}

function withAbort(promise, signal) {
  if (signal.aborted) {
    return Promise.reject(abortError());
  }
  return new Promise((resolve, reject) => {
    const onAbort = () => reject(abortError());
    signal.addEventListener("abort", onAbort, { once: true });
    promise.then(resolve, reject).finally(() => {
      signal.removeEventListener("abort", onAbort);
    });
  });
}

function abortError() {
  return new DOMException("Sparkline load was aborted.", "AbortError");
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function seriesSamples(series, name) {
  return series
    .filter((item) => item.name === name)
    .flatMap((item) => item.samples || [])
    .map((sample) => ({ timestamp: Date.parse(sample.timestamp), value: Number(sample.value) }))
    .filter((sample) => Number.isFinite(sample.timestamp) && Number.isFinite(sample.value))
    .sort((a, b) => a.timestamp - b.timestamp);
}

function sparklineDomain(...sampleSets) {
  const timestamps = sampleSets
    .flat()
    .map((sample) => sample.timestamp)
    .filter(Number.isFinite);
  if (!timestamps.length) {
    return { start: 0, end: 1 };
  }
  const start = Math.min(...timestamps);
  const end = Math.max(...timestamps);
  return { start, end: end > start ? end : start + 1 };
}

function sparklineScale(samples, domain) {
  const values = samples
    .filter((sample) => sample.timestamp >= domain.start && sample.timestamp <= domain.end)
    .map((sample) => sample.value);
  return Math.max(...values, 0);
}

function sparklinePoints(samples, domain, maxValue) {
  if (!samples.length) {
    return "";
  }
  if (maxValue <= 0) {
    return "";
  }
  const width = 120;
  const height = 36;
  const pad = 3;
  if (samples.length === 1) {
    const y = sparklineY(samples[0].value, maxValue, height, pad);
    return `0 ${y.toFixed(1)} ${width.toFixed(1)} ${y.toFixed(1)}`;
  }
  return samples.map((sample) => {
    const x = (sample.timestamp - domain.start) / (domain.end - domain.start) * width;
    const y = sparklineY(sample.value, maxValue, height, pad);
    return `${x.toFixed(1)} ${y.toFixed(1)}`;
  }).join(" ");
}

function sparklineY(value, maxValue, height, pad) {
  if (value <= 0 || maxValue <= 0) {
    return height - pad;
  }
  const ratio = Math.max(0, Math.min(1, value / maxValue));
  return pad + (1 - ratio) * (height - pad * 2);
}

function nearestSample(samples, timestamp) {
  if (!samples.length) {
    return null;
  }
  return samples.reduce((nearest, sample) => {
    if (!nearest) {
      return sample;
    }
    return Math.abs(sample.timestamp - timestamp) < Math.abs(nearest.timestamp - timestamp) ? sample : nearest;
  }, null);
}

function updateSparklinePoint(point, sample, domain, maxValue) {
  if (!sample || maxValue <= 0) {
    point.classList.remove("active");
    return;
  }
  const x = (sample.timestamp - domain.start) / (domain.end - domain.start) * 120;
  const y = sparklineY(sample.value, maxValue, 36, 3);
  point.setAttribute("cx", x.toFixed(1));
  point.setAttribute("cy", y.toFixed(1));
  point.classList.add("active");
}

function sparklineTooltipHTML(cpu, memory) {
  const timestamp = cpu?.timestamp || memory?.timestamp || Date.now();
  return `
    <span>${escapeHTML(formatSparklineTime(timestamp))}</span>
    <strong class="cpu">CPU ${escapeHTML(formatCPU(cpu?.value))}</strong>
    <strong class="memory">MEM ${escapeHTML(formatMemory(memory?.value))}</strong>
  `;
}

function formatSparklineTime(timestamp) {
  return new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(timestamp));
}

function formatCPU(value) {
  if (!Number.isFinite(value)) {
    return "-";
  }
  if (value < 1) {
    return `${Math.round(value * 1000)}m`;
  }
  return `${value.toFixed(value < 10 ? 2 : 1)} cores`;
}

function formatMemory(value) {
  if (!Number.isFinite(value)) {
    return "-";
  }
  const units = ["B", "Ki", "Mi", "Gi", "Ti"];
  let next = value;
  let unit = 0;
  while (Math.abs(next) >= 1024 && unit < units.length - 1) {
    next /= 1024;
    unit += 1;
  }
  const decimals = next >= 10 || unit === 0 ? 0 : 1;
  return `${next.toFixed(decimals)}${units[unit]}`;
}

if (!customElements.get("kubernetto-promql-sparkline")) {
  customElements.define("kubernetto-promql-sparkline", KubernettoPromqlSparkline);
}

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
  const clusters = activeClusterSelection();
  const activeResource = activeResourceButton();
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

function activeClusterSelection() {
  const buttons = Array.from(document.querySelectorAll("[data-cluster-context]"));
  if (!buttons.length) {
    return null;
  }
  const active = buttons
    .filter((button) => button.getAttribute("aria-pressed") === "true")
    .map((button) => button.dataset.clusterContext || "")
    .filter(Boolean);
  return clusterSelectionUrlValue(active.join(","));
}

function activeResourceButton() {
  return document.querySelector(".resource-child-button[data-resource-kind][aria-pressed='true']")
    || document.querySelector(".resource-group-button[data-resource-kind][aria-pressed='true']");
}

function clusterSelectionUrlValue(value) {
  const selected = String(value || "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
  if (!selected.length) {
    return "";
  }
  return selected.join(",");
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

function applyOptimisticResourceNav(resourceButton) {
  const nav = resourceButton.closest("#resource-nav");
  const group = resourceButton.closest("[data-resource-group]");
  const resourceKind = resourceButton.dataset.resourceKind || defaultUrlState.resource;
  if (!nav || !group) {
    return;
  }

  for (const button of nav.querySelectorAll(".resource-group-button, .resource-child-button")) {
    button.setAttribute("aria-pressed", "false");
  }
  for (const children of nav.querySelectorAll("[data-resource-group-children]")) {
    children.hidden = true;
  }

  const groupButton = group.querySelector(".resource-group-button");
  const children = group.querySelector("[data-resource-group-children]");
  groupButton?.setAttribute("aria-pressed", "true");
  if (children) {
    children.hidden = false;
  }
  for (const button of nav.querySelectorAll(`[data-resource-kind="${CSS.escape(resourceKind)}"]`)) {
    if (button.closest("[data-resource-group]") === group) {
      button.setAttribute("aria-pressed", "true");
    }
  }
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
    applyOptimisticResourceNav(resourceButton);
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

  const clusterButton = event.target.closest("[data-cluster-context]");
  if (clusterButton) {
    syncUrlState({
      clusters: clusterSelectionUrlValue(clusterButton.dataset.clusterSelection || ""),
      selectedName: "",
      selectedNamespace: "",
      detailMode: "overview",
    }, { mode: "push" });
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
    const patch = {
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
  const patch = {
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
