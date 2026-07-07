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

describe("edge routing waypoints", () => {
  it("emits dragged points to CUE and passes them into the edge data", () => {
    const points = [{ t: 0.5, off: 20 }];
    expect(edgesBody([edge({ points })])).toContain("off: 20");
    const diagram: Diagram = {
      nodes: [node({ id: "a" }), node({ id: "b" })],
      edges: [edge({ source: "a", target: "b", points })],
    };
    expect(toFlowEdges(diagram)[0].data?.waypoints).toEqual(points);
  });

  it("omits points when absent", () => {
    expect(edgesBody([edge()])).not.toContain("points:");
  });

  it("applies pinned waypoints to a derived edge (view state, not CUE)", () => {
    const pinned = [{ t: 0.5, off: 30 }];
    const diagram: Diagram = {
      nodes: [node({ id: "a" }), node({ id: "b" })],
      edges: [edge({ source: "a", target: "b" })],
    };
    const [flow] = toFlowEdges(diagram, null, {}, { e1: pinned });
    expect(flow.data?.waypoints).toEqual(pinned);
  });

  it("prefers the edge's own points over a pinned route", () => {
    const own = [{ t: 0.25, off: 10 }];
    const pinned = [{ t: 0.75, off: 40 }];
    const diagram: Diagram = {
      nodes: [node({ id: "a" }), node({ id: "b" })],
      edges: [edge({ source: "a", target: "b", points: own })],
    };
    const [flow] = toFlowEdges(diagram, null, {}, { e1: pinned });
    expect(flow.data?.waypoints).toEqual(own);
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
