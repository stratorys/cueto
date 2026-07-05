// Canvas -> code focus: tint the CUE block of the node selected on the canvas.
// Mirrors editorAnnotations.ts - one StateField of line decorations, fed by a
// single StateEffect. The tint maps through edits so it tracks the text until the
// next selection (or a clear) replaces it.

import { StateEffect, StateField, type Extension, type Text } from "@codemirror/state";
import { Decoration, type DecorationSet, EditorView } from "@codemirror/view";

// Character range [from, to) of the block to tint, or null to clear.
export type FocusRange = { from: number; to: number } | null;

export const setFocusRange = StateEffect.define<FocusRange>();

const focusLine = Decoration.line({ class: "cm-focus-line" });

// One full-line decoration for every line the range overlaps, clamped to the doc.
function buildDeco(range: FocusRange, doc: Text): DecorationSet {
  if (!range) return Decoration.none;
  const from = Math.max(0, Math.min(range.from, doc.length));
  const to = Math.max(from, Math.min(range.to, doc.length));
  const firstLine = doc.lineAt(from).number;
  const lastLine = doc.lineAt(to).number;
  const ranges = [];
  for (let n = firstLine; n <= lastLine; n++) {
    ranges.push(focusLine.range(doc.line(n).from));
  }
  return Decoration.set(ranges);
}

const focusField = StateField.define<DecorationSet>({
  create() {
    return Decoration.none;
  },
  update(deco, tr) {
    // Track the text through edits; a setFocusRange effect always wins and
    // rebuilds from the fresh doc.
    deco = deco.map(tr.changes);
    for (const effect of tr.effects) {
      if (effect.is(setFocusRange)) {
        deco = buildDeco(effect.value, tr.state.doc);
      }
    }
    return deco;
  },
  provide: (field) => EditorView.decorations.from(field),
});

const focusTheme = EditorView.baseTheme({
  ".cm-focus-line": { backgroundColor: "rgba(245, 158, 11, 0.12)" },
});

export function editorFocus(): Extension {
  return [focusField, focusTheme];
}
