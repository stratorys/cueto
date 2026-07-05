<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// Query bar + saved lenses: filter the model into a subgraph and dim the rest on
// the canvas. A query can be saved as a named lens (localStorage).
import { computed, onMounted, ref, watch } from "vue";
import { useDiagram } from "../useDiagram";
import { useHighlight } from "../composables/useHighlight";
import { runQuery } from "../analysis/query";
import type { Lens } from "../analysis/lenses";
import { deleteLens, loadLenses, saveLens } from "../analysis/lenses";

const { diagram } = useDiagram();
const { setHighlight, clearHighlight } = useHighlight();

const query = ref("");
const lenses = ref<Lens[]>([]);
const newLensName = ref("");

onMounted(() => {
  lenses.value = loadLenses();
});

const result = computed(() => runQuery(diagram.value, query.value));

const matched = computed(() => {
  const byId = new Map(diagram.value.nodes.map((n) => [n.id, n]));
  return [...result.value.nodeIds].map((id) => ({ id, label: byId.get(id)?.label || id }));
});

// Applying a query dims everything not matched. An empty query clears the filter.
watch(
  result,
  (r) => {
    if (!query.value.trim()) {
      clearHighlight();
      return;
    }
    setHighlight(r.nodeIds, r.edgeIds, "focus");
  },
  { deep: true },
);

function clear() {
  query.value = "";
  clearHighlight();
}

function save() {
  const name = newLensName.value.trim();
  if (!name || !query.value.trim()) return;
  lenses.value = saveLens({ id: crypto.randomUUID(), name, query: query.value });
  newLensName.value = "";
}

function apply(lens: Lens) {
  query.value = lens.query;
}

function remove(id: string) {
  lenses.value = deleteLens(id);
}
</script>

<template>
  <div class="flex flex-col gap-4 p-4 text-sm">
    <div class="flex flex-col gap-1">
      <input
        v-model="query"
        placeholder="type:table  label:~payment  orphan  n-n"
        class="rounded border border-slate-200 px-2 py-1 font-mono text-xs"
      />
      <div class="flex items-center justify-between text-xs text-slate-400">
        <span>{{ matched.length }} node(s) matched</span>
        <button v-if="query" class="hover:text-amber-700" @click="clear">clear</button>
      </div>
    </div>

    <!-- Save current query as a lens -->
    <div v-if="query.trim()" class="flex gap-1">
      <input
        v-model="newLensName"
        placeholder="lens name"
        class="min-w-0 flex-1 rounded border border-slate-200 px-2 py-1 text-xs"
        @keydown.enter="save"
      />
      <button
        class="flex-none rounded bg-amber-500 px-2 py-1 text-xs text-white disabled:opacity-40"
        :disabled="!newLensName.trim()"
        @click="save"
      >
        Save lens
      </button>
    </div>

    <!-- Results -->
    <section v-if="matched.length">
      <h3 class="mb-1 font-semibold text-slate-700">Matches</h3>
      <ul class="space-y-0.5">
        <li v-for="m in matched" :key="m.id" class="truncate px-2 py-1 text-slate-600">
          {{ m.label }}
        </li>
      </ul>
    </section>

    <!-- Saved lenses -->
    <section v-if="lenses.length">
      <h3 class="mb-1 font-semibold text-slate-700">Saved lenses</h3>
      <ul class="space-y-0.5">
        <li v-for="lens in lenses" :key="lens.id" class="flex items-center gap-1">
          <button
            class="min-w-0 flex-1 truncate rounded px-2 py-1 text-left hover:bg-amber-50"
            @click="apply(lens)"
          >
            {{ lens.name }}
            <span class="text-xs text-slate-400">{{ lens.query }}</span>
          </button>
          <button class="flex-none px-1 text-slate-400 hover:text-red-600" @click="remove(lens.id)">
            x
          </button>
        </li>
      </ul>
    </section>
  </div>
</template>
