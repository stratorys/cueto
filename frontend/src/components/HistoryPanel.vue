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
import {
  evalCue,
  fromEval,
  listVersions,
  listWorkspaceHistory,
  readVersion,
  readWorkspaceFile,
} from "../api";
import type { Diagram } from "../model";
import type { DiagramDiff } from "../analysis/diff";
import { diffDiagrams, isEmptyDiff } from "../analysis/diff";
import { useHighlight } from "../composables/useHighlight";
import { isWorkspace } from "../composables/useMode";
import { useProjects } from "../composables/useProjects";
import { activeFileName } from "../composables/useEditorFiles";

const { setHighlight } = useHighlight();
const { currentProjectId } = useProjects();

// One point in history, common to both modes: a version/commit id, an optional
// human label (the commit subject in workspace mode), and when it was recorded.
interface HistoryItem {
  version: string;
  label: string;
  when: string;
}

const versions = ref<HistoryItem[]>([]);
const baseId = ref("");
const compareId = ref("");
const diff = ref<DiagramDiff | null>(null);
const error = ref<string | null>(null);
const loading = ref(false);

const shortHash = (id: string) => id.slice(0, 7);
function versionLabel(v: HistoryItem): string {
  const when = v.when ? new Date(v.when).toLocaleString() : "unknown time";
  const head = v.label ? `${shortHash(v.version)} ${v.label}` : shortHash(v.version);
  return `${head} - ${when}`;
}

// Load the history, defaulting the diff selectors to the two newest points. In
// workspace mode this is the git log of the active file; in playground mode the
// current project's saved versions. Re-run when the project, the file, or the mode
// changes (the mode resolves after this panel first mounts).
async function refreshVersions() {
  versions.value = [];
  baseId.value = "";
  compareId.value = "";
  diff.value = null;
  error.value = null;

  if (isWorkspace.value) {
    const result = await listWorkspaceHistory(activeFileName.value);
    if (!result.ok) {
      error.value = result.error;
      return;
    }
    versions.value = result.entries.map((e) => ({ version: e.version, label: e.label, when: e.at }));
  } else {
    if (!currentProjectId.value) return;
    const result = await listVersions(currentProjectId.value);
    if (!result.ok) {
      error.value = result.error;
      return;
    }
    versions.value = result.versions.map((v) => ({ version: v.version, label: "", when: v.savedAt }));
  }

  // Default to comparing the two newest points (list is newest-first).
  if (versions.value.length >= 2) {
    compareId.value = versions.value[0].version;
    baseId.value = versions.value[1].version;
  }
}

onMounted(refreshVersions);
watch([currentProjectId, isWorkspace, activeFileName], refreshVersions);

// Read a version's stored text and evaluate it into a concrete Diagram, reusing the
// canvas eval pipeline (no second parser). The text comes from the version store in
// playground mode, or the file at that commit in workspace mode.
async function loadDiagram(id: string): Promise<Diagram | null> {
  const read = isWorkspace.value
    ? await readWorkspaceFile(activeFileName.value, id)
    : await readVersion(currentProjectId.value, id);
  if (!read.ok) {
    error.value = read.error;
    return null;
  }
  const evaluated = await evalCue(read.data);
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
      {{ isWorkspace
        ? "Need at least two commits touching this file to compare them."
        : "Save at least two versions (Cmd+S) to compare them." }}
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
