// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Diagram model - the JSON source of truth.
// Mirrors the CUE #Diagram / #Node / #Edge shape. Every node and edge carries a
// stable id so edits round-trip to CUE.

// A canvas element is a free-form "shape", a DB "table", a "container" that holds
// other nodes (children point at it via DiagramNode.parent), or one of the typed
// domain nodes ("entity" / "process" / "decision") that render a fixed silhouette.
export type NodeType = "shape" | "table" | "container" | "entity" | "process" | "decision";

// The typed domain nodes: a fixed-silhouette subset of NodeType with no per-node
// payload (unlike table's columns), drawn by TypedNode from its type alone.
export type TypedNodeType = "entity" | "process" | "decision";
export type ShapeKind = "rectangle" | "ellipse" | "diamond" | "line" | "text";

// A palette tool: a shape to draw, or the "connect" mode that reveals node
// handles so a handle-to-handle drag creates a relation edge.
export type Tool = ShapeKind | "connect";

// Domain-architecture metadata (mirrors the optional schema fields). `role`
// governs (policy/drift); `type` renders.
export type NodeRole = "service" | "database" | "queue" | "cache" | "gateway" | "external";
export type EdgeCall = "calls" | "reads" | "writes" | "publishes" | "subscribes";
export type EdgeProtocol = "http" | "grpc" | "amqp" | "sql";
export type EdgeCard = "1-1" | "1-n" | "n-n";

// One column of a DB table node. Mirrors the CUE #Column.
export interface Column {
  name: string;
  dbType: string;
  pk?: boolean;
  fk?: boolean;
}

export interface DiagramNode {
  id: string;
  type: NodeType;
  // Id of the containing node, when nested. A child's x/y are relative to its
  // parent's top-left (Vue Flow's parentNode convention); a top-level node's
  // x/y are absolute. Absent -> top level.
  parent?: string;
  x: number;
  y: number;
  // Explicit size in graph units. Absent -> the shape falls back to a min size.
  width?: number;
  height?: number;
  label: string;
  // Set only when type is "shape".
  shape?: ShapeKind;
  // Optional per-shape colors (any CSS color, e.g. "#f59e0b" or "transparent").
  fill?: string;
  stroke?: string;
  // Line only: drag direction. true = "\" (top-left -> bottom-right); absent = "/".
  flip?: boolean;
  // Set only when type is "table".
  columns?: Column[];
  // Optional domain metadata, round-tripped to CUE for policy/drift checks.
  role?: NodeRole;
  owner?: string;
  region?: string;
  zone?: string;
  // Which editable file authored this node (from /eval provenance). Drives which
  // file a canvas edit is written back into. Absent -> the primary data.cue.
  sourceFile?: string;
}

export interface DiagramEdge {
  id: string;
  source: string;
  target: string;
  // Which side handle the edge attaches to, e.g. "r" / "l". Omitted for a node's
  // default handle.
  sourceHandle?: string;
  targetHandle?: string;
  // Visual connector kind. "relation" is a plain link; "arrow" adds a filled
  // arrowhead; "inherit" a hollow (UML generalization) triangle; "line" is a bare
  // dashed connector. Drives the marker/dash in ElkEdge and round-trips to CUE.
  kind: "relation" | "arrow" | "inherit" | "line";
  // Optional free-form text drawn at the edge midpoint, edited inline by
  // double-clicking the edge (mirrors a shape's label).
  label?: string;
  // Optional domain metadata, round-tripped to CUE for policy/drift checks.
  card?: EdgeCard;
  call?: EdgeCall;
  protocol?: EdgeProtocol;
  sync?: boolean;
  // Which editable file authored this edge (from /eval provenance). Edges are a
  // single unsplittable list, so in practice all edges share one owner file.
  sourceFile?: string;
}

export interface Diagram {
  nodes: DiagramNode[];
  edges: DiagramEdge[];
  // Governance packs this diagram opts into (e.g. ["security"]).
  policies?: string[];
}

// One editable CUE file in the multi-file package: a bare .cue name and its text.
export interface EditorFile {
  name: string;
  text: string;
}

// Element -> owning file, as returned by /eval. `nodes` maps a node id to its
// file; `edges` names the single file that owns the edge list.
export interface Provenance {
  nodes: Record<string, string>;
  edges: string;
}

// A hardcoded starter model. Re-evaluated from the Go /eval service on text edits.
export const sampleDiagram: Diagram = {
  nodes: [
    { id: "a", type: "shape", shape: "rectangle", x: 120, y: 140, width: 140, height: 72, label: "" },
    { id: "b", type: "shape", shape: "ellipse", x: 440, y: 180, width: 140, height: 90, label: "" },
    { id: "c", type: "shape", shape: "diamond", x: 280, y: 380, width: 110, height: 110, label: "" },
  ],
  edges: [
    {
      id: "e_a_b",
      source: "a",
      sourceHandle: "r",
      target: "b",
      targetHandle: "l",
      kind: "relation",
    },
  ],
};
