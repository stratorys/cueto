<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
import { computed, nextTick, ref } from "vue";
import { Handle, Position } from "@vue-flow/core";
import { NodeResizer } from "@vue-flow/node-resizer";
import type { Column } from "../model";
import ColorPopover from "../components/ColorPopover.vue";
import { commitNodeLabel, commitNodeResize, connecting } from "../composables/useDiagramCanvas";

// Custom node for a DB table: a header (the table name) + one row per column.
// Columns are edited in the CUE pane (they round-trip through the model); the
// table name is edited inline by double-clicking the header. Each column exposes
// a left (target) and right (source) handle so relations can attach to a specific
// column, not just the node. Like a shape, a selected table shows resize handles
// and a fill/border color popover. Absent size -> the table auto-sizes to its
// columns; once resized it fills the drawn box (overflow clipped).
const props = defineProps<{
  id: string;
  selected?: boolean;
  data: { label: string; columns?: Column[]; fill?: string; stroke?: string };
}>();

// --- inline header rename (double-click) ------------------------------------
const editing = ref(false);
const draft = ref("");
const input = ref<HTMLInputElement>();

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
  if (next && next !== props.data.label) commitNodeLabel(props.id, next);
}
function cancelEdit() {
  editing.value = false;
}

// --- colors -----------------------------------------------------------------
// fill tints the column body (the header stays opaque); stroke sets the card
// border. An absent field leaves the default look untouched.
const cardStyle = computed(() => ({
  backgroundColor: props.data.fill,
  borderColor: props.data.stroke,
}));

// --- resize -----------------------------------------------------------------
function onResizeEnd(event: { params: { x: number; y: number; width: number; height: number } }) {
  commitNodeResize(props.id, event.params);
}
</script>

<template>
  <div class="group relative h-full w-full" :class="{ connecting }">
    <NodeResizer
      v-if="selected"
      :min-width="180"
      :min-height="48"
      color="#d97706"
      @resize-end="onResizeEnd"
    />

    <!-- Color popover: shown while selected, one row for fill, one for border. -->
    <ColorPopover v-if="selected" :id="id" />

    <div
      class="flex h-full w-full min-w-44 flex-col overflow-hidden rounded-md border bg-white text-sm shadow-sm transition-colors"
      :class="
        selected
          ? 'border-amber-500 ring-1 ring-amber-500/40'
          : 'border-slate-400 group-hover:border-amber-500'
      "
      :style="cardStyle"
    >
      <div
        class="relative shrink-0 border-b border-slate-200 bg-slate-100 px-2.5 py-1.5 text-center font-semibold text-slate-700"
        @dblclick.stop="startEdit"
      >
        <!-- Node-level handles at the header, for entity-level edges (a reference to the
             whole table, e.g. the inferred model view) that dock to no single column. -->
        <Handle id="table-target" type="target" :position="Position.Left" />
        <Handle id="table-source" type="source" :position="Position.Right" />
        <input
          v-if="editing"
          ref="input"
          v-model="draft"
          class="nodrag nopan w-full border-none bg-transparent text-center font-semibold text-slate-700 outline-none"
          @blur="commitEdit"
          @keydown.enter.prevent="commitEdit"
          @keydown.escape.prevent="cancelEdit"
          @pointerdown.stop
        />
        <template v-else>{{ data.label }}</template>
      </div>
      <div
        v-for="col in data.columns"
        :key="col.name"
        class="col relative flex items-center justify-between gap-3 border-t border-slate-100 px-2.5 py-1"
      >
        <Handle :id="`${col.name}-target`" type="target" :position="Position.Left" />
        <span class="min-w-0 flex-1 text-slate-800">
          {{ col.name }}
          <span
            v-if="col.pk"
            class="ml-1 inline-flex items-center rounded bg-amber-100 px-1 text-xs text-amber-800"
            >PK</span
          >
          <span
            v-if="col.fk"
            class="ml-1 inline-flex items-center rounded bg-blue-100 px-1 text-xs text-blue-800"
            >FK</span
          >
        </span>
        <span class="text-right font-mono text-slate-500">{{ col.dbType }}</span>
        <Handle :id="`${col.name}-source`" type="source" :position="Position.Right" />
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Vue Flow injects the handle element, so its dot styling and enlarged hit area
   are reached via :deep. Hidden until the node is hovered, matching ShapeNode. */
.col :deep(.vue-flow__handle) {
  width: 12px;
  height: 12px;
  background: rgba(245, 158, 11, 0.7);
  border: 2px solid #fff;
  opacity: 0;
  transition: opacity 0.12s ease;
  z-index: 2;
}

.group:hover .col :deep(.vue-flow__handle),
.connecting .col :deep(.vue-flow__handle) {
  opacity: 1;
}

/* Grow the interactive hit area beyond the visible dot without enlarging it. */
.col :deep(.vue-flow__handle)::after {
  content: "";
  position: absolute;
  inset: -6px;
}
</style>
