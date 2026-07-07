// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";
import type { Diagram, DiagramEdge, DiagramNode } from "./model";
import { edgesBody, facingHandle, nodeBody, toCue, toFlowEdges } from "./mapping";

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

describe("facingHandle (nearest-dot docking)", () => {
  // A table box centered at the origin; `ref` is where the edge needs to go.
  const self = { x: -50, y: -20, w: 100, h: 40 };

  it("keeps a source dot on the right when the other end is to the right", () => {
    expect(facingHandle("powers-source", self, { x: 500, y: 0 })).toBe("powers-source");
  });

  it("flips a source dot to the left when the other end is to the left", () => {
    expect(facingHandle("powers-source", self, { x: -500, y: 0 })).toBe("powers-source-l");
  });

  it("keeps a target dot on the left, or mirrors it right when the other end is right", () => {
    expect(facingHandle("id-target", self, { x: -500, y: 0 })).toBe("id-target");
    expect(facingHandle("id-target", self, { x: 500, y: 0 })).toBe("id-target-r");
  });

  it("flips the header handles too", () => {
    expect(facingHandle("table-source", self, { x: -500, y: 0 })).toBe("table-source-l");
    expect(facingHandle("table-target", self, { x: 500, y: 0 })).toBe("table-target-r");
  });

  it("leaves a shape side handle and unknown ids untouched", () => {
    expect(facingHandle("r", self, { x: -500, y: 0 })).toBe("r");
    expect(facingHandle(undefined, self, { x: 0, y: 0 })).toBeUndefined();
  });

  it("re-docks an edge in toFlowEdges from the node boxes", () => {
    // b sits far left of a, so a's source dot flips left and b's target dot flips right.
    const diagram: Diagram = {
      nodes: [node({ id: "a" }), node({ id: "b" })],
      edges: [edge({ source: "a", target: "b", sourceHandle: "fk-source", targetHandle: "id-target" })],
    };
    const boxes = {
      a: { x: 0, y: 0, w: 100, h: 40 },
      b: { x: -400, y: 0, w: 100, h: 40 },
    };
    const [flow] = toFlowEdges(diagram, null, {}, {}, boxes);
    expect(flow.sourceHandle).toBe("fk-source-l");
    expect(flow.targetHandle).toBe("id-target-r");
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
