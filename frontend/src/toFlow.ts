// Map the diagram model to Vue Flow's node/edge shape.
// Milestone 1 uses default nodes; milestone 2 swaps in custom node components.

import type { Edge, Node } from "@vue-flow/core";
import type { Diagram } from "./model";

export function toFlowNodes(diagram: Diagram): Node[] {
  return diagram.nodes.map((node) => ({
    id: node.id,
    // "table" routes to the custom TableNode; others use Vue Flow's default node.
    type: node.type === "table" ? "table" : undefined,
    position: { x: node.x, y: node.y },
    data: { label: node.label, columns: node.columns, type: node.type },
  }));
}

export function toFlowEdges(diagram: Diagram): Edge[] {
  return diagram.edges.map((edge) => ({
    id: edge.id,
    source: edge.source,
    target: edge.target,
    sourceHandle: edge.sourceHandle,
    targetHandle: edge.targetHandle,
    label: edge.card ?? edge.kind,
  }));
}
