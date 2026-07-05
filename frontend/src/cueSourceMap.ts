// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Locate an element's CUE block by scanning the current editor text (not a
// model-derived source map), so it stays correct after hand-edits, `Format`, or
// loading a saved version. A node is emitted by toCue() as a struct keyed by its
// id under `nodes:` (`<id>: { ... }`); an edge is an object in the `edges:` array
// carrying an `id: "<id>"` field.

// A bare key if it is a plain identifier, otherwise a quoted string key. Mirrors
// cueKey() in mapping.ts so the scan matches exactly how toCue() emits the key.
function cueKey(id: string): string {
  return /^[a-zA-Z_]\w*$/.test(id) ? id : JSON.stringify(id);
}

export type Range = { from: number; to: number };

// Character offsets [from, to) of the CUE block for `id`, whether it is a node
// (a `<id>: { ... }` struct key) or an edge (a `{ ... }` object carrying an
// `id: "<id>"` field). Null when neither exists.
export function findElementRange(cueText: string, id: string): Range | null {
  return findNodeRange(cueText, id) ?? findEdgeRange(cueText, id);
}

// Character offsets [from, to) of the `<id>: { ... }` block in `cueText`, or null
// when no such block exists. Brace matching skips braces inside double-quoted
// strings so a label like "a } b" never closes the block early.
export function findNodeRange(cueText: string, id: string): Range | null {
  const key = cueKey(id);
  const lines = cueText.split("\n");
  let lineStart = 0;
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const trimmed = line.trimStart();
    // The key must be immediately followed by `:` then `{` (an opening struct),
    // which excludes an edge's `id: "..."` field and any longer key with this
    // one as a prefix.
    if (trimmed.startsWith(key) && /^:\s*\{/.test(trimmed.slice(key.length))) {
      const to = matchBrace(lines, i, lineStart);
      if (to !== null) return { from: lineStart, to };
    }
    lineStart += line.length + 1; // +1 for the removed "\n"
  }
  return null;
}

// Character offsets [from, to) of the edge object carrying `id: "<id>"`, or null.
// The id is the first field toCue() emits per edge, so the object's opening `{`
// is the nearest brace-opening line above it.
export function findEdgeRange(cueText: string, id: string): Range | null {
  const target = `id: ${JSON.stringify(id)}`;
  const lines = cueText.split("\n");
  const idLine = lines.findIndex((line) => line.trim() === target);
  if (idLine < 0) return null;
  let open = -1;
  for (let i = idLine; i >= 0; i--) {
    if (lines[i].trimEnd().endsWith("{")) {
      open = i;
      break;
    }
  }
  if (open < 0) return null;
  let offset = 0;
  for (let i = 0; i < open; i++) offset += lines[i].length + 1;
  const to = matchBrace(lines, open, offset);
  return to === null ? null : { from: offset, to };
}

// End offset (exclusive) of the struct opened on line `startLine`, found by
// counting braces from that line while ignoring string contents. Null if the
// braces never balance.
function matchBrace(lines: string[], startLine: number, startOffset: number): number | null {
  let depth = 0;
  let opened = false;
  let inString = false;
  let escaped = false;
  let offset = startOffset;
  for (let i = startLine; i < lines.length; i++) {
    const line = lines[i];
    for (let k = 0; k < line.length; k++) {
      const c = line[k];
      if (inString) {
        if (escaped) escaped = false;
        else if (c === "\\") escaped = true;
        else if (c === '"') inString = false;
        continue;
      }
      if (c === '"') inString = true;
      else if (c === "{") {
        depth++;
        opened = true;
      } else if (c === "}") {
        depth--;
        if (opened && depth === 0) return offset + k + 1;
      }
    }
    offset += line.length + 1; // +1 for the "\n" between lines
  }
  return null;
}
