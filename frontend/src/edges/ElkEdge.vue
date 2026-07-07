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
import type { DiagramEdge, EdgeWaypoint } from "../model";
import {
  editingEdgeId,
  startEdgeEdit,
  cancelEdgeEdit,
  commitEdgeLabel,
  commitEdgeWaypoints,
} from "../composables/useDiagramCanvas";
import { screenToFlowCoordinate } from "../composables/flowStore";
import { absoluteToWaypoint, orthogonalRoute, roundedPath, waypointToAbsolute } from "./waypoints";
import type { Point } from "./waypoints";

// Draws an edge in priority order: the user's cosmetic trunk (data.waypoints, stored
// relative so it tracks endpoint moves), else the elkjs orthogonal route (data.points),
// else a smooth-step fallback. Grab the line and drag to slide its right-angle trunk
// aside for readability; drag back toward straight to release it. The relation is never
// demoted, only rerouted.
const props = defineProps<
  EdgeProps<{
    points?: Point[];
    waypoints?: EdgeWaypoint[];
    kind?: DiagramEdge["kind"];
    label?: string;
    card?: string;
    selfIndex?: number;
  }>
>();

const markerUrl = computed(() => {
  if (props.data?.kind === "arrow") return "url(#cueto-arrow)";
  if (props.data?.kind === "inherit") return "url(#cueto-inherit)";
  return undefined;
});

const edgeStyle = computed(() => {
  const dash = props.data?.kind === "line" ? { strokeDasharray: "6 4" } : {};
  return props.selected
    ? { ...(props.style as object), ...dash, stroke: "#f59e0b", strokeWidth: 2.5 }
    : { ...(props.style as object), ...dash };
});

// Live waypoints while dragging (uncommitted); otherwise the persisted ones.
const liveWaypoints = ref<EdgeWaypoint[] | null>(null);
const waypoints = computed(() => liveWaypoints.value ?? props.data?.waypoints ?? []);

const source = computed<Point>(() => ({ x: props.sourceX, y: props.sourceY }));
const target = computed<Point>(() => ({ x: props.targetX, y: props.targetY }));

// Self-loops route as an arc and coincident endpoints make the relative math
// degenerate, so waypoint editing is limited to distinct-endpoint edges. A derived
// edge is routable too - its waypoints live in ephemeral view state (commitEdgeWaypoints)
// rather than the coordinate-free CUE, so a relation can be nudged for readability.
const routable = computed(() => props.source !== props.target);

const route = computed(() => {
  // A cosmetic reroute: a right-angle path bent through the dragged point (free in both
  // axes), rounded corners for cleanliness, label riding the bend the user placed.
  if (waypoints.value.length) {
    const through = waypointToAbsolute(source.value, target.value, waypoints.value[0]);
    const pts = orthogonalRoute(source.value, target.value, through);
    return { d: roundedPath(pts, 12), labelX: through.x, labelY: through.y };
  }
  const points = props.data?.points;
  if (points && points.length >= 2) {
    const d = points.map((p, i) => `${i === 0 ? "M" : "L"} ${p.x} ${p.y}`).join(" ");
    const mid = Math.floor((points.length - 1) / 2);
    const a = points[mid];
    const b = points[mid + 1] ?? a;
    return { d, labelX: (a.x + b.x) / 2, labelY: (a.y + b.y) / 2 };
  }
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

// Grab-to-slide drag state. Pressing the selected line and dragging slides the relation's
// trunk to follow the cursor - a single offset, never a chain of points. It only commits
// once the pointer actually moves, so a press without a drag leaves no stray offset.
const DRAG_THRESHOLD_PX = 3;
// Released within this many graph units of the straight route, the reroute is dropped -
// drag the line back toward straight to release it.
const STRAIGHTEN_OFF = 8;
let dragging = false;
let dragOriginX = 0;
let dragOriginY = 0;
let dragMoved = false;

function onDragMove(event: PointerEvent) {
  if (!dragging) return;
  if (Math.hypot(event.clientX - dragOriginX, event.clientY - dragOriginY) > DRAG_THRESHOLD_PX) {
    dragMoved = true;
  }
  const p = screenToFlowCoordinate({ x: event.clientX, y: event.clientY });
  liveWaypoints.value = [absoluteToWaypoint(source.value, target.value, p)];
}

function onDragEnd() {
  window.removeEventListener("pointermove", onDragMove);
  window.removeEventListener("pointerup", onDragEnd);
  if (dragMoved && liveWaypoints.value) {
    const wp = liveWaypoints.value[0];
    // Slid back near the straight route it clears, so straightening needs no extra affordance.
    commitEdgeWaypoints(props.id, wp && Math.abs(wp.off) >= STRAIGHTEN_OFF ? [wp] : []);
  }
  liveWaypoints.value = null;
  dragging = false;
  dragMoved = false;
}

// Press on the edge line: start sliding its trunk from the current cursor position.
function onLineDown(event: PointerEvent) {
  const p = screenToFlowCoordinate({ x: event.clientX, y: event.clientY });
  liveWaypoints.value = [absoluteToWaypoint(source.value, target.value, p)];
  dragging = true;
  dragOriginX = event.clientX;
  dragOriginY = event.clientY;
  dragMoved = false;
  window.addEventListener("pointermove", onDragMove);
  window.addEventListener("pointerup", onDragEnd);
}

const meta = computed(() => props.data?.card ?? "");

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

  <!-- Grab band, shown once the edge is selected: press-drag to slide the relation's
       trunk aside for readability, drag back to straighten. `.stop` keeps the drag from
       reaching Vue Flow's edge-reconnect (updatable), which would detach the endpoint. -->
  <path
    v-if="routable && selected"
    class="cursor-grab"
    :d="route.d"
    fill="none"
    stroke="transparent"
    stroke-width="14"
    style="pointer-events: stroke"
    @pointerdown.stop.prevent="onLineDown"
  />

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
