// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// A minimal CUE language mode for CodeMirror 6.
// It is a lexer, not a parser - enough to color the two files this app shows
// (generated data.cue, hand-owned schema.cue). Token colors are fixed hex values
// (CodeMirror themes are JS objects, not Tailwind classes); a dark and a light
// style are provided because the editor pane is theme-toggleable (the REPL, which
// reuses cueLanguage(), stays dark).

import { StreamLanguage, HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { tags } from "@lezer/highlight";
import type { Extension } from "@codemirror/state";

const KEYWORD = /^(?:package|import|true|false|null|for|in|if|let)\b/;

export const cueMode = StreamLanguage.define<Record<string, never>>({
  name: "cue",
  token(stream) {
    if (stream.eatSpace()) return null;

    if (stream.match("//")) {
      stream.skipToEnd();
      return "cueComment";
    }
    if (stream.match(/^"(?:[^"\\]|\\.)*"/)) return "cueString";
    if (stream.match(/^#[A-Za-z_]\w*/)) return "cueDef";
    if (stream.match(KEYWORD)) return "cueKeyword";
    if (stream.match(/^\d+(?:\.\d+)?/)) return "cueNumber";

    // An identifier immediately followed by an optional `?` then `:` is a field
    // key; anything else is a bare value/reference.
    if (stream.match(/^[A-Za-z_]\w*/)) {
      const rest = stream.string.slice(stream.pos);
      return /^\??\s*:/.test(rest) ? "cueKey" : "cueVar";
    }
    if (stream.match(/^[{}[\],:|&?()]/)) return "cuePunct";

    stream.next();
    return null;
  },
  tokenTable: {
    cueComment: tags.comment,
    cueString: tags.string,
    cueDef: tags.typeName,
    cueKeyword: tags.keyword,
    cueNumber: tags.number,
    cueKey: tags.propertyName,
    cueVar: tags.variableName,
    cuePunct: tags.punctuation,
  },
});

// Dark-pane token colors (bright hues on a slate-900 background).
export const cueHighlightStyle = HighlightStyle.define([
  { tag: tags.comment, color: "#64748b", fontStyle: "italic" },
  { tag: tags.string, color: "#86efac" },
  { tag: tags.typeName, color: "#d97706", fontWeight: "600" },
  { tag: tags.keyword, color: "#c4b5fd" },
  { tag: tags.number, color: "#fca5a5" },
  { tag: tags.propertyName, color: "#93c5fd" },
  { tag: tags.variableName, color: "#e2e8f0" },
  { tag: tags.punctuation, color: "#64748b" },
]);

// Light-pane token colors (saturated 600/700 hues readable on white).
export const cueLightHighlightStyle = HighlightStyle.define([
  { tag: tags.comment, color: "#64748b", fontStyle: "italic" },
  { tag: tags.string, color: "#15803d" },
  { tag: tags.typeName, color: "#b45309", fontWeight: "600" },
  { tag: tags.keyword, color: "#7c3aed" },
  { tag: tags.number, color: "#dc2626" },
  { tag: tags.propertyName, color: "#2563eb" },
  { tag: tags.variableName, color: "#1e293b" },
  { tag: tags.punctuation, color: "#64748b" },
]);

// The lexer plus the dark highlight style. Used by the REPL input, which stays on a
// dark pane; the code editor composes the mode with a theme-swappable style itself.
export function cueLanguage(): Extension {
  return [cueMode, syntaxHighlighting(cueHighlightStyle)];
}
