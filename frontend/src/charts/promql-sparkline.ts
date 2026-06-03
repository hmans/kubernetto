import "./promql-sparkline.css";
import { loadSparklineData, type PrometheusRangeSeries } from "./promql-sparkline-data";

const sparklineWindowSeconds = 3600;
const sparklineStepSeconds = 180;

type SparklineSample = {
  timestamp: number;
  value: number;
};

type SparklineDomain = {
  start: number;
  end: number;
};

type SparklineHoverData = {
  cpuSamples: SparklineSample[];
  memorySamples: SparklineSample[];
  domain: SparklineDomain;
  cpuScale: number;
  memoryScale: number;
};

export class KubernettoPromqlSparkline extends HTMLElement {
  abortController: AbortController | null;
  loaded: boolean;
  loading: boolean;
  observer: IntersectionObserver | null;

  constructor() {
    super();
    this.abortController = null;
    this.loaded = false;
    this.loading = false;
    this.observer = null;
  }

  connectedCallback(): void {
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

  disconnectedCallback(): void {
    this.observer?.disconnect();
    this.observer = null;
    if (!this.loaded) {
      this.abortController?.abort();
    }
  }

  renderPlaceholder(): void {
    this.innerHTML = `<div class="sparkline-placeholder" title="Waiting for metrics" aria-hidden="true"></div>`;
  }

  renderEmpty(message = "No samples", kind = "empty"): void {
    this.innerHTML = `<span class="sparkline-empty ${kind}" title="${escapeHTML(message)}" aria-label="${escapeHTML(message)}">-</span>`;
  }

  async load(): Promise<void> {
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
      if (!(error instanceof DOMException && error.name === "AbortError")) {
        this.loaded = true;
        const message = error instanceof Error ? error.message : "Sparkline failed to load";
        this.renderEmpty(message || "Sparkline failed to load", "error");
      }
    } finally {
      this.loading = false;
      this.abortController = null;
    }
  }

  renderSparkline(series: PrometheusRangeSeries[]): void {
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

  bindSparklineHover(data: SparklineHoverData): void {
    const frame = this.querySelector<HTMLElement>(".sparkline-frame");
    const svg = this.querySelector<SVGSVGElement>(".table-sparkline");
    const cursor = this.querySelector<SVGLineElement>(".sparkline-cursor");
    const cpuPoint = this.querySelector<SVGCircleElement>(".sparkline-point.cpu");
    const memoryPoint = this.querySelector<SVGCircleElement>(".sparkline-point.memory");
    const tooltip = this.querySelector<HTMLElement>(".sparkline-tooltip");
    if (!frame || !svg || !cursor || !cpuPoint || !memoryPoint || !tooltip) {
      return;
    }

    const moveAt = (clientX: number) => {
      const rect = svg.getBoundingClientRect();
      if (rect.width <= 0) {
        return;
      }
      const ratio = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
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
    const move = (event: PointerEvent) => moveAt(event.clientX);
    const leave = () => {
      frame.classList.remove("active");
    };
    frame.addEventListener("pointermove", move);
    frame.addEventListener("pointerleave", leave);
    frame.addEventListener("focusin", () => {
      const rect = svg.getBoundingClientRect();
      moveAt(rect.left + rect.width);
    });
    frame.addEventListener("focusout", leave);
  }
}

function escapeHTML(value: unknown): string {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function seriesSamples(series: PrometheusRangeSeries[], name: string): SparklineSample[] {
  return series
    .filter((item) => item.name === name)
    .flatMap((item) => item.samples || [])
    .map((sample) => ({ timestamp: Date.parse(sample.timestamp), value: Number(sample.value) }))
    .filter((sample) => Number.isFinite(sample.timestamp) && Number.isFinite(sample.value))
    .sort((a, b) => a.timestamp - b.timestamp);
}

function sparklineDomain(...sampleSets: SparklineSample[][]): SparklineDomain {
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

function sparklineScale(samples: SparklineSample[], domain: SparklineDomain): number {
  const values = samples
    .filter((sample) => sample.timestamp >= domain.start && sample.timestamp <= domain.end)
    .map((sample) => sample.value);
  return Math.max(...values, 0);
}

function sparklinePoints(samples: SparklineSample[], domain: SparklineDomain, maxValue: number): string {
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

function sparklineY(value: number, maxValue: number, height: number, pad: number): number {
  if (value <= 0 || maxValue <= 0) {
    return height - pad;
  }
  const ratio = Math.max(0, Math.min(1, value / maxValue));
  return pad + (1 - ratio) * (height - pad * 2);
}

function nearestSample(samples: SparklineSample[], timestamp: number): SparklineSample | null {
  if (!samples.length) {
    return null;
  }
  let nearest = samples[0];
  for (const sample of samples.slice(1)) {
    if (Math.abs(sample.timestamp - timestamp) < Math.abs(nearest.timestamp - timestamp)) {
      nearest = sample;
    }
  }
  return nearest;
}

function updateSparklinePoint(point: SVGCircleElement, sample: SparklineSample | null, domain: SparklineDomain, maxValue: number): void {
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

function sparklineTooltipHTML(cpu: SparklineSample | null, memory: SparklineSample | null): string {
  const timestamp = cpu?.timestamp || memory?.timestamp || Date.now();
  return `
    <span>${escapeHTML(formatSparklineTime(timestamp))}</span>
    <strong class="cpu">CPU ${escapeHTML(formatCPU(cpu?.value))}</strong>
    <strong class="memory">MEM ${escapeHTML(formatMemory(memory?.value))}</strong>
  `;
}

function formatSparklineTime(timestamp: number): string {
  return new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(timestamp));
}

function formatCPU(value: number | undefined): string {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return "-";
  }
  if (value < 1) {
    return `${Math.round(value * 1000)}m`;
  }
  return `${value.toFixed(value < 10 ? 2 : 1)} cores`;
}

function formatMemory(value: number | undefined): string {
  if (typeof value !== "number" || !Number.isFinite(value)) {
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

export function registerPromqlSparkline() {
  if (!customElements.get("kubernetto-promql-sparkline")) {
    customElements.define("kubernetto-promql-sparkline", KubernettoPromqlSparkline);
  }
}
