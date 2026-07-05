<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// Right-hand inspector: three tabs over the same diagram model, sharing one
// canvas highlight. Switching tabs clears the highlight so one lens never leaks
// its selection into another.
import { ref, watch } from "vue";
import AnalysisPanel from "./AnalysisPanel.vue";
import ElementInspector from "./ElementInspector.vue";
import HistoryPanel from "./HistoryPanel.vue";
import QueryPanel from "./QueryPanel.vue";
import PolicyPanel from "./PolicyPanel.vue";
import { useDiagramCanvas } from "../composables/useDiagramCanvas";
import { useHighlight } from "../composables/useHighlight";

type Tab = "analysis" | "inspector" | "history" | "query" | "policy";
const tab = ref<Tab>("analysis");
const { clearHighlight } = useHighlight();
const { selectedElementId } = useDiagramCanvas();

const tabs: { id: Tab; label: string }[] = [
  { id: "analysis", label: "Analysis" },
  { id: "inspector", label: "Inspector" },
  { id: "history", label: "History" },
  { id: "query", label: "Query" },
  { id: "policy", label: "Policy" },
];

watch(tab, () => clearHighlight());

// Selecting a node/edge reveals its property editor; deselecting returns to the
// tab that was active before, so the empty Inspector is never left stranded.
let restoreTab: Tab = "analysis";
watch(selectedElementId, (id, prev) => {
  if (id && !prev) {
    // none -> selected: remember where we were, jump to the editor.
    if (tab.value !== "inspector") restoreTab = tab.value;
    tab.value = "inspector";
  } else if (!id && prev) {
    // selected -> none: restore only if the user did not navigate away.
    if (tab.value === "inspector") tab.value = restoreTab;
  }
});
</script>

<template>
  <div class="flex h-full flex-col border-l border-slate-200 bg-white">
    <div class="flex flex-none border-b border-slate-200 text-xs">
      <button
        v-for="t in tabs"
        :key="t.id"
        class="min-w-0 flex-1 truncate px-2 py-2 font-medium transition-colors"
        :class="tab === t.id
          ? 'border-b-2 border-amber-500 text-amber-700'
          : 'text-slate-500 hover:text-slate-800'"
        @click="tab = t.id"
      >
        {{ t.label }}
      </button>
    </div>
    <div class="min-h-0 flex-1 overflow-y-auto">
      <ElementInspector v-if="tab === 'inspector'" />
      <AnalysisPanel v-else-if="tab === 'analysis'" />
      <HistoryPanel v-else-if="tab === 'history'" />
      <QueryPanel v-else-if="tab === 'query'" />
      <PolicyPanel v-else />
    </div>
  </div>
</template>
