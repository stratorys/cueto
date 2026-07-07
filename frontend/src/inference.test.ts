// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";
import type { TraceEntry } from "./api";
import { indexTrace, legendKinds } from "./inference";

// A small trace mirroring an inferred two-registry module: two registry nodes and one
// key-set reference edge, the shape the backend emits for the "why" inspector + legend.
const trace: TraceEntry[] = [
  { element: "teams/red", kind: "node", rule: "registry", detail: "teams" },
  { element: "people/marty", kind: "node", rule: "registry", detail: "people" },
  {
    element: "people/marty--team-->teams/red",
    kind: "edge",
    rule: "key-set-ref",
    detail: "people.team -> teams",
  },
];

describe("indexTrace", () => {
  it("maps a node id to its registry entry", () => {
    const entry = indexTrace(trace).get("people/marty");
    expect(entry?.rule).toBe("registry");
    expect(entry?.detail).toBe("people");
  });

  it("maps an edge id to its key-set reference entry", () => {
    const entry = indexTrace(trace).get("people/marty--team-->teams/red");
    expect(entry?.rule).toBe("key-set-ref");
    expect(entry?.detail).toBe("people.team -> teams");
  });

  it("is empty for a declared view with no trace", () => {
    expect(indexTrace([]).size).toBe(0);
  });
});

describe("legendKinds", () => {
  it("lists one sorted entry per distinct registry", () => {
    expect(legendKinds(trace)).toEqual(["people", "teams"]);
  });

  it("dedupes registries with many members and ignores edges", () => {
    const many: TraceEntry[] = [
      { element: "people/a", kind: "node", rule: "registry", detail: "people" },
      { element: "people/b", kind: "node", rule: "registry", detail: "people" },
      {
        element: "people/a--friend-->people/b",
        kind: "edge",
        rule: "key-set-ref",
        detail: "people.friend -> people",
      },
    ];
    expect(legendKinds(many)).toEqual(["people"]);
  });

  it("is empty for a declared view", () => {
    expect(legendKinds([])).toEqual([]);
  });
});
