// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";
import type { Diagram, DiagramEdge, DiagramNode, NodeType } from "../model";
import { blastRadius, findCycles, orphans, simulateDown, singlePointsOfFailure } from "./graph";

// Compact fixture builders: ids only, geometry is irrelevant to graph analysis.
function node(id: string, type: NodeType = "shape"): DiagramNode {
  return { id, type, x: 0, y: 0, label: id };
}
function edge(source: string, target: string): DiagramEdge {
  return { id: `${source}_${target}`, source, target, kind: "relation" };
}
function diagram(nodes: DiagramNode[], edges: DiagramEdge[]): Diagram {
  return { nodes, edges };
}

const sorted = (s: Iterable<string>) => [...s].sort();

describe("blastRadius", () => {
  it("returns transitive dependents (who relies on the seed)", () => {
    // web -> api -> db : db down breaks api and web.
    const d = diagram(
      [node("web"), node("api"), node("db")],
      [edge("web", "api"), edge("api", "db")],
    );
    expect(sorted(blastRadius(d, "db"))).toEqual(["api", "web"]);
    // Direction flipped: what db depends on (nothing).
    expect(sorted(blastRadius(d, "db", "dependsOn"))).toEqual([]);
    expect(sorted(blastRadius(d, "web", "dependsOn"))).toEqual(["api", "db"]);
  });

  it("excludes the seed even inside a cycle", () => {
    const d = diagram([node("a"), node("b")], [edge("a", "b"), edge("b", "a")]);
    expect(sorted(blastRadius(d, "a"))).toEqual(["b"]);
  });
});

describe("simulateDown", () => {
  it("unions impact of multiple downed nodes and drops the downed set", () => {
    // web -> api -> db, log -> api
    const d = diagram(
      [node("web"), node("api"), node("db"), node("log")],
      [edge("web", "api"), edge("api", "db"), edge("log", "api")],
    );
    expect(sorted(simulateDown(d, ["api"]))).toEqual(["log", "web"]);
    expect(sorted(simulateDown(d, ["db", "log"]))).toEqual(["api", "web"]);
  });
});

describe("orphans", () => {
  it("finds nodes with no incident edge and ignores containers", () => {
    const d = diagram(
      [node("a"), node("b"), node("lonely"), node("group", "container")],
      [edge("a", "b")],
    );
    expect(sorted(orphans(d))).toEqual(["lonely"]);
  });
});

describe("findCycles", () => {
  it("detects a multi-node cycle and a self-loop, ignoring a DAG", () => {
    const d = diagram(
      [node("a"), node("b"), node("c"), node("d"), node("self")],
      [edge("a", "b"), edge("b", "c"), edge("c", "a"), edge("c", "d"), edge("self", "self")],
    );
    const cycles = findCycles(d).map((c) => sorted(c));
    expect(cycles).toContainEqual(["a", "b", "c"]);
    expect(cycles).toContainEqual(["self"]);
    expect(cycles).toHaveLength(2);
  });

  it("returns nothing for an acyclic graph", () => {
    const d = diagram([node("a"), node("b")], [edge("a", "b")]);
    expect(findCycles(d)).toEqual([]);
  });
});

describe("singlePointsOfFailure", () => {
  it("finds the cut vertex of a chain", () => {
    // a - b - c : b is the articulation point.
    const d = diagram([node("a"), node("b"), node("c")], [edge("a", "b"), edge("b", "c")]);
    expect(singlePointsOfFailure(d)).toEqual(["b"]);
  });

  it("finds no SPOF in a cycle (every node has an alternate path)", () => {
    const d = diagram(
      [node("a"), node("b"), node("c")],
      [edge("a", "b"), edge("b", "c"), edge("c", "a")],
    );
    expect(singlePointsOfFailure(d)).toEqual([]);
  });
});
