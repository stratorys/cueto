// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";
import type { Diagram, DiagramEdge, DiagramNode } from "./model";
import { edgesBody, nodeBody, toCue, toFlowEdges } from "./mapping";

// Optional metadata is authored via the inspector and only reaches CUE through
// these pure serializers, so lock in that the fields round-trip (and that absent
// fields stay absent, keeping data.cue minimal).

function node(over: Partial<DiagramNode> = {}): DiagramNode {
  return { id: "n1", type: "shape", x: 0, y: 0, label: "", ...over };
}

function edge(over: Partial<DiagramEdge> = {}): DiagramEdge {
  return { id: "e1", source: "a", target: "b", kind: "relation", ...over };
}

describe("nodeBody icon round-trip", () => {
  it("emits icon when set", () => {
    expect(nodeBody(node({ icon: "database" }))).toContain('icon: "database"');
  });

  it("omits icon when absent", () => {
    expect(nodeBody(node())).not.toContain("icon:");
  });
});

describe("edgesBody cardinality", () => {
  it("emits card when set", () => {
    const body = edgesBody([edge({ card: "1-n" })]);
    expect(body).toContain('card: "1-n"');
  });

  it("omits card when absent", () => {
    const body = edgesBody([edge()]);
    expect(body).not.toContain("card:");
  });
});

describe("node type fidelity", () => {
  it("round-trips the typed node types to CUE", () => {
    for (const type of ["entity", "process", "decision"] as const) {
      expect(nodeBody(node({ type }))).toContain(`type: "${type}"`);
    }
  });

  it("emits typed node types through toCue", () => {
    const cue = toCue({ nodes: [node({ id: "review", type: "process" })], edges: [] });
    expect(cue).toContain('type: "process"');
  });
});

describe("edge kind fidelity", () => {
  it("round-trips every edge kind to CUE", () => {
    for (const kind of ["relation", "arrow", "inherit", "line"] as const) {
      expect(edgesBody([edge({ kind })])).toContain(`kind: "${kind}"`);
    }
  });

  it("passes the edge kind into the Vue Flow edge data", () => {
    const diagram: Diagram = {
      nodes: [node({ id: "a" }), node({ id: "b" })],
      edges: [edge({ source: "a", target: "b", kind: "arrow" })],
    };
    const [flow] = toFlowEdges(diagram);
    expect(flow.type).toBe("elk");
    expect(flow.data?.kind).toBe("arrow");
  });
});
