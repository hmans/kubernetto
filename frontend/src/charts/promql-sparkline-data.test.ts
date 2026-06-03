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
  pods: Array<{ namespace: string; pod: string }>;
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

  it("batches pod requests into shared pod usage requests", async () => {
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
    const calls = fetchMock.mock.calls as unknown as Array<[RequestInfo | URL, RequestInit | undefined]>;
    expect(calls[0]?.[0]).toBe("/api/prometheus/pod-usage-range");
    const body = firstRequestBody(fetchMock);
    expect(body.pods).toEqual([
      { namespace: "default", pod: "api" },
      { namespace: "default", pod: "worker" },
    ]);
    expect(body.stepSeconds).toBe(180);
  });
});
