// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Auto-layout via elkjs. Pure translation between the diagram model and ELK: it
// builds a hierarchy-aware ELK graph (containers become nested ELK nodes, edges
// are attached under the lowest common ancestor of their endpoints), runs the
// "layered" algorithm with orthogonal edge routing, and returns:
//   - node geometry: x/y RELATIVE to the parent (Vue Flow's convention) + size
//   - edge routing: bend points in ABSOLUTE canvas coordinates
// The caller writes geometry back into the model and hands the points to the
// custom edge for rendering.

import type { ELK, ElkNode, ElkExtendedEdge } from "elkjs/lib/elk-api";
import type { Diagram, DiagramNode } from "./model";

export interface NodeGeometry {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface LayoutResult {
  // Keyed by node id; x/y are relative to the parent, matching the model.
  nodes: Record<string, NodeGeometry>;
  // Keyed by edge id; absolute-coordinate polyline points (>= 2).
  edges: Record<string, { x: number; y: number }[]>;
}

// elk.bundled.js is ~1.4 MB, so load it on first use (when auto-layout is run)
// rather than in the initial bundle. The instance is cached across layouts.
let elkPromise: Promise<ELK> | null = null;
function getElk(): Promise<ELK> {
  if (!elkPromise) {
    elkPromise = import("elkjs/lib/elk.bundled.js").then((m) => new m.default());
  }
  return elkPromise;
}

const ROOT_OPTIONS: Record<string, string> = {
  "elk.algorithm": "layered",
  "elk.direction": "DOWN",
  "elk.edgeRouting": "ORTHOGONAL",
  "elk.hierarchyHandling": "INCLUDE_CHILDREN",
  "elk.layered.spacing.nodeNodeBetweenLayers": "60",
  "elk.spacing.nodeNode": "40",
  "elk.spacing.edgeNode": "24",
};

// Container inset so children clear the header bar (top) and the frame edges.
const CONTAINER_PADDING = "[top=32,left=16,bottom=16,right=16]";

export async function layoutDiagram(
  diagram: Diagram,
  sizeOf: (node: DiagramNode) => { width: number; height: number },
): Promise<LayoutResult> {
  const byId = new Map(diagram.nodes.map((n) => [n.id, n]));
  const childrenOf = new Map<string, DiagramNode[]>();
  const roots: DiagramNode[] = [];
  for (const node of diagram.nodes) {
    if (node.parent && byId.has(node.parent)) {
      const arr = childrenOf.get(node.parent) ?? [];
      arr.push(node);
      childrenOf.set(node.parent, arr);
    } else {
      roots.push(node);
    }
  }

  // A node with children becomes an ELK container (ELK sizes it from its
  // contents); a leaf carries an explicit size.
  function toElkNode(node: DiagramNode): ElkNode {
    const kids = childrenOf.get(node.id) ?? [];
    if (kids.length) {
      return {
        id: node.id,
        layoutOptions: { "elk.padding": CONTAINER_PADDING },
        children: kids.map(toElkNode),
      };
    }
    const size = sizeOf(node);
    return { id: node.id, width: size.width, height: size.height };
  }

  // Ancestor chain from self up to the top, for the lowest-common-ancestor of an
  // edge's endpoints. ELK requires each edge to live in that common container.
  function ancestors(id: string): string[] {
    const chain: string[] = [];
    let cur: DiagramNode | undefined = byId.get(id);
    while (cur) {
      chain.push(cur.id);
      cur = cur.parent ? byId.get(cur.parent) : undefined;
    }
    return chain;
  }
  function lca(a: string, b: string): string | null {
    const setA = new Set(ancestors(a));
    for (const id of ancestors(b)) if (setA.has(id)) return id;
    return null;
  }

  // Bucket edges by the container they belong in (null = the root graph).
  const rootEdges: ElkExtendedEdge[] = [];
  const edgesByContainer = new Map<string, ElkExtendedEdge[]>();
  for (const edge of diagram.edges) {
    if (!byId.has(edge.source) || !byId.has(edge.target)) continue;
    const elkEdge: ElkExtendedEdge = {
      id: edge.id,
      sources: [edge.source],
      targets: [edge.target],
    };
    const container = lca(edge.source, edge.target);
    if (container) {
      const arr = edgesByContainer.get(container) ?? [];
      arr.push(elkEdge);
      edgesByContainer.set(container, arr);
    } else {
      rootEdges.push(elkEdge);
    }
  }

  // Inject each container's edges into its ELK node.
  function withEdges(elkNode: ElkNode): ElkNode {
    const edges = edgesByContainer.get(elkNode.id);
    if (edges) elkNode.edges = edges;
    if (elkNode.children) elkNode.children.forEach(withEdges);
    return elkNode;
  }

  const graph: ElkNode = {
    id: "root",
    layoutOptions: ROOT_OPTIONS,
    children: roots.map(toElkNode).map(withEdges),
    edges: rootEdges,
  };

  const elk = await getElk();
  const laid = await elk.layout(graph);

  const nodes: Record<string, NodeGeometry> = {};
  const absById = new Map<string, { x: number; y: number }>();
  function collectNodes(elkNode: ElkNode, parentAbs: { x: number; y: number }) {
    const x = elkNode.x ?? 0;
    const y = elkNode.y ?? 0;
    const abs = { x: parentAbs.x + x, y: parentAbs.y + y };
    if (elkNode.id !== "root") {
      absById.set(elkNode.id, abs);
      nodes[elkNode.id] = { x, y, width: elkNode.width ?? 0, height: elkNode.height ?? 0 };
    }
    for (const child of elkNode.children ?? []) collectNodes(child, abs);
  }
  collectNodes(laid, { x: 0, y: 0 });

  // Edge section coordinates are relative to the edge's container node; shift them
  // into absolute canvas space so the custom edge can draw them directly.
  const edges: Record<string, { x: number; y: number }[]> = {};
  function collectEdges(elkNode: ElkNode) {
    const offset =
      elkNode.id === "root" ? { x: 0, y: 0 } : absById.get(elkNode.id) ?? { x: 0, y: 0 };
    for (const edge of elkNode.edges ?? []) {
      const section = edge.sections?.[0];
      if (!section) continue;
      edges[edge.id] = [section.startPoint, ...(section.bendPoints ?? []), section.endPoint].map(
        (p) => ({ x: p.x + offset.x, y: p.y + offset.y }),
      );
    }
    for (const child of elkNode.children ?? []) collectEdges(child);
  }
  collectEdges(laid);

  return { nodes, edges };
}
