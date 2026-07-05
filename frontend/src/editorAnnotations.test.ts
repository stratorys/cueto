// buildDeco is pure (RawAnnotation[] + doc -> DecorationSet) and never touches the
// DOM, so it runs under the node test env. These tests lock in the runtime
// invariants CodeMirror would otherwise only enforce with a throw: ranges in
// document order, in-bounds lines, and the underline clamp to line end.

import { describe, expect, it } from "vitest";
import { Text } from "@codemirror/state";
import type { DecorationSet } from "@codemirror/view";
import { buildDeco, type RawAnnotation } from "./editorAnnotations";

// A doc with a 15-char line, a 9-char line, and an empty line. Offsets:
// line 1 -> [0, 15], line 2 -> [16, 25], line 3 -> [26, 26].
const doc = Text.of(["package diagram", 'name: "x"', ""]);

interface Entry {
  from: number;
  to: number;
  cls?: string;
  widget?: string;
}

// Flatten a DecorationSet into inspectable entries in iteration (document) order.
function collect(set: DecorationSet): Entry[] {
  const out: Entry[] = [];
  const cursor = set.iter();
  while (cursor.value) {
    const spec = cursor.value.spec as { class?: string; widget?: { text?: string } };
    out.push({ from: cursor.from, to: cursor.to, cls: spec.class, widget: spec.widget?.text });
    cursor.next();
  }
  return out;
}

describe("buildDeco", () => {
  it("returns ranges in non-decreasing document order regardless of input order", () => {
    const anns: RawAnnotation[] = [
      { line: 3, column: 1, text: "b", variant: "type" },
      { line: 1, column: 1, text: "a", variant: "error" },
      { line: 2, column: 3, text: "c", variant: "warning" },
    ];
    const ranges = collect(buildDeco(anns, doc));
    for (let i = 1; i < ranges.length; i++) {
      expect(ranges[i].from).toBeGreaterThanOrEqual(ranges[i - 1].from);
    }
  });

  it("drops annotations outside the document's line range", () => {
    const anns: RawAnnotation[] = [
      { line: 0, column: 1, text: "before", variant: "error" },
      { line: 99, column: 1, text: "after", variant: "error" },
    ];
    expect(collect(buildDeco(anns, doc))).toHaveLength(0);
  });

  it("emits a line decoration, an underline, and a widget for an error", () => {
    const ranges = collect(buildDeco([{ line: 2, column: 3, text: "oops", variant: "error" }], doc));
    expect(ranges).toContainEqual({ from: 16, to: 16, cls: "cm-xray-line-error", widget: undefined });
    expect(ranges).toContainEqual({ from: 18, to: 25, cls: "cm-xray-underline-error", widget: undefined });
    expect(ranges).toContainEqual({ from: 25, to: 25, cls: undefined, widget: "oops" });
  });

  it("omits the underline on an empty line but still emits the line deco and widget", () => {
    const ranges = collect(buildDeco([{ line: 3, column: 1, text: "e", variant: "error" }], doc));
    expect(ranges.some((r) => r.cls === "cm-xray-line-error")).toBe(true);
    expect(ranges.some((r) => r.cls === "cm-xray-underline-error")).toBe(false);
    expect(ranges.some((r) => r.widget === "e")).toBe(true);
  });

  it("omits the underline when the column is past the line's end", () => {
    const ranges = collect(buildDeco([{ line: 2, column: 99, text: "x", variant: "warning" }], doc));
    expect(ranges.some((r) => r.cls === "cm-xray-underline-warning")).toBe(false);
    expect(ranges.some((r) => r.widget === "x")).toBe(true);
  });

  it("emits only a widget for a type/optional hint (no line or underline)", () => {
    const ranges = collect(buildDeco([{ line: 2, column: 1, text: ": string", variant: "type" }], doc));
    expect(ranges).toHaveLength(1);
    expect(ranges[0]).toEqual({ from: 25, to: 25, cls: undefined, widget: ": string" });
  });

  it("builds a well-formed ordered set from mixed variants across lines", () => {
    const anns: RawAnnotation[] = [
      { line: 1, column: 1, text: ": pkg", variant: "type" },
      { line: 2, column: 3, text: "bad value", variant: "error" },
      { line: 2, column: 1, text: "+ port", variant: "optional" },
      { line: 3, column: 1, text: "incomplete", variant: "warning" },
    ];
    const ranges = collect(buildDeco(anns, doc));
    expect(ranges.length).toBeGreaterThan(0);
    for (let i = 1; i < ranges.length; i++) {
      expect(ranges[i].from).toBeGreaterThanOrEqual(ranges[i - 1].from);
    }
  });
});
