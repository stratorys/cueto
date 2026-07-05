// Map the backend /eval JSON to the frontend Diagram model.
// The backend returns nodes as a struct keyed by id (matching the CUE schema);
// the frontend model uses a node array. Edges already match one-to-one.

import type { Diagram, DiagramEdge, DiagramNode } from "./model";

interface EvalDiagram {
  nodes?: Record<string, Omit<DiagramNode, "id"> & { id?: string }>;
  edges?: DiagramEdge[];
}

export function fromEval(raw: unknown): Diagram {
  const source = (raw ?? {}) as EvalDiagram;

  const nodes: DiagramNode[] = Object.entries(source.nodes ?? {}).map(
    ([id, node]) => ({ ...node, id }),
  );

  const edges: DiagramEdge[] = source.edges ?? [];

  return { nodes, edges };
}
