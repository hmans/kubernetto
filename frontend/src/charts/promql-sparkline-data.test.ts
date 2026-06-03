import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  loadSparklineData,
  resetSparklineDataForTests,
  type PrometheusRangeData,
  type SparklineRequest,
} from "./promql-sparkline-data";

function request(patch: Partial<SparklineRequest> = {}): SparklineRequest {
  return {
    context: "test",
    namespace: "default",
    pod: "api",
    cpuQuery: "cpu_query",
    memoryQuery: "memory_query",
    windowSeconds: 3600,
    stepSeconds: 180,
    ...patch,
  };
}

function jsonResponse(data: PrometheusRangeData): Response {
  return {
    ok: true,
    json: async () => data,
  } as Response;
}

function firstRequestBody(fetchMock: ReturnType<typeof vi.fn>): {
  queries: Array<{ name: string; query: string }>;
  stepSeconds: number;
} {
  const calls = fetchMock.mock.calls as Array<[RequestInfo | URL, RequestInit | undefined]>;
  const init = calls[0]?.[1];

  expect(init).toBeDefined();
  return JSON.parse(String(init?.body));
}

describe("loadSparklineData", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    resetSparklineDataForTests();
  });

  afterEach(() => {
    resetSparklineDataForTests();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("batches pod requests into shared CPU and memory PromQL queries", async () => {
    const fetchMock = vi.fn(async () => jsonResponse({
      available: true,
      series: [
        { name: "cpu", metric: { namespace: "default", pod: "api" }, samples: [{ timestamp: "2026-06-03T10:00:00Z", value: 1 }] },
        { name: "memory", metric: { namespace: "default", pod: "api" }, samples: [{ timestamp: "2026-06-03T10:00:00Z", value: 2 }] },
        { name: "cpu", metric: { namespace: "default", pod: "worker" }, samples: [{ timestamp: "2026-06-03T10:00:00Z", value: 3 }] },
        { name: "memory", metric: { namespace: "default", pod: "worker" }, samples: [{ timestamp: "2026-06-03T10:00:00Z", value: 4 }] },
      ],
    }));
    vi.stubGlobal("fetch", fetchMock);

    const api = loadSparklineData(request({ pod: "api" }), new AbortController().signal);
    const worker = loadSparklineData(request({ pod: "worker" }), new AbortController().signal);

    await vi.advanceTimersByTimeAsync(30);

    await expect(api).resolves.toMatchObject({
      series: [
        { name: "cpu", metric: { pod: "api" } },
        { name: "memory", metric: { pod: "api" } },
      ],
    });
    await expect(worker).resolves.toMatchObject({
      series: [
        { name: "cpu", metric: { pod: "worker" } },
        { name: "memory", metric: { pod: "worker" } },
      ],
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const body = firstRequestBody(fetchMock);
    expect(body.queries).toHaveLength(2);
    expect(body.queries.map((query) => query.name)).toEqual(["cpu", "memory"]);
    expect(body.queries[0].query).toContain('pod=~"api|worker"');
    expect(body.queries[0].query).not.toContain("0:cpu");
    expect(body.stepSeconds).toBe(180);
  });

  it("keeps arbitrary PromQL requests on the generic query batching path", async () => {
    const fetchMock = vi.fn(async () => jsonResponse({
      available: true,
      series: [
        { name: "0:cpu", samples: [{ timestamp: "2026-06-03T10:00:00Z", value: 1 }] },
        { name: "0:memory", samples: [{ timestamp: "2026-06-03T10:00:00Z", value: 2 }] },
        { name: "1:cpu", samples: [{ timestamp: "2026-06-03T10:00:00Z", value: 3 }] },
        { name: "1:memory", samples: [{ timestamp: "2026-06-03T10:00:00Z", value: 4 }] },
      ],
    }));
    vi.stubGlobal("fetch", fetchMock);

    const first = loadSparklineData(request({ namespace: "", pod: "", cpuQuery: "up", memoryQuery: "process_resident_memory_bytes" }), new AbortController().signal);
    const second = loadSparklineData(request({ namespace: "", pod: "", cpuQuery: "go_threads", memoryQuery: "go_memstats_heap_alloc_bytes" }), new AbortController().signal);

    await vi.advanceTimersByTimeAsync(30);

    await expect(first).resolves.toMatchObject({
      series: [{ name: "cpu" }, { name: "memory" }],
    });
    await expect(second).resolves.toMatchObject({
      series: [{ name: "cpu" }, { name: "memory" }],
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const body = firstRequestBody(fetchMock);
    expect(body.queries.map((query) => query.name)).toEqual(["0:cpu", "0:memory", "1:cpu", "1:memory"]);
    expect(body.queries.map((query) => query.query)).toEqual([
      "up",
      "process_resident_memory_bytes",
      "go_threads",
      "go_memstats_heap_alloc_bytes",
    ]);
  });
});
