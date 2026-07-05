// Serialize the diagram model to CUE text (the `data.cue` body).
// This is the graph → CUE direction. Only data.cue is machine-written;
// schema.cue (the #Diagram / #Node / #Edge definitions) is hand-owned.

import type { Diagram } from "./model";

// A bare key if it is a plain identifier, otherwise a quoted string key.
function cueKey(key: string): string {
  return /^[a-zA-Z_]\w*$/.test(key) ? key : JSON.stringify(key);
}

// Emit a JSON-like value as CUE, tab-indented. undefined fields are dropped so
// optional model fields (pk, card, handles) only appear when set.
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

export function toCue(diagram: Diagram): string {
  // Nodes become a struct keyed by id (matches `nodes: [ID=string]: #Node`).
  const nodes: Record<string, unknown> = {};
  for (const node of diagram.nodes) {
    nodes[node.id] = {
      type: node.type,
      x: node.x,
      y: node.y,
      label: node.label,
      columns: node.columns,
    };
  }

  const edges = diagram.edges.map((edge) => ({
    id: edge.id,
    source: edge.source,
    sourceHandle: edge.sourceHandle,
    target: edge.target,
    targetHandle: edge.targetHandle,
    kind: edge.kind,
    card: edge.card,
  }));

  const body = emit({ nodes, edges }, 1);
  return `package diagram\n\ndiagram: #Diagram & ${body}\n`;
}
