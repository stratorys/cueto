// Model-level diff of two diagram versions. Unlike a text diff, this keys nodes
// and edges by their stable id and reports semantic changes: added/removed
// nodes, per-field node changes, added/removed edges, and edge rewires (same
// edge id, changed endpoints).
//
// Known limitation (v1): an edge dropped and re-added with a NEW id reads as a
// remove + add, not a rewire, because there is no id to match on. Matching such
// cases would need a fuzzy (source,target) fallback; left out deliberately.

import type { Diagram, DiagramEdge, DiagramNode } from "../model";

// A node present in both versions whose fields differ. `fields` lists the
// changed keys for a compact changelog line ("changed label, fill").
export interface NodeChange {
  id: string;
  before: DiagramNode;
  after: DiagramNode;
  fields: (keyof DiagramNode)[];
}

// An edge present in both versions whose source/target moved.
export interface EdgeRewire {
  id: string;
  before: DiagramEdge;
  after: DiagramEdge;
}

export interface DiagramDiff {
  nodesAdded: DiagramNode[];
  nodesRemoved: DiagramNode[];
  nodesChanged: NodeChange[];
  edgesAdded: DiagramEdge[];
  edgesRemoved: DiagramEdge[];
  edgesRewired: EdgeRewire[];
}

// True when the diff carries no changes at all.
export function isEmptyDiff(diff: DiagramDiff): boolean {
  return (
    diff.nodesAdded.length === 0 &&
    diff.nodesRemoved.length === 0 &&
    diff.nodesChanged.length === 0 &&
    diff.edgesAdded.length === 0 &&
    diff.edgesRemoved.length === 0 &&
    diff.edgesRewired.length === 0
  );
}

export function diffDiagrams(before: Diagram, after: Diagram): DiagramDiff {
  const beforeNodes = byId(before.nodes);
  const afterNodes = byId(after.nodes);
  const beforeEdges = byId(before.edges);
  const afterEdges = byId(after.edges);

  const nodesAdded: DiagramNode[] = [];
  const nodesRemoved: DiagramNode[] = [];
  const nodesChanged: NodeChange[] = [];

  for (const [id, after] of afterNodes) {
    const prev = beforeNodes.get(id);
    if (!prev) {
      nodesAdded.push(after);
      continue;
    }
    const fields = changedFields(prev, after);
    if (fields.length) nodesChanged.push({ id, before: prev, after, fields });
  }
  for (const [id, prev] of beforeNodes) {
    if (!afterNodes.has(id)) nodesRemoved.push(prev);
  }

  const edgesAdded: DiagramEdge[] = [];
  const edgesRemoved: DiagramEdge[] = [];
  const edgesRewired: EdgeRewire[] = [];

  for (const [id, after] of afterEdges) {
    const prev = beforeEdges.get(id);
    if (!prev) {
      edgesAdded.push(after);
      continue;
    }
    if (prev.source !== after.source || prev.target !== after.target) {
      edgesRewired.push({ id, before: prev, after });
    }
  }
  for (const [id, prev] of beforeEdges) {
    if (!afterEdges.has(id)) edgesRemoved.push(prev);
  }

  return { nodesAdded, nodesRemoved, nodesChanged, edgesAdded, edgesRemoved, edgesRewired };
}

function byId<T extends { id: string }>(items: T[]): Map<string, T> {
  return new Map(items.map((item) => [item.id, item]));
}

// Keys that differ between two nodes, compared by value. Objects/arrays (columns)
// are compared structurally via JSON so a column edit registers.
function changedFields(before: DiagramNode, after: DiagramNode): (keyof DiagramNode)[] {
  const keys = new Set<keyof DiagramNode>([
    ...(Object.keys(before) as (keyof DiagramNode)[]),
    ...(Object.keys(after) as (keyof DiagramNode)[]),
  ]);
  const changed: (keyof DiagramNode)[] = [];
  for (const key of keys) {
    if (key === "id") continue;
    if (!valueEqual(before[key], after[key])) changed.push(key);
  }
  return changed;
}

function valueEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (typeof a === "object" || typeof b === "object") {
    return JSON.stringify(a) === JSON.stringify(b);
  }
  return false;
}
