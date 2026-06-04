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

  it("scales namespace territory by contents and keeps sparse resources near the center", () => {
    const data: ClusterMapData = {
      context: "prod",
      cluster: "prod",
      updatedAt: new Date().toISOString(),
      truncated: false,
      counts: { nodes: 0, namespaces: 2, workloads: 7, pods: 18, services: 4, warnings: 0 },
      nodes: [],
      namespaces: [
        { id: "namespace:sparse", name: "sparse", podCount: 1, workloadCount: 1, serviceCount: 0, warningCount: 0 },
        { id: "namespace:dense", name: "dense", podCount: 17, workloadCount: 6, serviceCount: 4, warningCount: 0 },
      ],
      workloads: [
        workload("sparse", "one", 1),
        ...Array.from({ length: 6 }, (_, index) => workload("dense", `app-${index}`, 3)),
      ],
      pods: [
        pod("sparse", "one-0", "workload:Deployment:sparse:one"),
        ...Array.from({ length: 18 }, (_, index) => pod("dense", `app-${index}`, `workload:Deployment:dense:app-${index % 6}`)),
      ],
      services: Array.from({ length: 4 }, (_, index) => ({
        id: `service:dense:svc-${index}`,
        namespace: "dense",
        name: `svc-${index}`,
        type: "ClusterIP",
        selector: `app=${index}`,
        targetPodIds: [`pod:dense:app-${index}`],
        targetPodCount: 1,
        targetNamespace: "dense",
      })),
      warnings: [],
    };

    const layout = buildMapLayout(data);
    const sparse = layout.find((item) => item.id === "namespace:sparse");
    const dense = layout.find((item) => item.id === "namespace:dense");
    const sparseWorkload = layout.find((item) => item.id === "workload:Deployment:sparse:one");
    const sparsePod = layout.find((item) => item.id === "pod:sparse:one-0");

    expect(sparse).toBeTruthy();
    expect(dense).toBeTruthy();
    expect((dense?.size || 0) - (sparse?.size || 0)).toBeGreaterThan(2);
    expect(distance(sparse, sparseWorkload)).toBeLessThan((sparse?.size || 0) * 0.18);
    expect(distance(sparse, sparsePod)).toBeLessThan((sparse?.size || 0) * 0.55);
  });
});

function workload(namespace: string, name: string, desired: number) {
  return {
    id: `workload:Deployment:${namespace}:${name}`,
    kind: "deployments",
    namespace,
    name,
    ready: desired,
    desired,
    statusKey: "good",
    podIds: Array.from({ length: desired }, (_, index) => `pod:${namespace}:${name}-${index}`),
  };
}

function pod(namespace: string, name: string, ownerId: string) {
  return {
    id: `pod:${namespace}:${name}`,
    namespace,
    name,
    node: "",
    phase: "Running",
    ready: true,
    statusKey: "good",
    ownerKind: "Deployment",
    ownerName: ownerId.split(":").pop() || "",
    ownerId,
  };
}

function distance(left?: { x: number; z: number }, right?: { x: number; z: number }) {
  if (!left || !right) {
    return Number.POSITIVE_INFINITY;
  }
  return Math.hypot(left.x - right.x, left.z - right.z);
}
