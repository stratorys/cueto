<script setup lang="ts">
// Graph analysis: single points of failure, dependency cycles, orphans, and a
// what-if simulator that highlights the impact set of taking nodes "down".
import { computed } from "vue";
import { useAnalysis } from "../composables/useAnalysis";

const {
  diagram,
  direction,
  downNodes,
  spofs,
  cycles,
  orphanNodes,
  impacted,
  toggleDown,
  clearDown,
  focusNode,
  focusCycle,
} = useAnalysis();

// id -> display label (falls back to the id).
const labels = computed(() => {
  const map = new Map<string, string>();
  for (const node of diagram.value.nodes) map.set(node.id, node.label || node.id);
  return map;
});
const label = (id: string) => labels.value.get(id) ?? id;

// Nodes eligible for the what-if toggle (containers are structural).
const nodes = computed(() =>
  diagram.value.nodes.filter((n) => n.type !== "container"),
);
</script>

<template>
  <div class="flex flex-col gap-5 p-4 text-sm">
    <!-- Direction -->
    <div class="flex items-center gap-2">
      <span class="text-slate-500">Impact flows to</span>
      <div class="inline-flex overflow-hidden rounded-md border border-slate-200">
        <button
          class="px-2 py-1 text-xs"
          :class="direction === 'dependents' ? 'bg-amber-500 text-white' : 'text-slate-600'"
          @click="direction = 'dependents'"
        >
          dependents
        </button>
        <button
          class="px-2 py-1 text-xs"
          :class="direction === 'dependsOn' ? 'bg-amber-500 text-white' : 'text-slate-600'"
          @click="direction = 'dependsOn'"
        >
          depends-on
        </button>
      </div>
    </div>

    <!-- Single points of failure -->
    <section>
      <h3 class="mb-1 font-semibold text-slate-700">Single points of failure</h3>
      <p v-if="!spofs.length" class="text-slate-400">None.</p>
      <ul v-else class="space-y-0.5">
        <li v-for="id in spofs" :key="id">
          <button
            class="w-full truncate rounded px-2 py-1 text-left hover:bg-amber-50"
            @click="focusNode(id)"
          >
            {{ label(id) }}
          </button>
        </li>
      </ul>
    </section>

    <!-- Dependency cycles -->
    <section>
      <h3 class="mb-1 font-semibold text-slate-700">Dependency cycles</h3>
      <p v-if="!cycles.length" class="text-slate-400">None.</p>
      <ul v-else class="space-y-0.5">
        <li v-for="(cycle, i) in cycles" :key="i">
          <button
            class="w-full truncate rounded px-2 py-1 text-left hover:bg-amber-50"
            @click="focusCycle(cycle)"
          >
            {{ cycle.map(label).join(" -> ") }}
          </button>
        </li>
      </ul>
    </section>

    <!-- Orphans -->
    <section>
      <h3 class="mb-1 font-semibold text-slate-700">Orphans</h3>
      <p v-if="!orphanNodes.length" class="text-slate-400">None.</p>
      <ul v-else class="space-y-0.5">
        <li v-for="id in orphanNodes" :key="id">
          <button
            class="w-full truncate rounded px-2 py-1 text-left hover:bg-amber-50"
            @click="focusNode(id)"
          >
            {{ label(id) }}
          </button>
        </li>
      </ul>
    </section>

    <!-- What-if simulation -->
    <section>
      <div class="mb-1 flex items-center justify-between">
        <h3 class="font-semibold text-slate-700">What-if: nodes down</h3>
        <button
          v-if="downNodes.size"
          class="text-xs text-slate-500 hover:text-amber-700"
          @click="clearDown()"
        >
          clear
        </button>
      </div>
      <p v-if="downNodes.size" class="mb-2 text-xs text-amber-700">
        {{ impacted.size }} node(s) impacted.
      </p>
      <ul class="space-y-0.5">
        <li v-for="n in nodes" :key="n.id">
          <label class="flex cursor-pointer items-center gap-2 rounded px-2 py-1 hover:bg-slate-50">
            <input
              type="checkbox"
              class="accent-amber-500"
              :checked="downNodes.has(n.id)"
              @change="toggleDown(n.id)"
            />
            <span class="truncate">{{ n.label || n.id }}</span>
          </label>
        </li>
      </ul>
    </section>
  </div>
</template>
