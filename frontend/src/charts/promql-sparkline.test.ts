import { describe, expect, it } from "vitest";
import { KubernettoPromqlSparkline, registerPromqlSparkline } from "./promql-sparkline";

describe("KubernettoPromqlSparkline", () => {
  it("registers the custom element", () => {
    registerPromqlSparkline();

    expect(customElements.get("kubernetto-promql-sparkline")).toBe(KubernettoPromqlSparkline);
  });

  it("renders CPU and memory timeline series", () => {
    const element = new KubernettoPromqlSparkline();

    element.renderSparkline([
      {
        name: "cpu",
        samples: [
          { timestamp: "2026-06-03T10:00:00Z", value: 0.1 },
          { timestamp: "2026-06-03T10:03:00Z", value: 0.2 },
        ],
      },
      {
        name: "memory",
        samples: [
          { timestamp: "2026-06-03T10:00:00Z", value: 1024 },
          { timestamp: "2026-06-03T10:03:00Z", value: 2048 },
        ],
      },
    ]);

    expect(element.querySelector("svg.table-sparkline")).not.toBeNull();
    expect(element.querySelector(".timeline-line.cpu")?.getAttribute("points")).toContain("120.0");
    expect(element.querySelector(".timeline-line.memory")?.getAttribute("points")).toContain("120.0");
    expect(element.querySelector(".sparkline-tooltip")).not.toBeNull();
  });

  it("renders an empty state when there are no usable samples", () => {
    const element = new KubernettoPromqlSparkline();

    element.renderSparkline([]);

    expect(element.querySelector(".sparkline-empty")?.getAttribute("title")).toBe("No Prometheus samples");
    expect(element.querySelector("svg.table-sparkline")).toBeNull();
  });
});
