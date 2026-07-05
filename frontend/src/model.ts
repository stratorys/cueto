// Diagram model - the JSON source of truth.
// Mirrors the CUE #Diagram / #Node / #Edge shape from the design notes.
// Every node and edge carries a stable id so edits can round-trip to CUE later.

export type NodeType = "entity" | "table" | "process" | "decision";

export interface Column {
  name: string;
  dbType: string;
  pk?: boolean;
  fk?: boolean;
}

export interface DiagramNode {
  id: string;
  type: NodeType;
  x: number;
  y: number;
  label: string;
  columns?: Column[];
}

export type EdgeKind = "relation" | "arrow" | "inherit";
export type Cardinality = "1-1" | "1-n" | "n-n";

export interface DiagramEdge {
  id: string;
  source: string;
  target: string;
  // Which column handle the edge attaches to, e.g. "id-source" / "user_id-target".
  // Omitted for plain nodes that expose only a default handle.
  sourceHandle?: string;
  targetHandle?: string;
  kind: EdgeKind;
  card?: Cardinality;
}

export interface Diagram {
  nodes: DiagramNode[];
  edges: DiagramEdge[];
}

// Milestone 1: a hardcoded model. Later this comes from the Go /eval service.
export const sampleDiagram: Diagram = {
  nodes: [
    {
      id: "user",
      type: "table",
      x: 80,
      y: 80,
      label: "user",
      columns: [
        { name: "id", dbType: "uuid", pk: true },
        { name: "email", dbType: "text" },
      ],
    },
    {
      id: "order",
      type: "table",
      x: 460,
      y: 120,
      label: "order",
      columns: [
        { name: "id", dbType: "uuid", pk: true },
        { name: "user_id", dbType: "uuid", fk: true },
        { name: "total", dbType: "numeric" },
      ],
    },
    {
      id: "review",
      type: "process",
      x: 300,
      y: 380,
      label: "review order",
    },
  ],
  edges: [
    {
      id: "e_user_order",
      source: "user",
      sourceHandle: "id-source",
      target: "order",
      targetHandle: "user_id-target",
      kind: "relation",
      card: "1-n",
    },
    {
      id: "e_order_review",
      source: "order",
      sourceHandle: "id-source",
      target: "review",
      kind: "arrow",
    },
  ],
};
