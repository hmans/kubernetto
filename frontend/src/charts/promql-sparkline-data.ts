const sparklineCacheTTL = 60 * 1000;
const sparklineMaxConcurrentBatches = 2;
const sparklineMaxPodBatchItems = 8;
const sparklineMaxQueryBatchItems = 3;
let sparklineActiveBatches = 0;
let sparklineBatchTimer = 0;

export type PrometheusRangeSample = {
  timestamp: string;
  value: number;
};

export type PrometheusRangeSeries = {
  name: string;
  metric?: Record<string, string>;
  samples?: PrometheusRangeSample[];
};

export type PrometheusRangeData = {
  available: boolean;
  message?: string;
  error?: string;
  source?: string;
  window?: string;
  updatedAt?: string;
  series?: PrometheusRangeSeries[];
};

export type SparklineRequest = {
  context: string;
  namespace: string;
  pod: string;
  cpuQuery: string;
  memoryQuery: string;
  windowSeconds: number;
  stepSeconds: number;
};

type SparklineBatchItem = {
  request: SparklineRequest;
  key: string;
  resolve: (data: PrometheusRangeData) => void;
  reject: (error: unknown) => void;
};

const sparklineBatchQueue: SparklineBatchItem[] = [];
const sparklineCache = new Map<string, { data: PrometheusRangeData; updatedAt: number }>();
const sparklineInflight = new Map<string, Promise<PrometheusRangeData>>();

export function loadSparklineData(request: SparklineRequest, signal: AbortSignal): Promise<PrometheusRangeData> {
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

export function resetSparklineDataForTests(): void {
  if (sparklineBatchTimer) {
    window.clearTimeout(sparklineBatchTimer);
  }
  sparklineActiveBatches = 0;
  sparklineBatchTimer = 0;
  sparklineBatchQueue.splice(0);
  sparklineCache.clear();
  sparklineInflight.clear();
}

function enqueueSparklineBatch(request: SparklineRequest, key: string): Promise<PrometheusRangeData> {
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

function nextSparklineBatch(): SparklineBatchItem[] {
  const first = sparklineBatchQueue.shift();
  if (!first) {
    return [];
  }
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

async function runSparklineBatch(batch: SparklineBatchItem[]) {
  if (!batch.length) {
    return;
  }
  if (batch.every((item) => canUsePodBatch(item.request))) {
    await runPodSparklineBatch(batch);
    return;
  }
  await runQuerySparklineBatch(batch);
}

async function runPodSparklineBatch(batch: SparklineBatchItem[]) {
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
    const data = await response.json() as PrometheusRangeData;
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

async function runQuerySparklineBatch(batch: SparklineBatchItem[]) {
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
    const data = await response.json() as PrometheusRangeData;
    if (!response.ok || !data.available) {
      throw new Error(data.message || data.error || "Prometheus data unavailable");
    }

    for (const [index, item] of batch.entries()) {
      const prefix = `${index}:`;
      const itemData = {
        ...data,
        series: (data.series || [])
          .filter((series) => series.name.startsWith(prefix))
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

function sparklineBatchCompatible(a: SparklineRequest, b: SparklineRequest): boolean {
  return a.context === b.context &&
    a.windowSeconds === b.windowSeconds &&
    a.stepSeconds === b.stepSeconds &&
    canUsePodBatch(a) === canUsePodBatch(b);
}

function sparklineBatchLimit(request: SparklineRequest): number {
  return canUsePodBatch(request) ? sparklineMaxPodBatchItems : sparklineMaxQueryBatchItems;
}

function sparklineCacheKey(request: SparklineRequest): string {
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

function canUsePodBatch(request: SparklineRequest): boolean {
  return Boolean(request.namespace && request.pod);
}

function podBatchCPUQuery(requests: SparklineRequest[]): string {
  return `sum by (namespace, pod) (${podBatchSelectors(requests, "container_cpu_usage_seconds_total")
    .map((selector) => `rate(${selector}[5m])`)
    .join(" or ")})`;
}

function podBatchMemoryQuery(requests: SparklineRequest[]): string {
  return `sum by (namespace, pod) (${podBatchSelectors(requests, "container_memory_working_set_bytes").join(" or ")})`;
}

function podBatchSelectors(requests: SparklineRequest[], metric: string): string[] {
  return [...podBatchPodsByNamespace(requests).entries()]
    .map(([namespace, pods]) => `${metric}{namespace="${escapePrometheusLabelValue(namespace)}",pod=~"${podBatchPodRegex(pods)}",container!="",image!=""}`);
}

function podBatchPodsByNamespace(requests: SparklineRequest[]): Map<string, string[]> {
  const podsByNamespace = new Map<string, Set<string>>();
  for (const request of requests) {
    if (!podsByNamespace.has(request.namespace)) {
      podsByNamespace.set(request.namespace, new Set());
    }
    podsByNamespace.get(request.namespace)?.add(request.pod);
  }
  return new Map([...podsByNamespace.entries()].map(([namespace, pods]) => [namespace, [...pods].sort()]));
}

function podBatchPodRegex(pods: string[]): string {
  return escapePrometheusLabelValue(pods.map(escapeRegexLiteral).join("|"));
}

function podBatchSeriesByPod(series: PrometheusRangeSeries[]): Map<string, PrometheusRangeSeries[]> {
  const byPod = new Map<string, PrometheusRangeSeries[]>();
  for (const item of series) {
    const key = podBatchKey(item.metric?.namespace || "", item.metric?.pod || "");
    if (!byPod.has(key)) {
      byPod.set(key, []);
    }
    byPod.get(key)?.push(item);
  }
  return byPod;
}

function podBatchKey(namespace: string, pod: string): string {
  return `${namespace}\u0000${pod}`;
}

function escapeRegexLiteral(value: string): string {
  return String(value).replace(/[\\^$.*+?()[\]{}|]/g, "\\$&");
}

function escapePrometheusLabelValue(value: string): string {
  return String(value)
    .replaceAll("\\", "\\\\")
    .replaceAll("\n", "\\n")
    .replaceAll('"', '\\"');
}

function withAbort<T>(promise: Promise<T>, signal: AbortSignal): Promise<T> {
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
