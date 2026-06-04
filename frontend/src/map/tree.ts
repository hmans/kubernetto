import type { MapItemType, MapLayoutItem } from "./types";

export type MapTopologyTreeRow = {
  id: string;
  depth: number;
  item: MapLayoutItem;
};

export const defaultTopologyTreeLimit = 240;

export function buildTopologyTree(layout: MapLayoutItem[], limit = defaultTopologyTreeLimit, rootId = ""): MapTopologyTreeRow[] {
  const itemsById = new Map(layout.map((item) => [item.id, item]));
  const warningIdsByTarget = new Map<string, string[]>();
  for (const item of layout) {
    if (item.type !== "warning") {
      continue;
    }
    for (const targetId of item.targetIds) {
      const existing = warningIdsByTarget.get(targetId) || [];
      existing.push(item.id);
      warningIdsByTarget.set(targetId, existing);
    }
  }

  const rows: MapTopologyTreeRow[] = [];
  const seen = new Set<string>();
  const root = rootId ? itemsById.get(rootId) : undefined;

  const append = (item: MapLayoutItem, depth: number) => {
    if (seen.has(item.id) || rows.length >= limit) {
      return;
    }
    seen.add(item.id);
    rows.push({ id: item.id, depth, item });
    for (const child of treeChildren(item, itemsById, warningIdsByTarget)) {
      append(child, depth + 1);
      if (rows.length >= limit) {
        return;
      }
    }
  };

  if (root) {
    for (const child of treeChildren(root, itemsById, warningIdsByTarget)) {
      append(child, 0);
    }
    return rows;
  }

  const topLevel = layout
    .filter((item) => item.type === "namespace" || item.type === "node" || (item.type === "category" && item.label !== "Nodes"))
    .sort(compareTreeItems);

  for (const item of topLevel) {
    if (rows.length >= limit) {
      break;
    }
    seen.add(item.id);
    rows.push({ id: item.id, depth: 0, item });
  }

  return rows;
}

function treeChildren(item: MapLayoutItem, itemsById: Map<string, MapLayoutItem>, warningIdsByTarget: Map<string, string[]>) {
  const childIds = new Set([...item.ownedIds, ...item.trafficTargetIds, ...(warningIdsByTarget.get(item.id) || [])]);
  return [...childIds]
    .map((id) => itemsById.get(id))
    .filter((child): child is MapLayoutItem => Boolean(child))
    .sort(compareTreeItems);
}

function compareTreeItems(left: MapLayoutItem, right: MapLayoutItem) {
  const typeDelta = treeTypeRank(left.type) - treeTypeRank(right.type);
  if (typeDelta !== 0) {
    return typeDelta;
  }
  return left.label.localeCompare(right.label);
}

function treeTypeRank(type: MapItemType) {
  switch (type) {
    case "namespace":
      return 0;
    case "category":
      return 1;
    case "workload":
      return 2;
    case "service":
      return 3;
    case "warning":
      return 4;
    case "pod":
      return 5;
    case "node":
      return 6;
    case "clusterResource":
    default:
      return 7;
  }
}
