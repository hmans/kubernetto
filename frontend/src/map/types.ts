export type MapCounts = {
  nodes: number;
  namespaces: number;
  workloads: number;
  pods: number;
  services: number;
  warnings: number;
};

export type ClusterMapData = {
  context: string;
  cluster: string;
  updatedAt: string;
  counts: MapCounts;
  truncated: boolean;
  nodes: MapNode[];
  namespaces: MapNamespace[];
  workloads: MapWorkload[];
  pods: MapPod[];
  services: MapService[];
  warnings: MapWarning[];
  clusterResources?: MapClusterResource[];
};

export type MapNode = {
  id: string;
  name: string;
  ready: boolean;
  podCount: number;
};

export type MapNamespace = {
  id: string;
  name: string;
  podCount: number;
  workloadCount: number;
  serviceCount: number;
  warningCount: number;
};

export type MapWorkload = {
  id: string;
  kind: string;
  namespace: string;
  name: string;
  ready: number;
  desired: number;
  statusKey: string;
  podIds: string[] | null;
};

export type MapPod = {
  id: string;
  namespace: string;
  name: string;
  node: string;
  phase: string;
  ready: boolean;
  statusKey: string;
  ownerKind: string;
  ownerName: string;
  ownerId: string;
};

export type MapService = {
  id: string;
  namespace: string;
  name: string;
  type: string;
  selector: string;
  targetPodIds: string[] | null;
  targetPodCount: number;
  targetNamespace: string;
};

export type MapWarning = {
  id: string;
  reason: string;
  message: string;
  namespace: string;
  involvedObject: string;
  targetId: string;
  count: number;
  age: string;
};

export type MapClusterResource = {
  id: string;
  category: string;
  kind: string;
  name: string;
  detail: string;
  statusKey: string;
};

export type MapItemType = "category" | "clusterResource" | "node" | "namespace" | "workload" | "pod" | "service" | "warning";

export type MapLayoutItem = {
  id: string;
  type: MapItemType;
  label: string;
  detail: string;
  statusKey: string;
  x: number;
  y: number;
  z: number;
  size: number;
  href: string;
  targetIds: string[];
  trafficTargetIds: string[];
  ownedIds: string[];
};
