import { describe, expect, it } from "vitest";
import type { Diagram, DiagramEdge, DiagramNode } from "../model";
import { runQuery } from "./query";

function node(id: string, over: Partial<DiagramNode> & Record<string, unknown> = {}): DiagramNode {
  return { id, type: "shape", x: 0, y: 0, label: id, ...over } as DiagramNode;
}
function edge(id: string, source: string, target: string, over: Record<string, unknown> = {}): DiagramEdge {
  return { id, source, target, kind: "relation", ...over } as DiagramEdge;
}
function diagram(nodes: DiagramNode[], edges: DiagramEdge[]): Diagram {
  return { nodes, edges };
}

const ids = (s: Set<string>) => [...s].sort();

describe("runQuery", () => {
  const d = diagram(
    [
      node("users", { type: "table" }),
      node("orders", { type: "table" }),
      node("payment_box", { type: "shape", shape: "rectangle" }),
      node("lonely", { type: "shape" }),
    ],
    [edge("e1", "users", "orders", { card: "n-n" }), edge("e2", "users", "payment_box")],
  );

  it("matches an exact field and includes the induced edge", () => {
    const result = runQuery(d, "type:table");
    expect(ids(result.nodeIds)).toEqual(["orders", "users"]);
    expect(ids(result.edgeIds)).toEqual(["e1"]); // both endpoints are tables
  });

  it("matches a substring on label", () => {
    expect(ids(runQuery(d, "label:~pay").nodeIds)).toEqual(["payment_box"]);
  });

  it("ANDs multiple tokens", () => {
    expect(ids(runQuery(d, "type:shape shape:rectangle").nodeIds)).toEqual(["payment_box"]);
  });

  it("finds orphans", () => {
    expect(ids(runQuery(d, "orphan").nodeIds)).toEqual(["lonely"]);
  });

  it("selects n-n relations and their endpoints", () => {
    const result = runQuery(d, "n-n");
    expect(ids(result.nodeIds)).toEqual(["orders", "users"]);
    expect(ids(result.edgeIds)).toEqual(["e1"]);
  });

  it("returns empty for an unknown keyword and for an empty query", () => {
    expect(runQuery(d, "bogus").nodeIds.size).toBe(0);
    expect(runQuery(d, "   ").nodeIds.size).toBe(0);
  });
});
