// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Mapping between the diagram model and its two projections:
//  - model -> Vue Flow nodes/edges (the canvas view)
//  - model -> CUE text (the `data.cue` body, graph -> CUE direction)
// The #Diagram / #Node / #Edge definitions live in the imported diagram package
// (github.com/stratorys/cueto/diagram); the default project is `package main`.

import type { Edge, Node } from "@vue-flow/core";
import type { Diagram, DiagramEdge, DiagramNode } from "./model";

// --- model -> Vue Flow ------------------------------------------------------

// Vue Flow requires every parent node to appear before its children in the
// nodes array. Order nodes so ancestors always precede descendants.
function sortParentsFirst(nodes: DiagramNode[]): DiagramNode[] {
  const byId = new Map(nodes.map((n) => [n.id, n]));
  const ordered: DiagramNode[] = [];
  const seen = new Set<string>();
  function visit(node: DiagramNode) {
    if (seen.has(node.id)) return;
    const parent = node.parent ? byId.get(node.parent) : undefined;
    if (parent) visit(parent);
    seen.add(node.id);
    ordered.push(node);
  }
  for (const node of nodes) visit(node);
  return ordered;
}

// Absolute (canvas) top-left of a node, summing x/y up the parent chain.
function absolutePosition(
  node: DiagramNode,
  byId: Map<string, DiagramNode>,
): { x: number; y: number } {
  let x = 0;
  let y = 0;
  let cur: DiagramNode | undefined = node;
  while (cur) {
    x += cur.x ?? 0;
    y += cur.y ?? 0;
    cur = cur.parent ? byId.get(cur.parent) : undefined;
  }
  return { x, y };
}

// Ids visible when drilled into `focus`: the focus container plus every node
// nested below it. When focus is null every node is visible.
export function visibleIds(diagram: Diagram, focus: string | null): Set<string> | null {
  if (!focus) return null;
  const children = new Map<string, string[]>();
  for (const n of diagram.nodes) {
    if (!n.parent) continue;
    const siblings = children.get(n.parent) ?? [];
    siblings.push(n.id);
    children.set(n.parent, siblings);
  }
  const visible = new Set<string>([focus]);
  const stack = [focus];
  while (stack.length) {
    const id = stack.pop()!;
    for (const child of children.get(id) ?? []) {
      visible.add(child);
      stack.push(child);
    }
  }
  return visible;
}

// Build the Vue Flow node list. When `focus` is set, only the focus container and
// its descendants render: the focus node is promoted to a root (its own parent is
// stripped and it is placed at its absolute position) so the subtree fills the
// canvas, while descendants keep their relative parent links.
export function toFlowNodes(
  diagram: Diagram,
  focus: string | null = null,
  autoPositions: NodePositions = {},
): Node[] {
  const byId = new Map(diagram.nodes.map((n) => [n.id, n]));
  const visible = visibleIds(diagram, focus);
  const shown = visible
    ? diagram.nodes.filter((n) => visible.has(n.id))
    : diagram.nodes;
  return sortParentsFirst(shown).map((node) => {
    const isFocusRoot = node.id === focus;
    // A data-derived node has no coordinates: position comes from the last
    // auto-layout (ephemeral view state), falling back to the origin until it runs.
    const position =
      node.x === undefined || node.y === undefined
        ? (autoPositions[node.id] ?? { x: 0, y: 0 })
        : isFocusRoot
          ? absolutePosition(node, byId)
          : { x: node.x, y: node.y };
    const parent = isFocusRoot ? undefined : node.parent;
    // Both hand-drawn and derived nodes drag. A hand-drawn drag writes coordinates back
    // to the model/text; a derived drag is ephemeral - it only nudges the auto-layout
    // position for readability and is never written to the file (see onNodeDragStop).
    return {
      id: node.id,
      type: node.type,
      position,
      draggable: true,
      // Nesting: a child's position is relative to its parent, is clipped to the
      // parent's box, and grows the parent when dragged to its edge.
      parentNode: parent,
      extent: parent ? "parent" : undefined,
      expandParent: parent ? true : undefined,
      // Explicit size drives the node box; ShapeNode fills it. Omitted -> the
      // node auto-sizes to the node's min size.
      style:
        node.width && node.height
          ? { width: `${node.width}px`, height: `${node.height}px` }
          : undefined,
      data: {
        label: node.label,
        type: node.type,
        shape: node.shape,
        fill: node.fill,
        stroke: node.stroke,
        flip: node.flip,
        columns: node.columns,
        data: node.data,
      },
    };
  });
}

// Absolute-coordinate bend points per edge id, produced by the last auto-layout.
// Ephemeral view state (not in the model / CUE); the custom "elk" edge draws them
// and falls back to a smooth-step path when an edge has none.
export type EdgePoints = Record<string, { x: number; y: number }[]>;

// Absolute positions per node id from the last auto-layout of a coordinate-free
// (data-derived) diagram. Ephemeral view state, exactly like EdgePoints: never
// written into the model or the CUE, so the derived file stays coordinate-free.
export type NodePositions = Record<string, { x: number; y: number }>;

export function toFlowEdges(
  diagram: Diagram,
  focus: string | null = null,
  edgePoints: EdgePoints = {},
): Edge[] {
  const visible = visibleIds(diagram, focus);
  return diagram.edges
    .filter((edge) => !visible || (visible.has(edge.source) && visible.has(edge.target)))
    .map((edge) => ({
      id: edge.id,
      source: edge.source,
      target: edge.target,
      sourceHandle: edge.sourceHandle,
      targetHandle: edge.targetHandle,
      // `type` selects the ELK-polyline edge component (orthogonal to the visual
      // `kind`, which ElkEdge reads from data to pick its marker/dash).
      type: "elk",
      // Endpoints are draggable: reconnect to another handle, or drop in empty
      // space to turn the relation back into a floating line (see useDiagramCanvas).
      updatable: true,
      style: { stroke: "#64748b" },
      data: {
        points: edgePoints[edge.id],
        kind: edge.kind,
        label: edge.label,
        card: edge.card,
      },
    }));
}

// --- model -> CUE text ------------------------------------------------------

// A bare key if it is a plain identifier, otherwise a quoted string key.
function cueKey(key: string): string {
  return /^[a-zA-Z_]\w*$/.test(key) ? key : JSON.stringify(key);
}

// Emit a JSON-like value as CUE, tab-indented. undefined fields are dropped so
// optional model fields (handles) only appear when set.
function emit(value: unknown, indent: number): string {
  const pad = "\t".repeat(indent);
  const padIn = "\t".repeat(indent + 1);

  if (Array.isArray(value)) {
    if (value.length === 0) return "[]";
    const items = value.map((item) => padIn + emit(item, indent + 1));
    return `[\n${items.join(",\n")},\n${pad}]`;
  }

  if (value !== null && typeof value === "object") {
    const entries = Object.entries(value).filter(([, v]) => v !== undefined);
    if (entries.length === 0) return "{}";
    const lines = entries.map(
      ([k, v]) => `${padIn}${cueKey(k)}: ${emit(v, indent + 1)}`,
    );
    return `{\n${lines.join("\n")}\n${pad}}`;
  }

  if (typeof value === "string") return JSON.stringify(value);
  return String(value);
}

// The CUE-emitted field shape of one node. sourceFile is frontend-only provenance
// metadata and is deliberately excluded, so it never leaks into the CUE text.
function nodeFields(node: DiagramNode): Record<string, unknown> {
  return {
    type: node.type,
    parent: node.parent,
    x: node.x,
    y: node.y,
    width: node.width,
    height: node.height,
    label: node.label,
    shape: node.shape,
    fill: node.fill,
    stroke: node.stroke,
    flip: node.flip,
    icon: node.icon,
    columns: node.columns,
    data: node.data,
  };
}

// The CUE-emitted field shape of one edge (sourceFile excluded, as for nodes).
function edgeFields(edge: DiagramEdge): Record<string, unknown> {
  return {
    id: edge.id,
    source: edge.source,
    sourceHandle: edge.sourceHandle,
    target: edge.target,
    targetHandle: edge.targetHandle,
    kind: edge.kind,
    label: edge.label,
    card: edge.card,
  };
}

// One node's CUE struct body (`{ ... }`), for splicing into a file via /rewrite.
export function nodeBody(node: DiagramNode): string {
  return emit(nodeFields(node), 0);
}

// The whole edge list as a CUE list literal, for replacing an owner file's
// `edges:` via /rewrite.
export function edgesBody(edges: DiagramEdge[]): string {
  return emit(edges.map(edgeFields), 0);
}

// Sentinel marking the app-managed region at the end of a data-derived file.
// Everything from this line to EOF is regenerated from the drawn (coord-bearing)
// nodes; the derivation above it is authored by hand and never touched.
export const CANVAS_SENTINEL =
  "// >>> canvas: hand-drawn shapes (managed by the editor - do not edit below) <<<";

// The managed block for a derived diagram's hand-drawn shapes: a literal
// `diagram: nodes: { … }` that unifies with the derived nodes above. Emits only
// the given (drawn) nodes; empty string when there are none, so the block is
// dropped entirely. Reuses nodeFields/emit for identical formatting.
export function canvasBlock(nodes: DiagramNode[]): string {
  if (nodes.length === 0) return "";
  const struct: Record<string, unknown> = {};
  for (const node of nodes) struct[node.id] = nodeFields(node);
  return `${CANVAS_SENTINEL}\ndiagram: nodes: ${emit(struct, 0)}\n`;
}

export function toCue(diagram: Diagram): string {
  // Nodes become a struct keyed by id (matches `nodes: [ID=string]: #Node`).
  const nodes: Record<string, unknown> = {};
  for (const node of diagram.nodes) nodes[node.id] = nodeFields(node);

  const edges = diagram.edges.map(edgeFields);

  const body = emit({ nodes, edges }, 1);
  return `package main\n\nimport d "github.com/stratorys/cueto/diagram"\n\ndiagram: d.#Diagram & ${body}\n`;
}
