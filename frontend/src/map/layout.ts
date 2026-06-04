import type { ClusterMapData, MapClusterResource, MapLayoutItem, MapNamespace, MapPod, MapService, MapWarning, MapWorkload } from "./types";

const islandGapX = 21;
const islandGapZ = 16;
const categoryOrbit = 4.6;
const itemGap = 1.45;

export function buildMapLayout(data: ClusterMapData): MapLayoutItem[] {
  const items: MapLayoutItem[] = [];
  const namespacePositions = new Map<string, NamespaceFootprint>();
  const podPositions = new Map<string, { x: number; z: number }>();
  const workloadPositions = new Map<string, { x: number; z: number }>();
  const namespaceOwnedIds = new Map<string, Set<string>>();
  const nodeOwnedIds = new Map<string, Set<string>>();
  const namespaces = orderedNamespaces(data);
  const categoryGroups = clusterCategoryGroups(data);
  const workloadGroups = groupByNamespace(data.workloads);
  const podGroups = groupByNamespace(data.pods);
  const serviceGroups = groupByNamespace(data.services);
  const warningGroups = groupWarningsByNamespace(data.warnings);
  const topLevelCount = namespaces.length + categoryGroups.length;
  let topLevelIndex = 0;

  namespaces.forEach((namespace) => {
    const center = islandCenter(topLevelIndex, topLevelCount);
    const footprint = namespaceFootprint(namespace.name, workloadGroups, podGroups, serviceGroups, warningGroups);
    topLevelIndex += 1;
    namespacePositions.set(namespace.name, { ...footprint, x: center.x, z: center.z });
    items.push({
      id: namespace.id,
      type: "namespace",
      label: namespace.name,
      detail: `${namespace.podCount} pods · ${namespace.workloadCount} workloads · ${namespace.serviceCount} services`,
      statusKey: namespace.warningCount > 0 ? "warn" : "neutral",
      x: center.x,
      y: 0,
      z: center.z,
      size: footprint.radius,
      href: resourceHref("namespaces", "", namespace.name),
      targetIds: [],
      trafficTargetIds: [],
      ownedIds: [],
    });
  });

  const categoryPositions = new Map<string, { x: number; z: number; height: number }>();
  for (const category of categoryGroups) {
    const center = islandCenter(topLevelIndex, topLevelCount);
    const radius = categoryRadius(category);
    topLevelIndex += 1;
    categoryPositions.set(category.name, { x: center.x, z: center.z, height: radius * 2 });
    items.push({
      id: category.id,
      type: "category",
      label: category.name,
      detail: `${category.children.length} cluster-scoped resources`,
      statusKey: "neutral",
      x: center.x,
      y: 0.2,
      z: center.z,
      size: radius,
      href: category.href,
      targetIds: [],
      trafficTargetIds: [],
      ownedIds: category.children.map((child) => child.id).sort(),
    });
    if (category.name !== "Nodes") {
      category.children.forEach((resource, index) => {
        const point = resourceOrbitPoint(center, index, category.children.length, categoryOrbit);
        items.push(clusterResourceItem(resource, point.x, point.z));
      });
    }
  }

  data.nodes.forEach((node, index) => {
    const category = categoryPositions.get("Nodes") || fallbackNamespacePosition(namespacePositions);
    const point = resourceOrbitPoint(category, index, data.nodes.length, categoryOrbit);
    items.push({
      id: node.id,
      type: "node",
      label: node.name,
      detail: `${node.podCount} pods`,
      statusKey: node.ready ? "good" : "danger",
      x: point.x,
      y: 2,
      z: point.z,
      size: 1.9 + Math.min(4, node.podCount * 0.28),
      href: resourceHref("nodes", "", node.name),
      targetIds: [],
      trafficTargetIds: [],
      ownedIds: [],
    });
  });

  for (const [namespace, workloads] of workloadGroups) {
    const origin = namespacePositions.get(namespace) || fallbackNamespacePosition(namespacePositions);
    workloads.forEach((workload, index) => {
      const point = resourceOrbitPoint(origin, index, workloads.length, origin.workloadOrbit, namespaceAngleSeed(namespace));
      workloadPositions.set(workload.id, point);
      addOwned(namespaceOwnedIds, namespace, workload.id);
      items.push(workloadItem(workload, point.x, point.z));
    });
  }

  for (const [namespace, pods] of podGroups) {
    const origin = namespacePositions.get(namespace) || fallbackNamespacePosition(namespacePositions);
    const podsByOwner = groupPodsByOwner(pods);
    const usedSlots = new Map<string, number>();
    pods.forEach((pod, index) => {
      const ownerPoint = workloadPositions.get(pod.ownerId);
      const ownerPods = podsByOwner.get(pod.ownerId || "") || [];
      const ownerIndex = usedSlots.get(pod.ownerId || "") || 0;
      usedSlots.set(pod.ownerId || "", ownerIndex + 1);
      const point = ownerPoint
        ? branchPoint(origin, ownerPoint, ownerIndex, ownerPods.length, origin.podBranch)
        : resourceOrbitPoint(origin, index, pods.length, origin.podOrbit, namespaceAngleSeed(namespace) + 0.42);
      podPositions.set(pod.id, point);
      addOwned(namespaceOwnedIds, namespace, pod.id);
      if (pod.node) {
        addOwned(nodeOwnedIds, pod.node, pod.id);
      }
      items.push(podItem(pod, point.x, point.z));
    });
  }

  for (const [namespace, services] of serviceGroups) {
    const origin = namespacePositions.get(namespace) || fallbackNamespacePosition(namespacePositions);
    services.forEach((service, index) => {
      const targetPoint = averageTargetPoint(service.targetPodIds || [], podPositions);
      const point = targetPoint
        ? outerPoint(origin, targetPoint, index, services.length, origin.serviceOrbit)
        : resourceOrbitPoint(origin, index, services.length, origin.serviceOrbit, namespaceAngleSeed(namespace) + 0.9);
      addOwned(namespaceOwnedIds, namespace, service.id);
      items.push(serviceItem(service, point.x, point.z));
    });
  }

  data.warnings.forEach((warning, index) => {
    const target = items.find((item) => item.id === warning.targetId);
    const namespaceOrigin = namespacePositions.get(warning.namespace) || fallbackNamespacePosition(namespacePositions);
    const point = target
      ? outerPoint(namespaceOrigin, target, index, data.warnings.length, namespaceOrigin.warningOrbit)
      : resourceOrbitPoint(namespaceOrigin, index, data.warnings.length, namespaceOrigin.warningOrbit, namespaceAngleSeed(warning.namespace) + 1.3);
    items.push(warningItem(warning, point.x, point.z, index));
  });

  for (const item of items) {
    if (item.type === "namespace") {
      item.ownedIds = [...(namespaceOwnedIds.get(item.label) || [])].sort();
    }
    if (item.type === "node") {
      item.ownedIds = [...(nodeOwnedIds.get(item.label) || [])].sort();
    }
  }
  fitNamespaceRadii(items, warningGroups);

  return items;
}

type ClusterCategory = {
  id: string;
  name: string;
  href: string;
  children: MapClusterResource[];
};

type NamespaceFootprint = {
  x: number;
  z: number;
  radius: number;
  workloadOrbit: number;
  podOrbit: number;
  podBranch: number;
  serviceOrbit: number;
  warningOrbit: number;
};

function workloadItem(workload: MapWorkload, x: number, z: number): MapLayoutItem {
  return {
    id: workload.id,
    type: "workload",
    label: workload.name,
    detail: `${workload.kind} · ${workload.ready}/${workload.desired} ready`,
    statusKey: workload.statusKey || "neutral",
    x,
    y: 1.25,
    z,
    size: 1.25 + Math.min(2.6, Math.max(0, workload.desired) * 0.22),
    href: resourceHref(workload.kind, workload.namespace, workload.name),
    targetIds: workload.podIds || [],
    trafficTargetIds: [],
    ownedIds: workload.podIds || [],
  };
}

function podItem(pod: MapPod, x: number, z: number): MapLayoutItem {
  return {
    id: pod.id,
    type: "pod",
    label: pod.name,
    detail: `${pod.phase || "Unknown"} · ${pod.namespace}${pod.node ? ` · ${pod.node}` : ""}`,
    statusKey: pod.statusKey || (pod.ready ? "good" : "warn"),
    x,
    y: 1.05,
    z,
    size: 0.56,
    href: resourceHref("pods", pod.namespace, pod.name),
    targetIds: pod.ownerId ? [pod.ownerId] : [],
    trafficTargetIds: [],
    ownedIds: [],
  };
}

function serviceItem(service: MapService, x: number, z: number): MapLayoutItem {
  return {
    id: service.id,
    type: "service",
    label: service.name,
    detail: `${service.type || "Service"} · ${service.targetPodCount} targets${service.selector ? ` · ${service.selector}` : ""}`,
    statusKey: service.targetPodCount > 0 ? "good" : "neutral",
    x,
    y: 1.55,
    z,
    size: 0.86,
    href: resourceHref("services", service.namespace, service.name),
    targetIds: service.targetPodIds || [],
    trafficTargetIds: service.targetPodIds || [],
    ownedIds: [],
  };
}

function clusterResourceItem(resource: MapClusterResource, x: number, z: number): MapLayoutItem {
  return {
    id: resource.id,
    type: "clusterResource",
    label: resource.name,
    detail: `${resource.kind}${resource.detail ? ` · ${resource.detail}` : ""}`,
    statusKey: resource.statusKey || "neutral",
    x,
    y: 1.05,
    z,
    size: 0.86,
    href: resourceHref(resource.kind, "", resource.name),
    targetIds: [],
    trafficTargetIds: [],
    ownedIds: [],
  };
}

function warningItem(warning: MapWarning, x: number, z: number, index: number): MapLayoutItem {
  return {
    id: warning.id,
    type: "warning",
    label: warning.reason || "Warning",
    detail: `${warning.involvedObject || warning.namespace || "cluster"} · ${warning.age}`,
    statusKey: "danger",
    x: x + ((index % 3) - 1) * 0.55,
    y: 2.35,
    z: z + (Math.floor(index / 3) % 3 - 1) * 0.55,
    size: 0.5 + Math.min(1.3, warning.count * 0.08),
    href: eventHref(warning),
    targetIds: warning.targetId ? [warning.targetId] : [],
    trafficTargetIds: [],
    ownedIds: [],
  };
}

function addOwned(groups: Map<string, Set<string>>, key: string, id: string) {
  const group = groups.get(key) || new Set<string>();
  group.add(id);
  groups.set(key, group);
}

function orderedNamespaces(data: ClusterMapData): MapNamespace[] {
  const byName = new Map(data.namespaces.map((namespace) => [namespace.name, namespace]));
  for (const item of [...data.workloads, ...data.pods, ...data.services]) {
    if (item.namespace && !byName.has(item.namespace)) {
      byName.set(item.namespace, {
        id: `namespace:${item.namespace}`,
        name: item.namespace,
        podCount: 0,
        workloadCount: 0,
        serviceCount: 0,
        warningCount: 0,
      });
    }
  }
  for (const warning of data.warnings) {
    if (warning.namespace && !byName.has(warning.namespace)) {
      byName.set(warning.namespace, {
        id: `namespace:${warning.namespace}`,
        name: warning.namespace,
        podCount: 0,
        workloadCount: 0,
        serviceCount: 0,
        warningCount: 1,
      });
    }
  }
  return [...byName.values()].sort((left, right) => left.name.localeCompare(right.name));
}

function clusterCategoryGroups(data: ClusterMapData): ClusterCategory[] {
  const groups = new Map<string, MapClusterResource[]>();
  groups.set("Nodes", data.nodes.map((node) => ({
    id: node.id,
    category: "Nodes",
    kind: "nodes",
    name: node.name,
    detail: `${node.podCount} pods`,
    statusKey: node.ready ? "good" : "danger",
  })));
  for (const resource of data.clusterResources || []) {
    groups.set(resource.category, [...(groups.get(resource.category) || []), resource]);
  }
  return [...groups.entries()]
    .filter(([, children]) => children.length > 0)
    .map(([name, children]) => ({
      id: `category:${name}`,
      name,
      href: categoryHref(name, children),
      children,
    }))
    .sort((left, right) => categoryRank(left.name) - categoryRank(right.name) || left.name.localeCompare(right.name));
}

function categoryHref(name: string, children: MapClusterResource[]) {
  const first = children[0];
  if (name === "Nodes") {
    return resourceHref("nodes", "", "");
  }
  return first ? resourceHref(first.kind, "", "") : "/";
}

function categoryRank(name: string) {
  return ["Nodes", "Storage", "RBAC", "Scheduling", "Webhooks"].indexOf(name) === -1
    ? 99
    : ["Nodes", "Storage", "RBAC", "Scheduling", "Webhooks"].indexOf(name);
}

function namespaceFootprint(
  namespace: string,
  workloadGroups: Map<string, MapWorkload[]>,
  podGroups: Map<string, MapPod[]>,
  serviceGroups: Map<string, MapService[]>,
  warningGroups: Map<string, MapWarning[]>,
) {
  const workloads = workloadGroups.get(namespace)?.length || 0;
  const pods = podGroups.get(namespace)?.length || 0;
  const services = serviceGroups.get(namespace)?.length || 0;
  const warnings = warningGroups.get(namespace)?.length || 0;
  const visible = workloads + pods + services + warnings;
  const weight = Math.max(1, workloads * 1.45 + pods * 0.42 + services * 1.05 + warnings * 1.35);
  const radius = clamp(4.9 + Math.sqrt(weight) * 1.26, 5.25, 13.6);
  const compactness = visible <= 2 ? 0.22 : visible <= 5 ? 0.32 : visible <= 12 ? 0.43 : 0.52;
  const workloadOrbit = Math.max(0, radius * compactness);
  const podBranch = clamp(radius * 0.2, 1.25, 2.5);
  return {
    x: 0,
    z: 0,
    radius,
    workloadOrbit,
    podOrbit: Math.min(radius * 0.72, workloadOrbit + podBranch),
    podBranch,
    serviceOrbit: radius * (visible <= 4 ? 0.58 : 0.72),
    warningOrbit: radius * 0.84,
  };
}

function fitNamespaceRadii(items: MapLayoutItem[], warningGroups: Map<string, MapWarning[]>) {
  const itemsById = new Map(items.map((item) => [item.id, item]));
  for (const namespace of items.filter((item) => item.type === "namespace")) {
    const childIds = new Set(namespace.ownedIds);
    for (const warning of warningGroups.get(namespace.label) || []) {
      childIds.add(warning.id);
    }
    let requiredRadius = namespace.size;
    for (const childId of childIds) {
      const child = itemsById.get(childId);
      if (!child) {
        continue;
      }
      requiredRadius = Math.max(requiredRadius, Math.hypot(child.x - namespace.x, child.z - namespace.z) + child.size + 0.85);
    }
    namespace.size = requiredRadius;
  }
}

function categoryRadius(category: ClusterCategory) {
  return Math.max(5.4, Math.min(9.6, 4 + Math.sqrt(category.children.length) * 0.72));
}

function groupByNamespace<T extends { namespace: string }>(values: T[]): Map<string, T[]> {
  const groups = new Map<string, T[]>();
  for (const value of values) {
    const namespace = value.namespace || "cluster";
    groups.set(namespace, [...(groups.get(namespace) || []), value]);
  }
  return groups;
}

function groupWarningsByNamespace(values: MapWarning[]): Map<string, MapWarning[]> {
  const groups = new Map<string, MapWarning[]>();
  for (const value of values) {
    const namespace = value.namespace || "cluster";
    groups.set(namespace, [...(groups.get(namespace) || []), value]);
  }
  return groups;
}

function groupPodsByOwner(pods: MapPod[]): Map<string, MapPod[]> {
  const groups = new Map<string, MapPod[]>();
  for (const pod of pods) {
    const ownerID = pod.ownerId || "";
    groups.set(ownerID, [...(groups.get(ownerID) || []), pod]);
  }
  return groups;
}

function islandCenter(index: number, total: number) {
  const columns = Math.max(1, Math.ceil(Math.sqrt(total)));
  const rows = Math.max(1, Math.ceil(total / columns));
  const row = Math.floor(index / columns);
  const column = index % columns;
  const rowLength = row === rows - 1 ? total - row * columns || columns : columns;
  const offsetColumn = column + (columns - rowLength) / 2;
  return {
    x: (offsetColumn - (columns - 1) / 2) * islandGapX + (row % 2) * 3.2,
    z: (row - (rows - 1) / 2) * islandGapZ,
  };
}

function resourceOrbitPoint(center: { x: number; z: number }, index: number, total: number, radius: number, seed = -Math.PI / 2) {
  if (total <= 1) {
    return {
      x: center.x,
      z: center.z,
    };
  }
  const turns = total > 18 ? 1.55 : 1;
  const angle = seed + (index / total) * Math.PI * 2 * turns;
  const ring = radius + Math.floor(index / Math.max(1, Math.ceil(total / turns))) * 1.8;
  return {
    x: center.x + Math.cos(angle) * ring,
    z: center.z + Math.sin(angle) * ring,
  };
}

function branchPoint(
  center: { x: number; z: number },
  anchor: { x: number; z: number },
  index: number,
  total: number,
  distance: number,
) {
  const directionX = anchor.x - center.x;
  const directionZ = anchor.z - center.z;
  const length = Math.hypot(directionX, directionZ);
  if (length < 0.001) {
    const angle = -Math.PI / 2 + (index / Math.max(1, total)) * Math.PI * 2;
    const ring = total <= 1 ? distance * 0.62 : distance;
    return {
      x: anchor.x + Math.cos(angle) * ring,
      z: anchor.z + Math.sin(angle) * ring,
    };
  }
  const normalX = directionX / length;
  const normalZ = directionZ / length;
  const tangentX = -normalZ;
  const tangentZ = normalX;
  const spread = siblingOffset(index, total, itemGap);
  return {
    x: anchor.x + normalX * distance + tangentX * spread,
    z: anchor.z + normalZ * distance + tangentZ * spread,
  };
}

function outerPoint(
  center: { x: number; z: number },
  target: { x: number; z: number },
  index: number,
  total: number,
  radius: number,
) {
  const angle = Math.atan2(target.z - center.z, target.x - center.x);
  const spread = siblingOffset(index, total, 0.34);
  return {
    x: center.x + Math.cos(angle + spread) * radius,
    z: center.z + Math.sin(angle + spread) * radius,
  };
}

function siblingOffset(index: number, total: number, gap: number) {
  return (index - Math.max(0, total - 1) / 2) * gap;
}

function fallbackNamespacePosition(positions: Map<string, NamespaceFootprint>): NamespaceFootprint {
  return positions.values().next().value || {
    x: 0,
    z: 0,
    radius: 6,
    workloadOrbit: 2,
    podOrbit: 3.4,
    podBranch: 1.4,
    serviceOrbit: 4.1,
    warningOrbit: 5,
  };
}

function averageTargetPoint(targetIds: string[], positions: Map<string, { x: number; z: number }>) {
  let x = 0;
  let z = 0;
  let count = 0;
  for (const id of targetIds) {
    const position = positions.get(id);
    if (!position) {
      continue;
    }
    x += position.x;
    z += position.z;
    count += 1;
  }
  if (!count) {
    return null;
  }
  return { x: x / count, z: z / count };
}

function namespaceAngleSeed(namespace: string) {
  let hash = 0;
  for (let index = 0; index < namespace.length; index += 1) {
    hash = (hash * 31 + namespace.charCodeAt(index)) >>> 0;
  }
  return -Math.PI / 2 + ((hash % 360) / 360) * Math.PI * 0.36;
}

function clamp(value: number, min: number, max: number) {
  return Math.max(min, Math.min(max, value));
}

function resourceHref(resource: string, namespace: string, name: string): string {
  const url = new URL("/", window.location.origin);
  url.searchParams.set("resource", resource);
  if (namespace) {
    url.searchParams.set("namespace", namespace);
    url.searchParams.set("selectedNamespace", namespace);
  }
  if (name) {
    url.searchParams.set("selectedName", name);
  }
  return url.pathname + url.search;
}

function eventHref(warning: MapWarning): string {
  const [, namespace = "", name = ""] = warning.id.split(":");
  return resourceHref("events", namespace, name);
}
