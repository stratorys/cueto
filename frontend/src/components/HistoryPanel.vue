<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// Architecture changelog: pick two saved versions and see a model-level diff
// (added / removed / changed nodes, added / removed / rewired edges) rather than
// a text diff. Clicking a change highlights that element on the canvas.
import { computed, onMounted, ref, watch } from "vue";
import type { VersionMeta } from "../api";
import { evalCue, fromEval, listVersions, readVersion } from "../api";
import type { Diagram } from "../model";
import type { DiagramDiff } from "../analysis/diff";
import { diffDiagrams, isEmptyDiff } from "../analysis/diff";
import { useHighlight } from "../composables/useHighlight";

const { setHighlight } = useHighlight();

const versions = ref<VersionMeta[]>([]);
const baseId = ref("");
const compareId = ref("");
const diff = ref<DiagramDiff | null>(null);
const error = ref<string | null>(null);
const loading = ref(false);

const shortHash = (id: string) => id.slice(0, 7);
function versionLabel(v: VersionMeta): string {
  const when = v.savedAt ? new Date(v.savedAt).toLocaleString() : "unknown time";
  return `${shortHash(v.version)} - ${when}`;
}

onMounted(async () => {
  const result = await listVersions();
  if (!result.ok) {
    error.value = result.error;
    return;
  }
  versions.value = result.versions;
  // Default to comparing the two newest saves (list is newest-first).
  if (versions.value.length >= 2) {
    compareId.value = versions.value[0].version;
    baseId.value = versions.value[1].version;
  }
});

// Load a version's stored data.cue and evaluate it into a concrete Diagram,
// reusing the same eval pipeline the canvas uses (no second parser).
async function loadDiagram(id: string): Promise<Diagram | null> {
  const version = await readVersion(id);
  if (!version.ok) {
    error.value = version.error;
    return null;
  }
  const evaluated = await evalCue(version.data);
  if (!evaluated.ok) {
    error.value = evaluated.error;
    return null;
  }
  return fromEval(evaluated.diagram);
}

watch([baseId, compareId], async () => {
  diff.value = null;
  error.value = null;
  if (!baseId.value || !compareId.value || baseId.value === compareId.value) return;
  loading.value = true;
  const [base, compare] = await Promise.all([
    loadDiagram(baseId.value),
    loadDiagram(compareId.value),
  ]);
  loading.value = false;
  if (base && compare) diff.value = diffDiagrams(base, compare);
});

const empty = computed(() => diff.value !== null && isEmptyDiff(diff.value));
</script>

<template>
  <div class="flex flex-col gap-4 p-4 text-sm">
    <p v-if="versions.length < 2" class="text-slate-400">
      Save at least two versions (Cmd+S) to compare them.
    </p>

    <template v-else>
      <label class="flex flex-col gap-1">
        <span class="text-xs uppercase tracking-wide text-slate-400">Base</span>
        <select v-model="baseId" class="rounded border border-slate-200 px-2 py-1">
          <option value="">-</option>
          <option v-for="v in versions" :key="v.version" :value="v.version">
            {{ versionLabel(v) }}
          </option>
        </select>
      </label>
      <label class="flex flex-col gap-1">
        <span class="text-xs uppercase tracking-wide text-slate-400">Compare</span>
        <select v-model="compareId" class="rounded border border-slate-200 px-2 py-1">
          <option value="">-</option>
          <option v-for="v in versions" :key="v.version" :value="v.version">
            {{ versionLabel(v) }}
          </option>
        </select>
      </label>
    </template>

    <p v-if="error" class="whitespace-pre-wrap text-red-600">{{ error }}</p>
    <p v-else-if="loading" class="text-slate-400">Comparing...</p>
    <p v-else-if="empty" class="text-slate-400">No model-level changes.</p>

    <div v-else-if="diff" class="flex flex-col gap-4">
      <section v-if="diff.nodesAdded.length">
        <h3 class="mb-1 font-semibold text-emerald-700">Added nodes</h3>
        <ul class="space-y-0.5">
          <li v-for="n in diff.nodesAdded" :key="n.id">
            <button class="w-full truncate rounded px-2 py-1 text-left hover:bg-emerald-50" @click="setHighlight([n.id])">
              + {{ n.label || n.id }}
            </button>
          </li>
        </ul>
      </section>

      <section v-if="diff.nodesRemoved.length">
        <h3 class="mb-1 font-semibold text-red-700">Removed nodes</h3>
        <ul class="space-y-0.5">
          <li v-for="n in diff.nodesRemoved" :key="n.id" class="truncate px-2 py-1 text-slate-500">
            - {{ n.label || n.id }}
          </li>
        </ul>
      </section>

      <section v-if="diff.nodesChanged.length">
        <h3 class="mb-1 font-semibold text-amber-700">Changed nodes</h3>
        <ul class="space-y-0.5">
          <li v-for="c in diff.nodesChanged" :key="c.id">
            <button class="w-full truncate rounded px-2 py-1 text-left hover:bg-amber-50" @click="setHighlight([c.id])">
              {{ c.after.label || c.id }}
              <span class="text-xs text-slate-400">({{ c.fields.join(", ") }})</span>
            </button>
          </li>
        </ul>
      </section>

      <section v-if="diff.edgesAdded.length">
        <h3 class="mb-1 font-semibold text-emerald-700">Added edges</h3>
        <ul class="space-y-0.5">
          <li v-for="e in diff.edgesAdded" :key="e.id">
            <button class="w-full truncate rounded px-2 py-1 text-left hover:bg-emerald-50" @click="setHighlight([e.source, e.target], [e.id])">
              + {{ e.source }} -> {{ e.target }}
            </button>
          </li>
        </ul>
      </section>

      <section v-if="diff.edgesRemoved.length">
        <h3 class="mb-1 font-semibold text-red-700">Removed edges</h3>
        <ul class="space-y-0.5">
          <li v-for="e in diff.edgesRemoved" :key="e.id" class="truncate px-2 py-1 text-slate-500">
            - {{ e.source }} -> {{ e.target }}
          </li>
        </ul>
      </section>

      <section v-if="diff.edgesRewired.length">
        <h3 class="mb-1 font-semibold text-amber-700">Rewired edges</h3>
        <ul class="space-y-0.5">
          <li v-for="e in diff.edgesRewired" :key="e.id">
            <button class="w-full truncate rounded px-2 py-1 text-left hover:bg-amber-50" @click="setHighlight([e.after.source, e.after.target], [e.id])">
              {{ e.before.source }} -> {{ e.before.target }}
              <span class="text-slate-400">becomes</span>
              {{ e.after.source }} -> {{ e.after.target }}
            </button>
          </li>
        </ul>
      </section>
    </div>
  </div>
</template>
