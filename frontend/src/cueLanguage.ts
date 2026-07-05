// A minimal CUE language mode for CodeMirror 6.
// It is a lexer, not a parser - enough to color the two files this app shows
// (generated data.cue, hand-owned schema.cue). The editor panel is dark in both
// app themes, so the token colors are fixed hex values (CodeMirror themes are JS
// objects, not Tailwind classes).

import { StreamLanguage, HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { tags } from "@lezer/highlight";
import type { Extension } from "@codemirror/state";

const KEYWORD = /^(?:package|import|true|false|null|for|in|if|let)\b/;

const cueMode = StreamLanguage.define<Record<string, never>>({
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

const cueHighlightStyle = HighlightStyle.define([
  { tag: tags.comment, color: "#64748b", fontStyle: "italic" },
  { tag: tags.string, color: "#86efac" },
  { tag: tags.typeName, color: "#d97706", fontWeight: "600" },
  { tag: tags.keyword, color: "#c4b5fd" },
  { tag: tags.number, color: "#fca5a5" },
  { tag: tags.propertyName, color: "#93c5fd" },
  { tag: tags.variableName, color: "#e2e8f0" },
  { tag: tags.punctuation, color: "#64748b" },
]);

export function cueLanguage(): Extension {
  return [cueMode, syntaxHighlighting(cueHighlightStyle)];
}
