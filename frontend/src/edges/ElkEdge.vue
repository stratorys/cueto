<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { BaseEdge, EdgeLabelRenderer, getSmoothStepPath } from "@vue-flow/core";
import type { EdgeProps } from "@vue-flow/core";
import type { DiagramEdge } from "../model";
import {
  editingEdgeId,
  startEdgeEdit,
  cancelEdgeEdit,
  commitEdgeLabel,
} from "../composables/useDiagramCanvas";

// Edge that draws the exact orthogonal polyline elkjs computed for it, passed in
// as absolute-coordinate `data.points`. Until a layout runs (or after a manual
// drag clears the stale points) it falls back to Vue Flow's smooth-step path, so
// every edge renders regardless of layout state. `data.kind` picks the visual
// connector: a filled arrowhead, a hollow inheritance triangle, or a dashed line
// (markers are defined once in MarkerDefs, rendered by DiagramCanvas). A free-form
// `label` (double-click to edit) and the cardinality (card) render as pills at
// the edge midpoint.
const props = defineProps<
  EdgeProps<{
    points?: { x: number; y: number }[];
    kind?: DiagramEdge["kind"];
    label?: string;
    card?: string;
    selfIndex?: number;
  }>
>();

// Marker follows `kind`; the marker shapes inherit the edge stroke via
// `context-stroke`, so they track the amber selection color too. "relation" and
// "line" carry no marker.
const markerUrl = computed(() => {
  if (props.data?.kind === "arrow") return "url(#cueto-arrow)";
  if (props.data?.kind === "inherit") return "url(#cueto-inherit)";
  return undefined;
});

// The edge stroke is set inline (from the mapping), which beats the default
// theme's `.selected` CSS rule - so selection is drawn here instead: an amber,
// thicker stroke that overrides the base style while the edge is selected. A
// "line" kind renders dashed.
const edgeStyle = computed(() => {
  const dash = props.data?.kind === "line" ? { strokeDasharray: "6 4" } : {};
  return props.selected
    ? { ...(props.style as object), ...dash, stroke: "#f59e0b", strokeWidth: 2.5 }
    : { ...(props.style as object), ...dash };
});

// The SVG path plus the point where labels sit. For a laid-out polyline the label
// rides the middle of the central segment; the smooth-step fallback reports its
// own label anchor.
const route = computed(() => {
  const points = props.data?.points;
  if (points && points.length >= 2) {
    const d = points.map((p, i) => `${i === 0 ? "M" : "L"} ${p.x} ${p.y}`).join(" ");
    const mid = Math.floor((points.length - 1) / 2);
    const a = points[mid];
    const b = points[mid + 1] ?? a;
    return { d, labelX: (a.x + b.x) / 2, labelY: (a.y + b.y) / 2 };
  }
  // Self-loop (source === target, e.g. a self-referential relation): ELK gives no
  // route, so draw a compact arch from the source handle up and back to the target
  // handle, with the label at its apex. Multiple self-loops on one node fan out by
  // `selfIndex` - each successive arch reaches wider and higher - so they never stack
  // and their labels sit at different heights.
  if (props.source === props.target) {
    const fan = (props.data?.selfIndex ?? 0) * 34;
    const top = Math.min(props.sourceY, props.targetY) - 56 - fan;
    const spread = 52 + fan;
    const d = `M ${props.sourceX} ${props.sourceY} C ${props.sourceX + spread} ${top}, ${props.targetX - spread} ${top}, ${props.targetX} ${props.targetY}`;
    return { d, labelX: (props.sourceX + props.targetX) / 2, labelY: top + 8 };
  }
  const [d, labelX, labelY] = getSmoothStepPath({
    sourceX: props.sourceX,
    sourceY: props.sourceY,
    sourcePosition: props.sourcePosition,
    targetX: props.targetX,
    targetY: props.targetY,
    targetPosition: props.targetPosition,
  });
  return { d, labelX, labelY };
});

// Cardinality shown as a compact secondary pill, e.g. "1-n".
const meta = computed(() => props.data?.card ?? "");

// --- inline label editing (double-click the edge) ---------------------------
const editing = computed(() => editingEdgeId.value === props.id);
const draft = ref("");
const input = ref<HTMLInputElement>();

watch(editing, async (on) => {
  if (!on) return;
  draft.value = props.data?.label ?? "";
  await nextTick();
  input.value?.focus();
  input.value?.select();
});

function commitLabel() {
  if (editing.value) commitEdgeLabel(props.id, draft.value.trim());
}
</script>

<template>
  <BaseEdge :id="id" :path="route.d" :marker-end="markerUrl" :style="edgeStyle" />
  <EdgeLabelRenderer>
    <div
      v-if="data?.label || meta || editing"
      class="nodrag nopan absolute flex flex-col items-center leading-tight"
      :style="{
        transform: `translate(-50%, -50%) translate(${route.labelX}px, ${route.labelY}px)`,
        pointerEvents: 'all',
      }"
      @dblclick.stop="startEdgeEdit(id)"
    >
      <input
        v-if="editing"
        ref="input"
        v-model="draft"
        class="w-24 rounded-sm bg-slate-50 px-1 text-center text-xs text-slate-700 ring-1 ring-amber-400 outline-none"
        @blur="commitLabel"
        @keydown.enter.prevent="commitLabel"
        @keydown.escape.prevent="cancelEdgeEdit"
        @pointerdown.stop
      />
      <template v-else>
        <span
          v-if="data?.label"
          class="cursor-text bg-slate-50 px-1 text-xs font-medium"
          :class="selected ? 'text-amber-600' : 'text-slate-700'"
          >{{ data.label }}</span
        >
        <span
          v-if="meta"
          class="bg-slate-50 px-1 text-[10px] font-medium text-slate-400"
          >{{ meta }}</span
        >
      </template>
    </div>
  </EdgeLabelRenderer>
</template>
