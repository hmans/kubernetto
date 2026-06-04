import { describe, expect, it } from "vitest";

import { buildMapLayout } from "./layout";
import { buildTopologyTree } from "./tree";
import type { ClusterMapData } from "./types";

describe("cluster map topology tree", () => {
  it("groups owned resources and warning events without duplicating pods", () => {
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
    const rows = buildTopologyTree(layout);
    const rowFor = (id: string) => rows.find((row) => row.id === id);

    expect(rowFor("namespace:prod")?.depth).toBe(0);
    expect(rowFor("node:node-1")?.depth).toBe(0);
    expect(rowFor("workload:Deployment:prod:api")).toBeUndefined();
    expect(rowFor("category:Nodes")).toBeUndefined();

    const namespaceRows = buildTopologyTree(layout, 240, "namespace:prod");
    const namespaceRowFor = (id: string) => namespaceRows.find((row) => row.id === id);
    expect(namespaceRowFor("workload:Deployment:prod:api")?.depth).toBe(0);
    expect(namespaceRowFor("pod:prod:api-123")?.depth).toBe(1);
    expect(namespaceRowFor("event:prod:api-warning")?.depth).toBe(2);
    expect(namespaceRowFor("service:prod:api")?.depth).toBe(0);
    expect(namespaceRows.filter((row) => row.id === "pod:prod:api-123")).toHaveLength(1);

    const serviceRows = buildTopologyTree(layout, 240, "service:prod:api");
    expect(serviceRows.map((row) => row.id)).toEqual(["pod:prod:api-123", "event:prod:api-warning"]);
  });
});
