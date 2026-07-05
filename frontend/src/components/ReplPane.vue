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
// `diagram.nodes.payments_db.owner`. The input is a CodeMirror editor with
// autocomplete over the diagram's field paths, the CUE builtins, and every
// importable standard-library package (the backend injects the needed imports).
// The reference browser lists the same. It has no saved impact: nothing here
// mutates the editor files, the diagram, the schema, or any saved version, and
// the scroll-back lives only in component state - gone on refresh.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { EditorState } from "@codemirror/state";
import { EditorView, keymap, placeholder } from "@codemirror/view";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import {
  acceptCompletion,
  autocompletion,
  closeBrackets,
  closeBracketsKeymap,
  completionKeymap,
  startCompletion,
} from "@codemirror/autocomplete";
import { cueLanguage } from "../cueLanguage";
import { evalExpr, fetchCueMeta, type CueMeta, type CuePackage } from "../api";
import { files } from "../composables/useEditorFiles";
import { cueCompletionSource, walkKeys, type ReplCompletionData } from "../replCompletions";

// collapsed is App's REPL-pane state. The pane stays mounted (height 0) when
// collapsed, so the key-fetch is gated on it to avoid evaluating the diagram on
// every edit while the REPL is hidden.
const props = defineProps<{ collapsed?: boolean }>();

interface Entry {
  source: string;
  output: string;
  ok: boolean;
}

const entries = ref<Entry[]>([]);
const running = ref(false);
const logEl = ref<HTMLElement | null>(null);
const host = ref<HTMLDivElement>();

// Static CUE reference (fetched once) and the live diagram key paths (refreshed
// when the files change). Both feed autocomplete and the reference browser.
const meta = ref<CueMeta | null>(null);
const keys = ref<string[]>([]);

// Reference browser state.
const showRef = ref(false);
const refFilter = ref("");
const expanded = ref<Set<string>>(new Set());

let view: EditorView | undefined;
let keysTimer: ReturnType<typeof setTimeout> | undefined;
// Set when an edit arrived while collapsed, so the keys are refreshed once on the
// next expand instead of eagerly while hidden.
let keysStale = true;

// completionData is read lazily by the completion source, so it always sees the
// latest keys/meta. keys.value is replaced wholesale on refresh, so the source's
// identity-keyed cache invalidates correctly.
function completionData(): ReplCompletionData {
  return { keys: keys.value, meta: meta.value };
}

async function run() {
  if (running.value || !view) return;
  const text = view.state.doc.toString().trim();
  if (!text) return;
  running.value = true;
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

function clearLog() {
  entries.value = [];
}

// refreshKeys evaluates `diagram` against the live files and flattens the concrete
// result into dotted field paths. A currently-invalid diagram just leaves the last
// good key set in place.
async function refreshKeys() {
  const result = await evalExpr("diagram", files.value);
  if (result.ok) keys.value = [...walkKeys("diagram", result.result)].sort();
  keysStale = false;
}

// scheduleKeys debounces a key refresh, but only while the pane is visible; a
// collapsed pane just records that its keys are now stale.
function scheduleKeys() {
  if (props.collapsed) {
    keysStale = true;
    return;
  }
  clearTimeout(keysTimer);
  keysTimer = setTimeout(refreshKeys, 600);
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
        cueLanguage("light"),
        placeholder("diagram.nodes.payments_db.owner"),
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
          ...completionKeymap,
          ...historyKeymap,
          ...defaultKeymap,
        ]),
        EditorView.lineWrapping,
        replTheme,
      ],
    }),
  });
  const m = await fetchCueMeta();
  if (m.ok) meta.value = { builtins: m.builtins, packages: m.packages };
  if (!props.collapsed) refreshKeys();
});

// Keep the diagram key paths current as the files are edited, debounced so a burst
// of keystrokes triggers one eval (skipped while collapsed).
watch(files, scheduleKeys, { deep: true });

// Catch up on any edits that landed while collapsed the moment the pane reopens.
watch(
  () => props.collapsed,
  (collapsed) => {
    if (!collapsed && keysStale) refreshKeys();
  },
);

onBeforeUnmount(() => {
  clearTimeout(keysTimer);
  view?.destroy();
});

// The REPL input sits in the light pane. Compact, wraps long lines, scrolls past a
// few lines rather than pushing the log away.
const replTheme = EditorView.theme({
  "&": { fontSize: "12px" },
  ".cm-scroller": {
    fontFamily: "ui-monospace, Consolas, monospace",
    lineHeight: "18px",
    maxHeight: "120px",
  },
  ".cm-content": { padding: "6px 8px", caretColor: "#0f172a" },
  ".cm-cursor, .cm-dropCursor": { borderLeftColor: "#0f172a" },
  "&.cm-focused": { outline: "none" },
  ".cm-placeholder": { color: "#94a3b8" },
});
</script>

<template>
  <div class="flex h-full flex-col border-t border-slate-200 bg-white">
    <div
      class="flex flex-none items-center justify-between border-b border-slate-200 px-3 py-1.5 text-xs"
    >
      <span class="font-semibold text-slate-700">REPL</span>
      <div class="flex items-center gap-3 text-slate-400">
        <span class="font-mono">query the diagram · nothing saved</span>
        <button
          class="hover:text-amber-700"
          :class="{ 'text-amber-700': showRef }"
          @click="showRef = !showRef"
        >
          {{ showRef ? "log" : "browse" }}
        </button>
        <button v-if="entries.length && !showRef" class="hover:text-amber-700" @click="clearLog">
          clear
        </button>
      </div>
    </div>

    <!-- Reference browser: diagram keys, CUE builtins, importable packages. -->
    <div v-if="showRef" class="flex min-h-0 flex-1 flex-col">
      <div class="flex-none border-b border-slate-100 px-3 py-1.5">
        <input
          v-model="refFilter"
          spellcheck="false"
          placeholder="filter keys, builtins, functions…"
          class="w-full rounded border border-slate-200 px-2 py-1 font-mono text-xs focus:border-amber-500 focus:outline-none"
        />
      </div>
      <div class="min-h-0 flex-1 overflow-y-auto px-3 py-2 text-xs">
        <section class="mb-3">
          <h4 class="mb-1 font-semibold uppercase tracking-wide text-slate-400">Keys</h4>
          <p v-if="!filteredKeys.length" class="text-slate-300">none</p>
          <ul class="space-y-0.5">
            <li v-for="k in filteredKeys" :key="k">
              <button
                class="font-mono text-teal-700 hover:underline"
                @click="insert(k)"
              >
                {{ k }}
              </button>
            </li>
          </ul>
        </section>

        <section class="mb-3">
          <h4 class="mb-1 font-semibold uppercase tracking-wide text-slate-400">Builtins</h4>
          <div class="flex flex-wrap gap-x-3 gap-y-0.5">
            <button
              v-for="b in filteredBuiltins"
              :key="b.name"
              class="font-mono text-amber-700 hover:underline"
              @click="insert(b.name)"
            >
              {{ b.name }}
            </button>
          </div>
        </section>

        <section>
          <h4 class="mb-1 font-semibold uppercase tracking-wide text-slate-400">Packages</h4>
          <p v-if="!packageView.length" class="text-slate-300">none</p>
          <div v-for="row in packageView" :key="row.pkg.path" class="mb-0.5">
            <button
              class="flex w-full items-center gap-1 text-left font-mono text-violet-700 hover:underline"
              @click="togglePkg(row.pkg.name)"
            >
              <span class="w-3 select-none text-slate-400">{{ row.open ? "▾" : "▸" }}</span>
              <span>{{ row.pkg.name }}</span>
              <span class="text-slate-300">{{ row.pkg.path }}</span>
            </button>
            <ul v-if="row.open" class="ml-4 mt-0.5 flex flex-wrap gap-x-3 gap-y-0.5">
              <li v-for="m in row.members" :key="m.name">
                <button
                  class="font-mono hover:underline"
                  :class="m.isFunc ? 'text-sky-700' : 'text-slate-600'"
                  @click="insert(`${row.pkg.name}.${m.name}`)"
                >
                  {{ m.name }}<span v-if="m.isFunc" class="text-slate-300">()</span>
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
      <p v-if="!entries.length" class="text-slate-400">
        Query the diagram in the editor - e.g.
        <span class="font-mono">diagram.nodes.payments_db.owner</span> or
        <span class="font-mono">[for e in diagram.edges if e.sync {e.id}]</span>.
        Type to autocomplete keys, builtins, and package functions (⌃Space to force
        it); ⌘/Ctrl+Enter runs; ⌃L or ⌘K clears. Results never touch your files or
        saved versions.
      </p>
      <div v-for="(entry, i) in entries" :key="i" class="space-y-0.5">
        <pre class="whitespace-pre-wrap break-words text-slate-500"><span
          class="select-none text-amber-600">&gt; </span>{{ entry.source }}</pre>
        <pre
          class="whitespace-pre-wrap break-words"
          :class="entry.ok ? 'text-slate-800' : 'text-red-600'"
        >{{ entry.output }}</pre>
      </div>
    </div>

    <div class="flex flex-none items-stretch gap-2 border-t border-slate-200 p-2">
      <div
        ref="host"
        class="min-w-0 flex-1 overflow-hidden rounded border border-slate-200 focus-within:border-amber-500"
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
