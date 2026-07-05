<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// Ephemeral CUE REPL. Each entry evaluates a standalone CUE snippet against the
// backend (/repl) and shows the concrete value as JSON, or its diagnostics. It is
// a scratchpad with no saved impact: nothing here touches the editor files, the
// diagram model, the schema, or any saved version, and the scroll-back lives only
// in component state - it is gone on refresh.
import { nextTick, ref } from "vue";
import { evalExpr } from "../api";

interface Entry {
  source: string;
  output: string;
  ok: boolean;
}

const source = ref("");
const entries = ref<Entry[]>([]);
const running = ref(false);
const logEl = ref<HTMLElement | null>(null);

async function run() {
  const text = source.value.trim();
  if (!text || running.value) return;
  running.value = true;
  const result = await evalExpr(text);
  entries.value.push({
    source: source.value,
    output: result.ok ? JSON.stringify(result.result, null, 2) : result.error,
    ok: result.ok,
  });
  source.value = "";
  running.value = false;
  await nextTick();
  if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight;
}

function clearLog() {
  entries.value = [];
}
</script>

<template>
  <div class="flex h-full flex-col border-t border-slate-200 bg-white">
    <div
      class="flex flex-none items-center justify-between border-b border-slate-200 px-3 py-1.5 text-xs"
    >
      <span class="font-semibold text-slate-700">REPL</span>
      <div class="flex items-center gap-3 text-slate-400">
        <span class="font-mono">eval CUE · nothing saved</span>
        <button v-if="entries.length" class="hover:text-amber-700" @click="clearLog">clear</button>
      </div>
    </div>

    <div ref="logEl" class="min-h-0 flex-1 space-y-2 overflow-y-auto px-3 py-2 font-mono text-xs">
      <p v-if="!entries.length" class="text-slate-400">
        Evaluate a standalone CUE snippet. ⌘/Ctrl+Enter runs it; ⌃L or ⌘K clears
        the log. Results never touch your files or saved versions.
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
      <textarea
        v-model="source"
        rows="2"
        spellcheck="false"
        placeholder="a: b: 3&#10;c: a.b + 1"
        class="min-w-0 flex-1 resize-none rounded border border-slate-200 px-2 py-1 font-mono text-xs focus:border-amber-500 focus:outline-none"
        @keydown.enter.meta.prevent="run"
        @keydown.enter.ctrl.prevent="run"
        @keydown.l.ctrl.prevent="clearLog"
        @keydown.k.meta.prevent="clearLog"
      />
      <button
        class="flex-none rounded bg-amber-500 px-3 text-xs font-medium text-white disabled:opacity-40"
        :disabled="running || !source.trim()"
        @click="run"
      >
        Run
      </button>
    </div>
  </div>
</template>
