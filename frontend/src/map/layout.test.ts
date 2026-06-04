import { describe, expect, it } from "vitest";

import { buildMapLayout } from "./layout";
import type { ClusterMapData } from "./types";

describe("cluster map layout", () => {
  it("places topology objects and creates detail links", () => {
    const data: ClusterMapData = {
      context: "prod",
      cluster: "prod",
      updatedAt: new Date().toISOString(),
      truncated: false,
      counts: { nodes: 1, namespaces: 1, workloads: 1, pods: 1, services: 1, warnings: 1 },
      nodes: [{ id: "node:node-1", name: "node-1", ready: true, podCount: 1 }],
      namespaces: [{ id: "namespace:prod", name: "prod", podCount: 1, workloadCount: 1, serviceCount: 1, warningCount: 1 }],
      workloads: [{
        id: "workload:Deployment:prod:api",
        kind: "deployments",
        namespace: "prod",
        name: "api",
        ready: 1,
        desired: 1,
        statusKey: "good",
        podIds: ["pod:prod:api-123"],
      }],
      pods: [{
        id: "pod:prod:api-123",
        namespace: "prod",
        name: "api-123",
        node: "node-1",
        phase: "Running",
        ready: true,
        statusKey: "good",
        ownerKind: "Deployment",
        ownerName: "api",
        ownerId: "workload:Deployment:prod:api",
      }],
      services: [{
        id: "service:prod:api",
        namespace: "prod",
        name: "api",
        type: "ClusterIP",
        selector: "app=api",
        targetPodIds: ["pod:prod:api-123"],
        targetPodCount: 1,
        targetNamespace: "prod",
      }],
      warnings: [{
        id: "event:prod:api-warning",
        reason: "BackOff",
        message: "retrying",
        namespace: "prod",
        involvedObject: "Pod/api-123",
        targetId: "pod:prod:api-123",
        count: 3,
        age: "1m ago",
      }],
    };

    const layout = buildMapLayout(data);

    expect(layout.map((item) => item.type)).toEqual(expect.arrayContaining(["node", "namespace", "workload", "pod", "service", "warning"]));
    expect(layout.find((item) => item.id === "pod:prod:api-123")?.href).toBe("/?resource=pods&namespace=prod&selectedNamespace=prod&selectedName=api-123");
    expect(layout.find((item) => item.id === "namespace:prod")?.ownedIds).toEqual([
      "pod:prod:api-123",
      "service:prod:api",
      "workload:Deployment:prod:api",
    ]);
    expect(layout.find((item) => item.id === "node:node-1")?.ownedIds).toEqual(["pod:prod:api-123"]);
    expect(layout.find((item) => item.id === "workload:Deployment:prod:api")?.ownedIds).toEqual(["pod:prod:api-123"]);
    expect(layout.find((item) => item.id === "service:prod:api")?.targetIds).toEqual(["pod:prod:api-123"]);
    expect(layout.find((item) => item.id === "service:prod:api")?.trafficTargetIds).toEqual(["pod:prod:api-123"]);
    expect(layout.find((item) => item.id === "workload:Deployment:prod:api")?.trafficTargetIds).toEqual([]);
    expect(layout.find((item) => item.id === "event:prod:api-warning")?.targetIds).toEqual(["pod:prod:api-123"]);
    expect(layout.find((item) => item.id === "event:prod:api-warning")?.trafficTargetIds).toEqual([]);

    const namespace = layout.find((item) => item.id === "namespace:prod");
    expect(namespace).toBeTruthy();
    for (const id of [...(namespace?.ownedIds || []), "event:prod:api-warning"]) {
      const child = layout.find((item) => item.id === id);
      expect(child).toBeTruthy();
      expect(Math.hypot((child?.x || 0) - (namespace?.x || 0), (child?.z || 0) - (namespace?.z || 0)) + (child?.size || 0)).toBeLessThanOrEqual(
        (namespace?.size || 0) + 0.001,
      );
    }
  });
});
