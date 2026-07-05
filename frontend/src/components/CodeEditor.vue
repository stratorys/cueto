<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import { Compartment, EditorState, type StateEffect } from "@codemirror/state";
import { EditorView, keymap } from "@codemirror/view";
import { indentWithTab } from "@codemirror/commands";
import { basicSetup } from "codemirror";
import { cueLanguage } from "../cueLanguage";
import { editorAnnotations, setAnnotations, type RawAnnotation } from "../editorAnnotations";
import { editorFocus, setFocusRange } from "../editorFocus";
import { findElementRange } from "../cueSourceMap";
import type { Diagnostic, Hint } from "../api";

const props = defineProps<{
  modelValue: string;
  readOnly?: boolean;
  diagnostics?: Diagnostic[];
  hints?: Hint[];
  // Draw inlay hints when true (default). Diagnostics are shown regardless.
  showHints?: boolean;
  // Id of the node or edge selected on the canvas: its CUE block is tinted and
  // scrolled into view. null clears the tint.
  focusId?: string | null;
}>();
const emit = defineEmits<{ "update:modelValue": [value: string]; save: [] }>();

// Fold diagnostics and hints into the editor's single annotation stream. A
// diagnostic without a position (line 0) is skipped here; it still shows in the
// error panel. Incomplete/missing values read as warnings, everything else as
// errors.
function toAnnotations(): RawAnnotation[] {
  const out: RawAnnotation[] = [];
  for (const d of props.diagnostics ?? []) {
    if (!d.line) continue;
    out.push({
      line: d.line,
      column: d.column || 1,
      text: d.message,
      variant: d.kind === "incomplete" ? "warning" : "error",
    });
  }
  for (const h of props.showHints === false ? [] : props.hints ?? []) {
    if (!h.line) continue;
    out.push({
      line: h.line,
      column: h.column || 1,
      text: h.kind === "optional" ? `+ ${h.label}` : `: ${h.label}`,
      variant: h.kind === "optional" ? "optional" : "type",
    });
  }
  return out;
}

function pushAnnotations() {
  view?.dispatch({ effects: setAnnotations.of(toAnnotations()) });
}

// Tint the selected node's CUE block. Located by scanning the live doc, so it
// works regardless of how the text got there; an id with no block (renamed /
// broken CUE) just clears the tint. `scroll` only on a selection change - not when
// a graph edit regenerated the doc, so editing never yanks the viewport.
function pushFocus(scroll = false) {
  if (!view) return;
  const range = props.focusId
    ? findElementRange(view.state.doc.toString(), props.focusId)
    : null;
  const effects: StateEffect<unknown>[] = [setFocusRange.of(range)];
  if (range && scroll) effects.push(EditorView.scrollIntoView(range.from, { y: "center" }));
  view.dispatch({ effects });
}

const host = ref<HTMLDivElement>();
let view: EditorView | undefined;
const readOnly = new Compartment();
// Set while pushing an external value into the editor, so the resulting doc
// change doesn't echo back out as a user edit.
let applyingExternal = false;

// The pane owns the background; the editor is transparent over it. Colors are
// fixed hex (CodeMirror themes are JS objects, not Tailwind classes). The code
// pane is always dark (the rest of the app is light), so { dark: true } is
// passed below - otherwise the cursor and selection layers render invisible on
// the dark background.
const theme = EditorView.theme(
  {
    "&": { height: "100%", backgroundColor: "transparent", color: "#e2e8f0" },
    ".cm-scroller": {
      fontFamily: "ui-monospace, Consolas, monospace",
      // Integer px only: a fractional font size makes CodeMirror's average
      // char-width measurement diverge from each glyph's rounded advance on
      // HiDPI, so the caret drifts further from the text the longer the line.
      fontSize: "13px",
      lineHeight: "20px",
    },
    ".cm-content": { caretColor: "#e2e8f0" },
    ".cm-cursor, .cm-dropCursor": {
      borderLeftColor: "#e2e8f0",
      borderLeftWidth: "2px",
    },
    ".cm-gutters": {
      backgroundColor: "transparent",
      color: "#334155",
      border: "none",
    },
    ".cm-activeLine": { backgroundColor: "transparent" },
    ".cm-activeLineGutter": { backgroundColor: "transparent" },
    "&.cm-focused": { outline: "none" },
    "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": {
      backgroundColor: "rgba(148, 163, 184, 0.3)",
    },
  },
  { dark: true },
);

onMounted(() => {
  view = new EditorView({
    parent: host.value,
    state: EditorState.create({
      doc: props.modelValue,
      extensions: [
        basicSetup,
        // Editor-style Tab: indent instead of moving focus out (basicSetup omits
        // this by default). Cmd/Ctrl+S saves rather than falling through to the
        // browser's save-page dialog.
        keymap.of([
          indentWithTab,
          { key: "Mod-s", preventDefault: true, run: () => (emit("save"), true) },
        ]),
        cueLanguage(),
        editorAnnotations(),
        editorFocus(),
        theme,
        EditorState.tabSize.of(2),
        readOnly.of(EditorState.readOnly.of(!!props.readOnly)),
        EditorView.updateListener.of((update) => {
          if (update.docChanged && !applyingExternal) {
            emit("update:modelValue", update.state.doc.toString());
          }
        }),
      ],
    }),
  });
  pushAnnotations();
  pushFocus(true);
});

// Re-render the x-ray whenever eval produces new diagnostics or hints. Not a doc
// change, so it never echoes back through the update listener.
watch(() => [props.diagnostics, props.hints, props.showHints], pushAnnotations);

// Re-tint and scroll whenever the canvas selection changes.
watch(() => props.focusId, () => pushFocus(true));

// External value change (e.g. a graph edit regenerated the CUE): replace the doc
// without re-emitting.
watch(
  () => props.modelValue,
  (value) => {
    if (!view || value === view.state.doc.toString()) return;
    applyingExternal = true;
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: value },
    });
    applyingExternal = false;
    // A full-doc replace can't map the old tint onto the new text; re-locate the
    // selected node's block in the regenerated CUE.
    pushFocus();
  },
);

watch(
  () => props.readOnly,
  (value) => {
    view?.dispatch({
      effects: readOnly.reconfigure(EditorState.readOnly.of(!!value)),
    });
  },
);

onBeforeUnmount(() => view?.destroy());
</script>

<template>
  <div ref="host" class="h-full overflow-hidden" />
</template>
