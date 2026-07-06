// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// The "x-ray" decoration layer for the CUE editor: renders diagnostics and inlay
// hints as ghost text that lives in the editor but never in the document.
//
// One StateField holds every decoration; two producers feed it (eval diagnostics
// and type/optional hints), both as RawAnnotation records keyed by 1-based line.
// Offsets are derived from the live doc inside the field so a stale annotation can
// never point past a now-shorter document. The editor panel is dark in both app
// themes, so colors are fixed hex, matching cueLanguage.ts.

import { StateEffect, StateField, type Extension, type Range, type Text } from "@codemirror/state";
import { Decoration, type DecorationSet, EditorView, WidgetType } from "@codemirror/view";

export type AnnotationVariant = "error" | "warning" | "type" | "optional";

// A position-tagged annotation. column is 1-based; the ghost text always renders at
// end of line. The diagnostic underline itself is drawn by @codemirror/lint (in
// CodeEditor), which also owns the gutter markers and hover tooltips.
export interface RawAnnotation {
  line: number;
  column: number;
  text: string;
  variant: AnnotationVariant;
}

// setAnnotations replaces the whole annotation set (eval results are absolute,
// never incremental).
export const setAnnotations = StateEffect.define<RawAnnotation[]>();

// Ghost text appended after a line. Inert: it takes no events and is hidden from
// assistive tech (the real diagnostics live in the error panel and the graph).
class GhostWidget extends WidgetType {
  readonly text: string;
  readonly variant: AnnotationVariant;
  constructor(text: string, variant: AnnotationVariant) {
    super();
    this.text = text;
    this.variant = variant;
  }
  eq(other: GhostWidget) {
    return other.text === this.text && other.variant === this.variant;
  }
  toDOM() {
    const span = document.createElement("span");
    span.className = `cm-xray cm-xray-${this.variant}`;
    span.textContent = this.text;
    // The visible text is truncated with an ellipsis; keep the full label reachable
    // on hover.
    span.title = this.text;
    span.setAttribute("aria-hidden", "true");
    return span;
  }
  ignoreEvent() {
    return true;
  }
}

const lineDeco: Record<"error" | "warning", Decoration> = {
  error: Decoration.line({ class: "cm-xray-line-error" }),
  warning: Decoration.line({ class: "cm-xray-line-warning" }),
};

// Exported for unit testing: pure (RawAnnotation[] + doc -> DecorationSet), it
// never touches the DOM, so it runs under the node test env. The underline is drawn
// by @codemirror/lint, not here - this layer owns the line tint and the ghost text.
export function buildDeco(annotations: RawAnnotation[], doc: Text): DecorationSet {
  const ranges: Range<Decoration>[] = [];
  for (const a of annotations) {
    if (a.line < 1 || a.line > doc.lines) continue;
    const line = doc.line(a.line);
    if (a.variant === "error" || a.variant === "warning") {
      ranges.push(lineDeco[a.variant].range(line.from));
    }
    ranges.push(
      Decoration.widget({ widget: new GhostWidget(a.text, a.variant), side: 1 }).range(line.to),
    );
  }
  // sort=true: mixed line/widget decorations must be in document order.
  return Decoration.set(ranges, true);
}

const annotationsField = StateField.define<DecorationSet>({
  create() {
    return Decoration.none;
  },
  update(deco, tr) {
    // Map through edits so annotations track the text until the next eval replaces
    // them; a setAnnotations effect always wins and rebuilds from the fresh doc.
    deco = deco.map(tr.changes);
    for (const effect of tr.effects) {
      if (effect.is(setAnnotations)) {
        deco = buildDeco(effect.value, tr.state.doc);
      }
    }
    return deco;
  },
  provide: (field) => EditorView.decorations.from(field),
});

const annotationTheme = EditorView.baseTheme({
  ".cm-xray": {
    display: "inline-block",
    verticalAlign: "bottom",
    maxWidth: "40ch",
    overflow: "hidden",
    textOverflow: "ellipsis",
    marginLeft: "1.5ch",
    fontStyle: "italic",
    whiteSpace: "pre",
  },
  ".cm-xray-type": { color: "#64748b" },
  ".cm-xray-optional": { color: "#475569" },
  ".cm-xray-error": { color: "#f87171" },
  ".cm-xray-warning": { color: "#fbbf24" },
  ".cm-xray-line-error": { backgroundColor: "rgba(248, 113, 113, 0.08)" },
  ".cm-xray-line-warning": { backgroundColor: "rgba(251, 191, 36, 0.08)" },
});

export function editorAnnotations(): Extension {
  return [annotationsField, annotationTheme];
}
