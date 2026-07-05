// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Autocomplete for the CUE REPL. Candidates come from three sources: the field
// paths of the live diagram (so a query can drill into `diagram.nodes.x.owner`),
// the no-import CUE builtins, and every importable standard-library package with
// its members. The backend injects the needed imports, so a member reference like
// `strings.ToUpper` can be typed directly - no `import` line in the REPL.

import type {
  Completion,
  CompletionContext,
  CompletionResult,
  CompletionSource,
} from "@codemirror/autocomplete";
import type { CueMeta } from "./api";

// A CUE bare identifier. Non-identifier keys (addressed in CUE with quoted/index
// syntax) are skipped when flattening paths, since completion inserts bare text.
const IDENT = /^[A-Za-z_]\w*$/;

// CUE keywords and primitive types worth completing at an expression position.
const KEYWORDS = [
  "for",
  "in",
  "if",
  "let",
  "true",
  "false",
  "null",
  "string",
  "bytes",
  "int",
  "float",
  "number",
  "bool",
];

// walkKeys flattens a JSON value into dotted, identifier-only field paths under
// root (e.g. "diagram.nodes.a.owner"). Arrays and non-identifier keys are not
// descended: CUE addresses them with index/quoted syntax the UI does not build.
// Depth is bounded so a deep or cyclic-looking structure cannot blow up.
export function walkKeys(
  root: string,
  value: unknown,
  out: Set<string> = new Set(),
  depth = 0,
): Set<string> {
  out.add(root);
  if (depth >= 8 || value === null || typeof value !== "object" || Array.isArray(value)) {
    return out;
  }
  for (const [key, child] of Object.entries(value as Record<string, unknown>)) {
    if (!IDENT.test(key)) continue;
    walkKeys(`${root}.${key}`, child, out, depth + 1);
  }
  return out;
}

// The data a completion draws on: the diagram key paths (from the last successful
// eval) and the static CUE reference (fetched once). Either may be empty until it
// loads; completion degrades gracefully.
export interface ReplCompletionData {
  keys: string[];
  meta: CueMeta | null;
}

// buildOptions is memoized on its two inputs by reference: keys is replaced
// wholesale on each refresh and meta is fetched once, so identity comparison is a
// correct and cheap cache key.
let cacheKeys: string[] | null = null;
let cacheMeta: CueMeta | null = null;
let cacheOptions: Completion[] = [];

function buildOptions(keys: string[], meta: CueMeta | null): Completion[] {
  if (keys === cacheKeys && meta === cacheMeta) return cacheOptions;

  const options: Completion[] = [];
  // Diagram keys are the point of the tool, so boost them above the stdlib.
  for (const key of keys) {
    options.push({ label: key, type: "property", detail: "key", boost: 2 });
  }
  for (const word of KEYWORDS) {
    options.push({ label: word, type: "keyword" });
  }
  if (meta) {
    for (const builtin of meta.builtins) {
      options.push({ label: builtin.name, type: "function", detail: "builtin", boost: 1 });
    }
    for (const pkg of meta.packages) {
      options.push({ label: pkg.name, type: "namespace", detail: "package" });
      for (const member of pkg.members) {
        options.push({
          label: `${pkg.name}.${member.name}`,
          type: member.isFunc ? "function" : "variable",
          detail: pkg.name,
        });
      }
    }
  }

  cacheKeys = keys;
  cacheMeta = meta;
  cacheOptions = options;
  return options;
}

// cueCompletionSource returns a CodeMirror completion source over the current
// data (read lazily via get, so it always sees the latest keys/meta). It matches
// the dotted identifier before the cursor, so `strings.To` narrows to
// strings.ToUpper and `diagram.no` to diagram.nodes. validFor keeps the popup
// filtering as the user types more of the same token without re-querying.
export function cueCompletionSource(get: () => ReplCompletionData): CompletionSource {
  return (context: CompletionContext): CompletionResult | null => {
    const token = context.matchBefore(/[\w.]*/);
    if (!token) return null;
    if (token.from === token.to && !context.explicit) return null;
    const { keys, meta } = get();
    const options = buildOptions(keys, meta);
    if (options.length === 0) return null;
    return { from: token.from, options, validFor: /^[\w.]*$/ };
  };
}
