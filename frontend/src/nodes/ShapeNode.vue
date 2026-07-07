<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
import { computed, nextTick, ref } from "vue";
import { NodeResizer } from "@vue-flow/node-resizer";
import type { ShapeKind } from "../model";
import ColorPopover from "../components/ColorPopover.vue";
import EdgeHandles from "./EdgeHandles.vue";
import {
  commitNodeResize,
  commitNodeLabel,
  beginLineDrag,
  dragLineTo,
  endLineDrag,
  connecting,
} from "../composables/useDiagramCanvas";

// Free-form canvas shape: plain geometry, no icon. The node box is sized by Vue
// Flow (created/drawn size); the shape fills it. Connections are made by dragging
// from a shape's border (the four side handles, invisible until hovered, when the
// border glows). Selecting a shape shows resize handles and a color popover.
// Double-click a shape to edit its label inline. A "line" is decorative: no
// connection handles, no resizer. A "text" shape is a bare label (no box unless a
// border color is set).
const props = defineProps<{
  id: string;
  selected?: boolean;
  data: { label?: string; shape?: ShapeKind; fill?: string; stroke?: string; flip?: boolean };
}>();

const shape = computed<ShapeKind>(() => props.data.shape ?? "rectangle");

// --- inline label editing (double-click) ------------------------------------
const editing = ref(false);
const draft = ref("");
const input = ref<HTMLTextAreaElement>();

async function startEdit() {
  if (shape.value === "line") return;
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
// field leaves the shape's default look untouched.
const boxStyle = computed(() => ({
  backgroundColor: props.data.fill,
  borderColor: props.data.stroke,
}));

function onResizeEnd(event: {
  params: { x: number; y: number; width: number; height: number };
}) {
  commitNodeResize(props.id, event.params);
}

// --- line endpoint handles (free 360 drag) ----------------------------------
// Each end sits on a box corner chosen by `flip`. Window listeners are used so the
// drag survives the node re-rendering as its geometry updates live.
function endStyle(which: number) {
  const flip = props.data.flip;
  return {
    left: which === 0 ? "0%" : "100%",
    top: which === 0 ? (flip ? "0%" : "100%") : flip ? "100%" : "0%",
  };
}
function onEndMove(event: PointerEvent) {
  dragLineTo(event.clientX, event.clientY);
}
function onEndUp() {
  window.removeEventListener("pointermove", onEndMove);
  window.removeEventListener("pointerup", onEndUp);
  endLineDrag(props.id);
}
function onEndDown(which: number) {
  beginLineDrag(props.id, which);
  window.addEventListener("pointermove", onEndMove);
  window.addEventListener("pointerup", onEndUp);
}
</script>

<template>
  <div class="group relative h-full w-full" :class="{ connecting }" @dblclick.stop="startEdit">
    <NodeResizer
      v-if="selected && shape !== 'line'"
      :min-width="40"
      :min-height="30"
      color="#d97706"
      @resize-end="onResizeEnd"
    />

    <!-- Color popover: shown while selected, one row for fill, one for border. -->
    <ColorPopover v-if="selected" :id="id" />

    <!-- Line: follows the drag direction via `flip`; the border color sets its stroke. -->
    <svg v-if="shape === 'line'" viewBox="0 0 100 100" preserveAspectRatio="none" class="h-full w-full">
      <line
        :x1="2"
        :y1="data.flip ? 2 : 98"
        :x2="98"
        :y2="data.flip ? 98 : 2"
        :stroke="data.stroke ?? '#64748b'"
        stroke-width="2"
        vector-effect="non-scaling-stroke"
      />
    </svg>

    <!-- Diamond: a rotated square with upright content. -->
    <div v-else-if="shape === 'diamond'" class="relative flex h-full min-h-16 w-full min-w-16 items-center justify-center">
      <div
        class="absolute inset-2 rotate-45 rounded-sm border border-slate-400 bg-white transition-colors group-hover:border-amber-500"
        :style="boxStyle"
      />
      <span v-if="!editing && data.label" class="relative z-10 px-2 text-center text-xs text-slate-600">{{ data.label }}</span>
    </div>

    <!-- Text: a bare label; a border only appears when a border color is set. -->
    <div
      v-else-if="shape === 'text'"
      class="flex h-full min-h-6 w-full min-w-16 items-center justify-center rounded-md px-2"
      :class="data.stroke ? 'border' : ''"
      :style="boxStyle"
    >
      <span v-if="!editing" class="text-center text-sm" :class="data.label ? 'text-slate-700' : 'text-slate-400'">
        {{ data.label || "Text" }}
      </span>
    </div>

    <!-- Rectangle / ellipse. -->
    <div
      v-else
      class="flex h-full min-h-12 w-full min-w-24 items-center justify-center border border-slate-400 bg-white transition-colors group-hover:border-amber-500"
      :class="shape === 'ellipse' ? '' : 'rounded-md'"
      :style="[boxStyle, shape === 'ellipse' ? { borderRadius: '50%' } : {}]"
    >
      <span v-if="!editing && data.label" class="px-2 text-center text-sm text-slate-600">{{ data.label }}</span>
    </div>

    <!-- Inline label editor overlay (one at a time, shape-agnostic). -->
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

    <!-- Line endpoint handles: drag either end freely to any angle. Position is
         set inline (runtime geometry chosen by `flip`). -->
    <template v-if="shape === 'line' && selected">
      <div
        v-for="which in [0, 1]"
        :key="which"
        class="nodrag nopan absolute z-30 h-3 w-3 -translate-x-1/2 -translate-y-1/2 cursor-grab rounded-full border-2 border-amber-600 bg-white active:cursor-grabbing"
        :style="endStyle(which)"
        @pointerdown.stop.prevent="onEndDown(which)"
      />
    </template>

    <!-- Border-drag connection handles: one bar per side, glow on hover. Not for lines. -->
    <EdgeHandles v-if="shape !== 'line'" :node-id="id" />
  </div>
</template>
