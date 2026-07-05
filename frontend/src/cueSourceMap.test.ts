// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";
import { findEdgeRange, findElementRange, findNodeRange } from "./cueSourceMap";
import { toCue } from "./mapping";
import { sampleDiagram } from "./model";
import type { Diagram } from "./model";

const slice = (text: string, r: { from: number; to: number } | null) =>
  r ? text.slice(r.from, r.to) : null;

describe("findNodeRange", () => {
  const text = toCue(sampleDiagram);

  it("covers a node's whole struct block", () => {
    const block = slice(text, findNodeRange(text, "a"));
    expect(block).not.toBeNull();
    // The range starts at the line's beginning (leading indentation included) so
    // a full-line editor decoration tints the whole block.
    expect(block!.trimStart().startsWith("a: {")).toBe(true);
    expect(block!.endsWith("}")).toBe(true);
    expect(block).toContain('type: "shape"');
    // The block stops at its own closing brace, before the next sibling key.
    expect(block).not.toContain("b: {");
  });

  it("resolves every sample node id", () => {
    for (const id of ["a", "b", "c"]) {
      expect(findNodeRange(text, id)).not.toBeNull();
    }
  });

  it("returns null for an unknown id", () => {
    expect(findNodeRange(text, "zzz")).toBeNull();
  });

  it("does not match an edge's id field", () => {
    // e_a_b is an edge id, emitted as `id: "e_a_b"`, not a struct key.
    expect(findNodeRange(text, "e_a_b")).toBeNull();
  });

  it("handles a quoted (non-identifier) key", () => {
    const diagram: Diagram = {
      nodes: [{ id: "a-b", type: "shape", shape: "rectangle", x: 0, y: 0, label: "x" }],
      edges: [],
    };
    const t = toCue(diagram);
    const block = slice(t, findNodeRange(t, "a-b"));
    expect(block).not.toBeNull();
    expect(block!.trimStart().startsWith('"a-b": {')).toBe(true);
  });

  it("finds an edge object by its id field", () => {
    const block = slice(text, findEdgeRange(text, "e_a_b"));
    expect(block).not.toBeNull();
    expect(block!.trimStart().startsWith("{")).toBe(true);
    expect(block!.endsWith("}")).toBe(true);
    expect(block).toContain('id: "e_a_b"');
    expect(block).toContain('source: "a"');
  });

  it("returns null for an unknown edge id", () => {
    expect(findEdgeRange(text, "nope")).toBeNull();
  });

  it("findElementRange resolves both a node and an edge", () => {
    expect(findElementRange(text, "a")).toEqual(findNodeRange(text, "a"));
    expect(findElementRange(text, "e_a_b")).toEqual(findEdgeRange(text, "e_a_b"));
    expect(findElementRange(text, "ghost")).toBeNull();
  });

  it("is not fooled by braces inside a string value", () => {
    const diagram: Diagram = {
      nodes: [
        { id: "a", type: "shape", shape: "rectangle", x: 0, y: 0, label: "has } brace" },
        { id: "b", type: "shape", shape: "ellipse", x: 0, y: 0, label: "ok" },
      ],
      edges: [],
    };
    const t = toCue(diagram);
    const block = slice(t, findNodeRange(t, "a"));
    expect(block).toContain('label: "has } brace"');
    expect(block).not.toContain("b: {");
  });
});
