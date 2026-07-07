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
import { autocompletion } from "@codemirror/autocomplete";
import { lintGutter, setDiagnostics, type Diagnostic as LintDiagnostic } from "@codemirror/lint";
import { syntaxHighlighting } from "@codemirror/language";
import { basicSetup } from "codemirror";
import { cueMode, cueHighlightStyle, cueLightHighlightStyle } from "../cueLanguage";
import { useTheme, type Theme } from "../composables/useTheme";
import { editorAnnotations, setAnnotations, type RawAnnotation } from "../editorAnnotations";
import { editorFocus, setFocusRange } from "../editorFocus";
import { findElementRange } from "../cueSourceMap";
import { cueCompletionSource } from "../replCompletions";
import { useCueCompletion } from "../composables/useCueCompletion";
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
const emit = defineEmits<{
  "update:modelValue": [value: string];
  save: [];
  cursor: [pos: { line: number; col: number }];
}>();

// Autocomplete over the diagram field paths, CUE builtins, and stdlib packages,
// shared with the REPL. Only the editable editor completes (not the schema view).
const { completionData, start } = useCueCompletion();

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
  for (const h of props.showHints === false ? [] : (props.hints ?? [])) {
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

// Map eval diagnostics to @codemirror/lint diagnostics: an underline from the
// reported column to end of line, which also drives the gutter marker and the hover
// tooltip. Positioned line 0 diagnostics are skipped (they still show in the strip).
function toLintDiagnostics(): LintDiagnostic[] {
  if (!view) return [];
  const doc = view.state.doc;
  const out: LintDiagnostic[] = [];
  for (const d of props.diagnostics ?? []) {
    if (!d.line || d.line > doc.lines) continue;
    const line = doc.line(d.line);
    const from = Math.min(line.from + Math.max(0, (d.column || 1) - 1), line.to);
    out.push({
      from,
      to: line.to,
      severity: d.kind === "incomplete" ? "warning" : "error",
      message: d.message,
    });
  }
  return out;
}

function pushDiagnostics() {
  if (!view || props.readOnly) return;
  view.dispatch(setDiagnostics(view.state, toLintDiagnostics()));
}

// Tint the selected node's CUE block. Located by scanning the live doc, so it
// works regardless of how the text got there; an id with no block (renamed /
// broken CUE) just clears the tint. `scroll` only on a selection change - not when
// a graph edit regenerated the doc, so editing never yanks the viewport.
function pushFocus(scroll = false) {
  if (!view) return;
  const range = props.focusId ? findElementRange(view.state.doc.toString(), props.focusId) : null;
  const effects: StateEffect<unknown>[] = [setFocusRange.of(range)];
  if (range && scroll) effects.push(EditorView.scrollIntoView(range.from, { y: "center" }));
  view.dispatch({ effects });
}

const host = ref<HTMLDivElement>();
let view: EditorView | undefined;
const readOnlyCompartment = new Compartment();
// Set while pushing an external value into the editor, so the resulting doc
// change doesn't echo back out as a user edit.
let applyingExternal = false;

// The pane owns the background; the editor is transparent over it. Colors are
// fixed hex (CodeMirror themes are JS objects, not Tailwind classes). Two themes
// are defined because the pane is toggleable (useTheme); the `{ dark }` flag keeps
// the cursor/selection layers legible on the matching background.
const darkTheme = EditorView.theme(
  {
    "&": { height: "100%", backgroundColor: "transparent", color: "#e2e8f0" },
    ".cm-scroller": {
      fontFamily: "'JetBrains Mono', ui-monospace, Consolas, monospace",
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
    ".cm-activeLine": { backgroundColor: "rgba(148, 163, 184, 0.08)" },
    ".cm-activeLineGutter": { backgroundColor: "rgba(148, 163, 184, 0.08)" },
    "&.cm-focused": { outline: "none" },
    "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": {
      backgroundColor: "rgba(148, 163, 184, 0.3)",
    },
    // Search panel (⌘F, from basicSetup). Themed to match the dark pane instead of
    // CodeMirror's default light chrome.
    ".cm-panels": { backgroundColor: "#0f172a", color: "#e2e8f0" },
    ".cm-panels.cm-panels-bottom": { borderTop: "1px solid #1e293b" },
    ".cm-panels.cm-panels-top": { borderBottom: "1px solid #1e293b" },
    ".cm-panel.cm-search": {
      backgroundColor: "#0f172a",
      padding: "6px 8px",
      fontFamily: "'JetBrains Mono', ui-monospace, Consolas, monospace",
      fontSize: "12px",
    },
    ".cm-panel.cm-search label": { color: "#94a3b8" },
    ".cm-panel.cm-search input": {
      backgroundColor: "#1e293b",
      color: "#e2e8f0",
      border: "1px solid #334155",
      borderRadius: "4px",
      padding: "2px 6px",
    },
    ".cm-panel.cm-search input:focus": { outline: "none", borderColor: "#f59e0b" },
    ".cm-panel.cm-search button": {
      backgroundColor: "#1e293b",
      color: "#cbd5e1",
      border: "1px solid #334155",
      borderRadius: "4px",
      backgroundImage: "none",
    },
    ".cm-panel.cm-search button:hover": { backgroundColor: "#334155" },
    ".cm-panel.cm-search .cm-button[name='close'], .cm-panel.cm-search button[name='close']": {
      color: "#94a3b8",
    },
    ".cm-searchMatch": { backgroundColor: "rgba(245, 158, 11, 0.25)" },
    ".cm-searchMatch-selected": { backgroundColor: "rgba(245, 158, 11, 0.55)" },
  },
  { dark: true },
);

// Light-pane counterpart: dark ink on the pane's white background, same amber
// accent for search matches and focus.
const lightTheme = EditorView.theme(
  {
    "&": { height: "100%", backgroundColor: "transparent", color: "#1e293b" },
    ".cm-scroller": {
      fontFamily: "'JetBrains Mono', ui-monospace, Consolas, monospace",
      fontSize: "13px",
      lineHeight: "20px",
    },
    ".cm-content": { caretColor: "#1e293b" },
    ".cm-cursor, .cm-dropCursor": {
      borderLeftColor: "#1e293b",
      borderLeftWidth: "2px",
    },
    ".cm-gutters": {
      backgroundColor: "transparent",
      color: "#94a3b8",
      border: "none",
    },
    ".cm-activeLine": { backgroundColor: "rgba(148, 163, 184, 0.12)" },
    ".cm-activeLineGutter": { backgroundColor: "rgba(148, 163, 184, 0.12)" },
    "&.cm-focused": { outline: "none" },
    "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": {
      backgroundColor: "rgba(148, 163, 184, 0.35)",
    },
    ".cm-panels": { backgroundColor: "#f1f5f9", color: "#1e293b" },
    ".cm-panels.cm-panels-bottom": { borderTop: "1px solid #e2e8f0" },
    ".cm-panels.cm-panels-top": { borderBottom: "1px solid #e2e8f0" },
    ".cm-panel.cm-search": {
      backgroundColor: "#f1f5f9",
      padding: "6px 8px",
      fontFamily: "'JetBrains Mono', ui-monospace, Consolas, monospace",
      fontSize: "12px",
    },
    ".cm-panel.cm-search label": { color: "#475569" },
    ".cm-panel.cm-search input": {
      backgroundColor: "#ffffff",
      color: "#1e293b",
      border: "1px solid #cbd5e1",
      borderRadius: "4px",
      padding: "2px 6px",
    },
    ".cm-panel.cm-search input:focus": { outline: "none", borderColor: "#f59e0b" },
    ".cm-panel.cm-search button": {
      backgroundColor: "#ffffff",
      color: "#334155",
      border: "1px solid #cbd5e1",
      borderRadius: "4px",
      backgroundImage: "none",
    },
    ".cm-panel.cm-search button:hover": { backgroundColor: "#e2e8f0" },
    ".cm-panel.cm-search .cm-button[name='close'], .cm-panel.cm-search button[name='close']": {
      color: "#475569",
    },
    ".cm-searchMatch": { backgroundColor: "rgba(245, 158, 11, 0.25)" },
    ".cm-searchMatch-selected": { backgroundColor: "rgba(245, 158, 11, 0.55)" },
  },
  { dark: false },
);

// Theme lives in a compartment so a toggle can swap the editor chrome and the CUE
// token colors together without rebuilding the view.
const { theme: paneTheme } = useTheme();
const themeCompartment = new Compartment();
function themeExtension(t: Theme) {
  return t === "dark"
    ? [darkTheme, syntaxHighlighting(cueHighlightStyle)]
    : [lightTheme, syntaxHighlighting(cueLightHighlightStyle)];
}

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
        cueMode,
        ...(props.readOnly
          ? []
          : [autocompletion({ override: [cueCompletionSource(completionData)] }), lintGutter()]),
        editorAnnotations(),
        editorFocus(),
        themeCompartment.of(themeExtension(paneTheme.value)),
        EditorState.tabSize.of(2),
        readOnlyCompartment.of(EditorState.readOnly.of(!!props.readOnly)),
        EditorView.updateListener.of((update) => {
          if (update.docChanged && !applyingExternal) {
            emit("update:modelValue", update.state.doc.toString());
          }
          if ((update.docChanged || update.selectionSet) && !props.readOnly) {
            const head = update.state.selection.main.head;
            const line = update.state.doc.lineAt(head);
            emit("cursor", { line: line.number, col: head - line.from + 1 });
          }
        }),
      ],
    }),
  });
  pushAnnotations();
  pushDiagnostics();
  pushFocus(true);
  if (!props.readOnly) start();
});

// Move the caret to line:col, scroll it into view, and focus - the status bar's and
// problems strip's jump-to-problem. 1-based line and column.
function revealLine(line: number, col = 1) {
  if (!view) return;
  const target = Math.max(1, Math.min(line, view.state.doc.lines));
  const lineObj = view.state.doc.line(target);
  const pos = Math.min(lineObj.from + Math.max(0, col - 1), lineObj.to);
  view.dispatch({ selection: { anchor: pos }, scrollIntoView: true });
  view.focus();
}
defineExpose({ revealLine });

// Re-render the x-ray whenever eval produces new diagnostics or hints. Not a doc
// change, so it never echoes back through the update listener.
watch(
  () => [props.diagnostics, props.hints, props.showHints],
  () => {
    pushAnnotations();
    pushDiagnostics();
  },
);

// Re-tint and scroll whenever the canvas selection changes.
watch(
  () => props.focusId,
  () => pushFocus(true),
);

// Swap the editor chrome and token colors when the pane theme toggles.
watch(paneTheme, (value) => {
  view?.dispatch({ effects: themeCompartment.reconfigure(themeExtension(value)) });
});

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
      effects: readOnlyCompartment.reconfigure(EditorState.readOnly.of(!!value)),
    });
  },
);

onBeforeUnmount(() => view?.destroy());
</script>

<template>
  <div ref="host" class="h-full overflow-hidden" />
</template>
