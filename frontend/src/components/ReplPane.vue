<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// Ephemeral CUE REPL. Each entry evaluates a CUE expression against the diagram
// currently in the editor (the files are sent to /repl and overlaid on the
// schema), so a query can read the live `diagram` - e.g.
// `diagram.nodes.user.label`. The input is a CodeMirror editor with
// autocomplete over the diagram's field paths, the CUE builtins, and every
// importable standard-library package (the backend injects the needed imports).
// The reference browser lists the same. It has no saved impact: nothing here
// mutates the editor files, the diagram, the schema, or any saved version, and
// the scroll-back lives only in component state - gone on refresh.
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import { EditorState } from "@codemirror/state";
import { EditorView, keymap, placeholder } from "@codemirror/view";
import { defaultKeymap, history, historyKeymap, insertNewlineAndIndent } from "@codemirror/commands";
import {
  acceptCompletion,
  autocompletion,
  closeBrackets,
  closeBracketsKeymap,
  completionKeymap,
  startCompletion,
} from "@codemirror/autocomplete";
import { cueLanguage } from "../cueLanguage";
import { evalExpr, type CuePackage } from "../api";
import { files } from "../composables/useEditorFiles";
import { cueCompletionSource } from "../replCompletions";
import { useCueCompletion } from "../composables/useCueCompletion";

// collapsed is App's REPL-pane state; the pane stays mounted (height 0) when
// collapsed. Declared for the template contract; the completion data now tracks the
// diagram independently in useCueCompletion.
defineProps<{ collapsed?: boolean }>();

interface Entry {
  source: string;
  output: string;
  ok: boolean;
}

const entries = ref<Entry[]>([]);
const running = ref(false);
const logEl = ref<HTMLElement | null>(null);
const host = ref<HTMLDivElement>();

// Submitted expressions, oldest first, recalled with Up/Down. Kept separate from
// `entries` (the output log) so it stays a clean list of inputs. histIndex points
// into it while navigating, or -1 when editing a fresh line.
const submitted = ref<string[]>([]);
let histIndex = -1;

// Shared with the code editor: the static CUE reference and the live diagram key
// paths. Both feed autocomplete and the reference browser.
const { meta, keys, completionData, start } = useCueCompletion();

// Reference browser state.
const showRef = ref(false);
const refFilter = ref("");
const expanded = ref<Set<string>>(new Set());

let view: EditorView | undefined;

async function run() {
  if (running.value || !view) return;
  const text = view.state.doc.toString().trim();
  if (!text) return;
  running.value = true;
  // Record the input for Up/Down recall (skip a straight repeat of the last one).
  if (submitted.value[submitted.value.length - 1] !== text) submitted.value.push(text);
  histIndex = -1;
  const result = await evalExpr(text, files.value);
  entries.value.push({
    source: text,
    output: result.ok ? JSON.stringify(result.result, null, 2) : result.error,
    ok: result.ok,
  });
  view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: "" } });
  running.value = false;
  await nextTick();
  if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight;
}

// Replace the whole input and drop the caret at its end.
function setInput(text: string) {
  view?.dispatch({
    changes: { from: 0, to: view.state.doc.length, insert: text },
    selection: { anchor: text.length },
  });
}

// Up recalls the previous submission, but only when the caret is on the first line
// (so Up otherwise moves between lines of a multi-line expression).
function recallPrev(v: EditorView): boolean {
  if (v.state.doc.lineAt(v.state.selection.main.head).number !== 1) return false;
  if (!submitted.value.length) return false;
  if (histIndex === -1) histIndex = submitted.value.length;
  if (histIndex === 0) return true;
  histIndex--;
  setInput(submitted.value[histIndex]);
  return true;
}

// Down walks back toward the fresh empty line, only from the last line.
function recallNext(v: EditorView): boolean {
  const doc = v.state.doc;
  if (doc.lineAt(v.state.selection.main.head).number !== doc.lines) return false;
  if (histIndex === -1) return false;
  histIndex++;
  if (histIndex >= submitted.value.length) {
    histIndex = -1;
    setInput("");
  } else {
    setInput(submitted.value[histIndex]);
  }
  return true;
}

function clearLog() {
  entries.value = [];
}

// insert drops text at the cursor (replacing any selection) and refocuses the
// editor - the reference browser's click-to-insert.
function insert(text: string) {
  if (!view) return;
  const { from, to } = view.state.selection.main;
  view.dispatch({
    changes: { from, to, insert: text },
    selection: { anchor: from + text.length },
  });
  view.focus();
}

function togglePkg(name: string) {
  const next = new Set(expanded.value);
  if (next.has(name)) next.delete(name);
  else next.add(name);
  expanded.value = next;
}

const query = computed(() => refFilter.value.trim().toLowerCase());

const filteredKeys = computed(() =>
  keys.value.filter((k) => k.toLowerCase().includes(query.value)),
);
const filteredBuiltins = computed(() =>
  (meta.value?.builtins ?? []).filter((b) => b.name.toLowerCase().includes(query.value)),
);

// Each package with the members that match the filter, and whether it should show
// expanded: explicitly toggled, or auto-expanded because the filter hit a member.
const packageView = computed(() => {
  const q = query.value;
  const rows: { pkg: CuePackage; members: CuePackage["members"]; open: boolean }[] = [];
  for (const pkg of meta.value?.packages ?? []) {
    const nameMatch = pkg.name.toLowerCase().includes(q);
    const members = q
      ? pkg.members.filter((m) => `${pkg.name}.${m.name}`.toLowerCase().includes(q))
      : pkg.members;
    if (!nameMatch && members.length === 0) continue;
    const open = expanded.value.has(pkg.name) || (q !== "" && members.length > 0);
    rows.push({ pkg, members, open });
  }
  return rows;
});

onMounted(async () => {
  view = new EditorView({
    parent: host.value,
    state: EditorState.create({
      doc: "",
      extensions: [
        history(),
        closeBrackets(),
        cueLanguage(),
        placeholder("diagram.nodes.user.label"),
        autocompletion({ override: [cueCompletionSource(completionData)] }),
        keymap.of([
          { key: "Mod-Enter", preventDefault: true, run: () => (void run(), true) },
          { key: "Ctrl-l", preventDefault: true, run: () => (clearLog(), true) },
          { key: "Mod-k", preventDefault: true, run: () => (clearLog(), true) },
          { key: "Ctrl-Space", run: startCompletion },
          // Tab accepts the highlighted completion (as in VSCode / the devtools
          // console). acceptCompletion is a no-op when no popup is open, so Tab
          // then falls through to its normal behaviour.
          { key: "Tab", run: acceptCompletion },
          ...closeBracketsKeymap,
          // Completion keymap first, so Enter/Up/Down drive an open popup before
          // they run / recall history.
          ...completionKeymap,
          // Enter runs a single-line expression; a multi-line one falls through to
          // insert a newline. Shift-Enter always inserts a newline.
          {
            key: "Enter",
            run: (v) => {
              if (v.state.doc.lines > 1) return false;
              void run();
              return true;
            },
          },
          { key: "Shift-Enter", run: insertNewlineAndIndent },
          { key: "ArrowUp", run: recallPrev },
          { key: "ArrowDown", run: recallNext },
          ...historyKeymap,
          ...defaultKeymap,
        ]),
        EditorView.lineWrapping,
        replTheme,
      ],
    }),
  });
  start();
});

onBeforeUnmount(() => {
  view?.destroy();
});

// The REPL is a code surface: dark like the editor pane. Compact, wraps long lines,
// scrolls past a few lines rather than pushing the log away. Transparent over the
// dark pane; { dark: true } so the cursor/selection layers render on it.
const replTheme = EditorView.theme(
  {
    "&": { fontSize: "12px", backgroundColor: "transparent", color: "#e2e8f0" },
    ".cm-scroller": {
      fontFamily: "'JetBrains Mono', ui-monospace, Consolas, monospace",
      lineHeight: "18px",
      maxHeight: "120px",
    },
    ".cm-content": { padding: "6px 8px", caretColor: "#e2e8f0" },
    ".cm-cursor, .cm-dropCursor": { borderLeftColor: "#e2e8f0" },
    "&.cm-focused": { outline: "none" },
    "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": {
      backgroundColor: "rgba(148, 163, 184, 0.3)",
    },
    ".cm-placeholder": { color: "#64748b" },
  },
  { dark: true },
);
</script>

<template>
  <div class="flex h-full flex-col border-t border-slate-800 bg-slate-900 text-slate-200">
    <div
      class="flex flex-none items-center justify-between border-b border-slate-800 px-3 py-1.5 text-xs"
    >
      <span class="font-semibold text-slate-300">REPL</span>
      <div class="flex items-center gap-3 text-slate-500">
        <span class="font-mono">query the diagram · nothing saved</span>
        <button
          class="hover:text-amber-400"
          :class="{ 'text-amber-400': showRef }"
          @click="showRef = !showRef"
        >
          {{ showRef ? "log" : "browse" }}
        </button>
        <button v-if="entries.length && !showRef" class="hover:text-amber-400" @click="clearLog">
          clear
        </button>
      </div>
    </div>

    <!-- Reference browser: diagram keys, CUE builtins, importable packages. -->
    <div v-if="showRef" class="flex min-h-0 flex-1 flex-col">
      <div class="flex-none border-b border-slate-800 px-3 py-1.5">
        <input
          v-model="refFilter"
          spellcheck="false"
          placeholder="filter keys, builtins, functions…"
          class="w-full rounded border border-slate-700 bg-slate-800 px-2 py-1 font-mono text-xs text-slate-200 placeholder-slate-500 focus:border-amber-500 focus:outline-none"
        />
      </div>
      <div class="min-h-0 flex-1 overflow-y-auto px-3 py-2 text-xs">
        <section class="mb-3">
          <h4 class="mb-1 font-semibold uppercase tracking-wide text-slate-500">Keys</h4>
          <p v-if="!filteredKeys.length" class="text-slate-600">none</p>
          <ul class="space-y-0.5">
            <li v-for="k in filteredKeys" :key="k">
              <button
                class="font-mono text-teal-400 hover:underline"
                @click="insert(k)"
              >
                {{ k }}
              </button>
            </li>
          </ul>
        </section>

        <section class="mb-3">
          <h4 class="mb-1 font-semibold uppercase tracking-wide text-slate-500">Builtins</h4>
          <div class="flex flex-wrap gap-x-3 gap-y-0.5">
            <button
              v-for="b in filteredBuiltins"
              :key="b.name"
              class="font-mono text-amber-400 hover:underline"
              @click="insert(b.name)"
            >
              {{ b.name }}
            </button>
          </div>
        </section>

        <section>
          <h4 class="mb-1 font-semibold uppercase tracking-wide text-slate-500">Packages</h4>
          <p v-if="!packageView.length" class="text-slate-600">none</p>
          <div v-for="row in packageView" :key="row.pkg.path" class="mb-0.5">
            <button
              class="flex w-full items-center gap-1 text-left font-mono text-violet-400 hover:underline"
              @click="togglePkg(row.pkg.name)"
            >
              <span class="w-3 select-none text-slate-500">{{ row.open ? "▾" : "▸" }}</span>
              <span>{{ row.pkg.name }}</span>
              <span class="text-slate-600">{{ row.pkg.path }}</span>
            </button>
            <ul v-if="row.open" class="ml-4 mt-0.5 flex flex-wrap gap-x-3 gap-y-0.5">
              <li v-for="m in row.members" :key="m.name">
                <button
                  class="font-mono hover:underline"
                  :class="m.isFunc ? 'text-sky-400' : 'text-slate-400'"
                  @click="insert(`${row.pkg.name}.${m.name}`)"
                >
                  {{ m.name }}<span v-if="m.isFunc" class="text-slate-600">()</span>
                </button>
              </li>
            </ul>
          </div>
        </section>
      </div>
    </div>

    <!-- Scroll-back log. -->
    <div
      v-show="!showRef"
      ref="logEl"
      class="min-h-0 flex-1 space-y-2 overflow-y-auto px-3 py-2 font-mono text-xs"
    >
      <p v-if="!entries.length" class="text-slate-500">
        Query the diagram in the editor - e.g.
        <span class="font-mono text-slate-400">diagram.nodes.user.label</span> or
        <span class="font-mono text-slate-400">[for e in diagram.edges if e.kind == "arrow" {e.id}]</span>.
        Type to autocomplete keys, builtins, and package functions (⌃Space to force
        it); Enter runs, ⇧Enter adds a line, ↑/↓ recall history; ⌃L or ⌘K clears.
        Results never touch your files or saved versions.
      </p>
      <div v-for="(entry, i) in entries" :key="i" class="space-y-0.5">
        <pre class="whitespace-pre-wrap break-words text-slate-400"><span
          class="select-none text-amber-500">&gt; </span>{{ entry.source }}</pre>
        <pre
          class="whitespace-pre-wrap break-words"
          :class="entry.ok ? 'text-slate-200' : 'text-red-400'"
        >{{ entry.output }}</pre>
      </div>
    </div>

    <div class="flex flex-none items-stretch gap-2 border-t border-slate-800 p-2">
      <div
        ref="host"
        class="min-w-0 flex-1 overflow-hidden rounded border border-slate-700 bg-slate-800 focus-within:border-amber-500"
      />
      <button
        class="flex-none rounded bg-amber-500 px-3 text-xs font-medium text-white disabled:opacity-40"
        :disabled="running"
        @click="run"
      >
        Run
      </button>
    </div>
  </div>
</template>
