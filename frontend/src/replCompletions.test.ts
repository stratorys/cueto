// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";
import type { CompletionContext, CompletionResult } from "@codemirror/autocomplete";
import type { CueMeta } from "./api";
import { cueCompletionSource, walkKeys } from "./replCompletions";

const meta: CueMeta = {
  builtins: [{ name: "len", isFunc: true }],
  packages: [
    {
      path: "strings",
      name: "strings",
      members: [
        { name: "ToUpper", isFunc: true },
        { name: "MinRunes", isFunc: true },
      ],
    },
  ],
};

// A minimal stand-in for CodeMirror's CompletionContext: the source only reads
// matchBefore and explicit.
function fakeCtx(text: string, explicit = false): CompletionContext {
  return {
    explicit,
    matchBefore(_re: RegExp) {
      const matched = /[\w.]*$/.exec(text)?.[0] ?? "";
      return { from: text.length - matched.length, to: text.length, text: matched };
    },
  } as unknown as CompletionContext;
}

describe("walkKeys", () => {
  it("flattens nested structs into dotted paths", () => {
    const keys = walkKeys("diagram", {
      nodes: { a: { owner: "x" } },
      edges: [{ id: "e1" }],
    });
    expect(keys.has("diagram")).toBe(true);
    expect(keys.has("diagram.nodes")).toBe(true);
    expect(keys.has("diagram.nodes.a")).toBe(true);
    expect(keys.has("diagram.nodes.a.owner")).toBe(true);
    // Arrays are not descended into.
    expect(keys.has("diagram.edges")).toBe(true);
    expect([...keys].some((k) => k.includes("e1"))).toBe(false);
  });

  it("skips non-identifier keys", () => {
    const keys = walkKeys("diagram", { "a-b": { c: 1 }, ok: { d: 2 } });
    expect(keys.has("diagram.ok.d")).toBe(true);
    expect([...keys].some((k) => k.includes("a-b"))).toBe(false);
  });
});

describe("cueCompletionSource", () => {
  const source = cueCompletionSource(() => ({ keys: ["diagram", "diagram.nodes"], meta }));

  it("offers keys, builtins, and package members", () => {
    const result = source(fakeCtx("diagram.no")) as CompletionResult | null;
    expect(result).not.toBeNull();
    const labels = result!.options.map((o) => o.label);
    expect(labels).toContain("diagram.nodes");
    expect(labels).toContain("len");
    expect(labels).toContain("strings.ToUpper");
    // from is the start of the dotted token, so the whole reference is replaced.
    expect(result!.from).toBe(0);
  });

  it("returns null for an empty implicit token", () => {
    expect(source(fakeCtx("", false))).toBeNull();
  });

  it("offers completions for an empty token when explicitly invoked", () => {
    expect(source(fakeCtx("", true))).not.toBeNull();
  });
});
