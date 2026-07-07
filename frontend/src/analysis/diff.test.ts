// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";
import type { Diagram, DiagramEdge, DiagramNode } from "../model";
import { diffDiagrams, isEmptyDiff } from "./diff";

function node(id: string, over: Partial<DiagramNode> = {}): DiagramNode {
  return { id, type: "shape", x: 0, y: 0, label: id, ...over };
}
function edge(id: string, source: string, target: string): DiagramEdge {
  return { id, source, target, kind: "relation" };
}
function diagram(nodes: DiagramNode[], edges: DiagramEdge[]): Diagram {
  return { nodes, edges };
}

describe("diffDiagrams", () => {
  it("reports added, removed and field-changed nodes", () => {
    const before = diagram([node("a", { label: "A" }), node("b")], []);
    const after = diagram([node("a", { label: "A2" }), node("c")], []);
    const diff = diffDiagrams(before, after);

    expect(diff.nodesAdded.map((n) => n.id)).toEqual(["c"]);
    expect(diff.nodesRemoved.map((n) => n.id)).toEqual(["b"]);
    expect(diff.nodesChanged).toHaveLength(1);
    expect(diff.nodesChanged[0].id).toBe("a");
    expect(diff.nodesChanged[0].fields).toEqual(["label"]);
  });

  it("detects an edge rewire vs an add/remove", () => {
    const before = diagram(
      [node("a"), node("b"), node("c")],
      [edge("e1", "a", "b"), edge("e2", "a", "c")],
    );
    const after = diagram(
      [node("a"), node("b"), node("c")],
      [edge("e1", "a", "c"), edge("e3", "b", "c")], // e1 rewired, e2 removed, e3 added
    );
    const diff = diffDiagrams(before, after);

    expect(diff.edgesRewired).toHaveLength(1);
    expect(diff.edgesRewired[0].id).toBe("e1");
    expect(diff.edgesRewired[0].after.target).toBe("c");
    expect(diff.edgesRemoved.map((e) => e.id)).toEqual(["e2"]);
    expect(diff.edgesAdded.map((e) => e.id)).toEqual(["e3"]);
  });

  it("detects column changes via structural comparison", () => {
    const before = diagram(
      [node("t", { type: "table", columns: [{ name: "id", dbType: "int" }] })],
      [],
    );
    const after = diagram(
      [node("t", { type: "table", columns: [{ name: "id", dbType: "uuid" }] })],
      [],
    );
    const diff = diffDiagrams(before, after);
    expect(diff.nodesChanged[0].fields).toEqual(["columns"]);
  });

  it("is empty for identical diagrams", () => {
    const d = diagram([node("a")], [edge("e", "a", "a")]);
    expect(isEmptyDiff(diffDiagrams(d, structuredClone(d)))).toBe(true);
  });
});
