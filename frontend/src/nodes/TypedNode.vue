<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
import { computed, nextTick, ref } from "vue";
import { NodeResizer } from "@vue-flow/node-resizer";
import type { TypedNodeType } from "../model";
import ColorPopover from "../components/ColorPopover.vue";
import EdgeHandles from "./EdgeHandles.vue";
import { commitNodeLabel, commitNodeResize, connecting } from "../composables/useDiagramCanvas";

// Typed domain node: a fixed silhouette chosen by `data.type`, with no per-node
// payload (unlike a table's columns). "entity" is a titled box, "process" a
// rounded rectangle, "decision" a diamond. Like a shape, a selected typed node
// shows resize handles and a fill/border color popover; double-click edits the
// label inline; the four side handles let relations attach to it.
const props = defineProps<{
  id: string;
  selected?: boolean;
  data: {
    label?: string;
    type: TypedNodeType;
    fill?: string;
    stroke?: string;
    // Arbitrary structured payload from #Node.data, shown as a key/value card.
    data?: Record<string, unknown>;
  };
}>();

const kind = computed<TypedNodeType>(() => props.data.type);

// The data card's rows: one per top-level field of `data.data`. Non-scalar values
// are compact-JSON so a nested object/list still reads on one line.
const dataRows = computed<{ key: string; value: string }[]>(() => {
  const payload = props.data.data;
  if (!payload || typeof payload !== "object") return [];
  return Object.entries(payload).map(([key, value]) => ({
    key,
    value: value === null || typeof value !== "object" ? String(value) : JSON.stringify(value),
  }));
});

// --- inline label editing (double-click) ------------------------------------
const editing = ref(false);
const draft = ref("");
const input = ref<HTMLTextAreaElement>();

async function startEdit() {
  draft.value = props.data.label ?? "";
  editing.value = true;
  await nextTick();
  input.value?.focus();
  input.value?.select();
}
function commitEdit() {
  if (!editing.value) return;
  editing.value = false;
  const next = draft.value.trim();
  if (next !== (props.data.label ?? "")) commitNodeLabel(props.id, next);
}
function cancelEdit() {
  editing.value = false;
}

// --- colors -----------------------------------------------------------------
// Inline style wins over the default classes only when a color is set; an absent
// field leaves the node's default look untouched.
const boxStyle = computed(() => ({
  backgroundColor: props.data.fill,
  borderColor: props.data.stroke,
}));

function onResizeEnd(event: { params: { x: number; y: number; width: number; height: number } }) {
  commitNodeResize(props.id, event.params);
}
</script>

<template>
  <div class="group relative h-full w-full" :class="{ connecting }" @dblclick.stop="startEdit">
    <NodeResizer
      v-if="selected"
      :min-width="40"
      :min-height="30"
      color="#d97706"
      @resize-end="onResizeEnd"
    />

    <!-- Color popover: shown while selected, one row for fill, one for border. -->
    <ColorPopover v-if="selected" :id="id" />

    <!-- Entity: a titled box (header + body), an ER entity with no attributes. -->
    <div
      v-if="kind === 'entity'"
      class="flex h-full min-h-12 w-full min-w-28 flex-col overflow-hidden rounded-md border border-slate-400 bg-white transition-colors group-hover:border-amber-500"
      :style="boxStyle"
    >
      <div
        class="shrink-0 border-b border-slate-200 bg-slate-100 px-2.5 py-1 text-center text-sm font-semibold text-slate-700"
      >
        <span v-if="!editing">{{ data.label || "Entity" }}</span>
      </div>
      <!-- Data card: one row per field of #Node.data (mirrors TableNode's rows). -->
      <div v-if="dataRows.length" class="min-h-0 flex-1">
        <div
          v-for="row in dataRows"
          :key="row.key"
          class="flex items-center justify-between gap-3 border-t border-slate-100 px-2.5 py-1 text-xs"
        >
          <span class="min-w-0 flex-1 truncate text-slate-500">{{ row.key }}</span>
          <span class="truncate text-right font-mono text-slate-700">{{ row.value }}</span>
        </div>
      </div>
      <div v-else class="min-h-0 flex-1" />
    </div>

    <!-- Decision: a rotated square with upright content. -->
    <div
      v-else-if="kind === 'decision'"
      class="relative flex h-full min-h-16 w-full min-w-16 items-center justify-center"
    >
      <div
        class="absolute inset-2 rotate-45 rounded-sm border border-slate-400 bg-white transition-colors group-hover:border-amber-500"
        :style="boxStyle"
      />
      <span
        v-if="!editing && data.label"
        class="relative z-10 px-2 text-center text-xs text-slate-600"
        >{{ data.label }}</span
      >
    </div>

    <!-- Process: a rounded rectangle (a flowchart process step). -->
    <div
      v-else
      class="flex h-full min-h-12 w-full min-w-24 items-center justify-center rounded-2xl border border-slate-400 bg-white transition-colors group-hover:border-amber-500"
      :style="boxStyle"
    >
      <span v-if="!editing && data.label" class="px-2 text-center text-sm text-slate-600">{{
        data.label
      }}</span>
    </div>

    <!-- Inline label editor overlay (one at a time, silhouette-agnostic). -->
    <textarea
      v-if="editing"
      ref="input"
      v-model="draft"
      class="nodrag nopan absolute inset-1 z-20 resize-none rounded border border-amber-400 bg-white/95 px-1 py-0.5 text-center text-sm text-slate-700 outline-none"
      @blur="commitEdit"
      @keydown.enter.prevent="commitEdit"
      @keydown.escape.prevent="cancelEdit"
      @pointerdown.stop
    />

    <!-- Border-drag connection handles: one bar per side, glow on hover. -->
    <EdgeHandles :node-id="id" />
  </div>
</template>
