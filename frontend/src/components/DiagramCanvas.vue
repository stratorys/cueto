<script setup lang="ts">
import { computed, markRaw, onBeforeUnmount, onMounted, ref } from "vue";
import { ConnectionMode, VueFlow } from "@vue-flow/core";
import type { EdgeTypesObject, NodeTypesObject } from "@vue-flow/core";
import { Background } from "@vue-flow/background";
import { Controls } from "@vue-flow/controls";
import type { ShapeKind } from "../model";
import { useDiagramCanvas } from "../composables/useDiagramCanvas";
import ShapeNode from "../nodes/ShapeNode.vue";
import TableNode from "../nodes/TableNode.vue";
import ContainerNode from "../nodes/ContainerNode.vue";
import ElkEdge from "../edges/ElkEdge.vue";
import ShapePalette from "./ShapePalette.vue";
import Toolbar from "./Toolbar.vue";

// Registered here (not in the composable) so the composable never imports the
// node components - they import commit helpers from the composable.
const nodeTypes = {
  shape: markRaw(ShapeNode),
  table: markRaw(TableNode),
  container: markRaw(ContainerNode),
} as unknown as NodeTypesObject;

const edgeTypes = {
  elk: markRaw(ElkEdge),
} as unknown as EdgeTypesObject;

const {
  nodes,
  edges,
  gridColor,
  activeTool,
  armTool,
  disarmTool,
  placeShape,
  placeTable,
  placeContainer,
  drawShape,
  connectShapes,
  onUndo,
  onRedo,
  canUndo,
  canRedo,
  save,
  breadcrumb,
  setFocus,
  layout,
} = useDiagramCanvas();

// Drag a shape or a table from the palette: drop it at the drop point.
function onDrop(event: DragEvent) {
  const kind = event.dataTransfer?.getData("application/shape");
  if (!kind) return;
  if (kind === "table") {
    placeTable(event.clientX, event.clientY);
    return;
  }
  if (kind === "container") {
    placeContainer(event.clientX, event.clientY);
    return;
  }
  placeShape(kind as ShapeKind, event.clientX, event.clientY);
}

// Palette table button click: drop a table at the canvas center.
function onAddTable() {
  const rect = host.value?.getBoundingClientRect();
  if (!rect) return;
  placeTable(rect.left + rect.width / 2, rect.top + rect.height / 2);
}

// Palette container button click: drop a container at the canvas center.
function onAddContainer() {
  const rect = host.value?.getBoundingClientRect();
  if (!rect) return;
  placeContainer(rect.left + rect.width / 2, rect.top + rect.height / 2);
}

// Draw-mode overlay: while a tool is armed, press-drag-release on the canvas
// draws the shape at that size (a click = default size). The overlay sits above
// the Vue Flow pane so the drag draws instead of panning.
const host = ref<HTMLDivElement>();
const drawLayer = ref<HTMLDivElement>();
const draw = ref<{ x0: number; y0: number; x1: number; y1: number } | null>(null);

// The armed *draw* tool: a shape tool, or null when idle or in connect mode.
// Connect mode must NOT raise the draw overlay - the overlay sits above Vue Flow
// and would swallow the handle-to-handle drags that create relations.
const drawTool = computed<ShapeKind | null>(() =>
  activeTool.value && activeTool.value !== "connect" ? activeTool.value : null,
);

const preview = computed(() => {
  const d = draw.value;
  const rect = host.value?.getBoundingClientRect();
  if (!d || !rect) return null;
  return {
    left: Math.min(d.x0, d.x1) - rect.left,
    top: Math.min(d.y0, d.y1) - rect.top,
    width: Math.abs(d.x1 - d.x0),
    height: Math.abs(d.y1 - d.y0),
  };
});

// Line preview direction, matching drawShape's flip rule (same-sign dx/dy -> "\").
const drawFlip = computed(() => {
  const d = draw.value;
  return d ? (d.x1 - d.x0) * (d.y1 - d.y0) > 0 : false;
});

// The draw overlay sits above Vue Flow, so to find what a client point is over
// (a handle or a node) we momentarily disable the overlay and hit-test the DOM
// underneath. Returns the exact { nodeId, handleId } when the point is on a
// handle; when it is on a shape/container body (but no handle) it snaps to that
// node's nearest side; null over empty canvas or a table body (which has only
// column handles). This lets the line tool connect between the points you pick.
function endpointUnder(
  clientX: number,
  clientY: number,
): { nodeId: string; handleId: string } | null {
  const overlay = drawLayer.value;
  const prev = overlay?.style.pointerEvents ?? "";
  if (overlay) overlay.style.pointerEvents = "none";
  const el = document.elementFromPoint(clientX, clientY);
  if (overlay) overlay.style.pointerEvents = prev;

  const handle = el?.closest?.(".vue-flow__handle") as HTMLElement | null;
  if (handle) {
    const nodeId = handle.getAttribute("data-nodeid");
    const handleId = handle.getAttribute("data-handleid");
    if (nodeId && handleId) return { nodeId, handleId };
  }
  const node = el?.closest?.(".vue-flow__node") as HTMLElement | null;
  // Only shapes and containers expose the t/r/b/l side handles a nearest-side
  // snap would target; tables use per-column handles, so require an exact hit.
  if (
    node &&
    (node.classList.contains("vue-flow__node-shape") ||
      node.classList.contains("vue-flow__node-container"))
  ) {
    const nodeId = node.getAttribute("data-id");
    if (!nodeId) return null;
    const r = node.getBoundingClientRect();
    const side = { t: clientY - r.top, b: r.bottom - clientY, l: clientX - r.left, r: r.right - clientX };
    const handleId = Object.entries(side).sort((a, b) => a[1] - b[1])[0][0];
    return { nodeId, handleId };
  }
  return null;
}

function onDrawStart(event: PointerEvent) {
  if (!drawTool.value) return;
  (event.target as HTMLElement).setPointerCapture(event.pointerId);
  draw.value = { x0: event.clientX, y0: event.clientY, x1: event.clientX, y1: event.clientY };
}
function onDrawMove(event: PointerEvent) {
  if (!draw.value) return;
  draw.value = { ...draw.value, x1: event.clientX, y1: event.clientY };
}
function onDrawEnd() {
  const d = draw.value;
  draw.value = null;
  if (!d || !drawTool.value) return;
  // Line tool: a drag from one shape's handle to another's makes a relation with
  // the exact points chosen; anything else falls through to a decorative line.
  if (drawTool.value === "line") {
    const from = endpointUnder(d.x0, d.y0);
    const to = endpointUnder(d.x1, d.y1);
    if (from && to && from.nodeId !== to.nodeId) {
      connectShapes(from.nodeId, from.handleId, to.nodeId, to.handleId);
      return;
    }
  }
  drawShape(drawTool.value, d.x0, d.y0, d.x1, d.y1);
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === "Escape") {
    // A held tool disarms first; otherwise Escape climbs one level out of a
    // drilled-into container (to its parent, or the top level).
    if (activeTool.value) {
      disarmTool();
      return;
    }
    const el = event.target as HTMLElement | null;
    if (el?.closest?.(".cm-editor") || el?.tagName === "INPUT" || el?.tagName === "TEXTAREA") {
      return;
    }
    if (breadcrumb.value.length) {
      const parent = breadcrumb.value[breadcrumb.value.length - 2];
      setFocus(parent ? parent.id : null);
    }
    return;
  }
  // Let the CUE editor keep its own text undo/redo when it has focus.
  const target = event.target as HTMLElement | null;
  if (target?.closest?.(".cm-editor")) return;

  const mod = event.metaKey || event.ctrlKey;
  if (!mod) return;
  const key = event.key.toLowerCase();
  if (key === "z") {
    event.preventDefault();
    if (event.shiftKey) onRedo();
    else onUndo();
  } else if (key === "y") {
    event.preventDefault();
    onRedo();
  } else if (key === "s") {
    // Persist the current CUE as an immutable version, and suppress the browser
    // save dialog so the shortcut is not disruptive.
    event.preventDefault();
    save();
  }
}
onMounted(() => window.addEventListener("keydown", onKeydown));
onBeforeUnmount(() => window.removeEventListener("keydown", onKeydown));
</script>

<template>
  <div
    ref="host"
    class="relative h-full w-full"
    :class="drawTool ? 'cursor-crosshair' : ''"
    @drop.prevent="onDrop"
    @dragover.prevent
  >
    <Toolbar
      :can-undo="canUndo"
      :can-redo="canRedo"
      @undo="onUndo"
      @redo="onRedo"
      @layout="layout"
    />
    <ShapePalette
      :active="activeTool"
      @arm="armTool"
      @add-table="onAddTable"
      @add-container="onAddContainer"
    />

    <!-- Drill-down breadcrumb: shown only while focused into a container. -->
    <div
      v-if="breadcrumb.length"
      class="absolute left-1/2 top-3 z-10 flex -translate-x-1/2 items-center gap-1 rounded-lg border border-slate-200 bg-white/90 px-2 py-1 text-sm shadow-sm backdrop-blur"
    >
      <button class="rounded px-1.5 py-0.5 text-slate-600 hover:bg-slate-100" @click="setFocus(null)">
        All
      </button>
      <template v-for="(crumb, i) in breadcrumb" :key="crumb.id">
        <span class="text-slate-300">/</span>
        <button
          class="rounded px-1.5 py-0.5 hover:bg-slate-100"
          :class="i === breadcrumb.length - 1 ? 'font-semibold text-amber-700' : 'text-slate-600'"
          @click="setFocus(crumb.id)"
        >
          {{ crumb.label }}
        </button>
      </template>
    </div>
    <VueFlow
      id="diagram"
      v-model:nodes="nodes"
      v-model:edges="edges"
      :node-types="nodeTypes"
      :edge-types="edgeTypes"
      :connect-on-click="false"
      :connection-mode="ConnectionMode.Loose"
      :delete-key-code="['Backspace', 'Delete']"
      fit-view-on-init
    >
      <Background variant="lines" :gap="22" :size="1" :pattern-color="gridColor" />
      <Controls />
    </VueFlow>

    <!-- Draw overlay, only while a shape draw tool is armed (never in connect
         mode, so handle drags reach Vue Flow). Transparent, so the line tool's
         visible handles show through; endpointUnder() hit-tests beneath it. -->
    <div
      v-if="drawTool"
      ref="drawLayer"
      class="absolute inset-0 z-10 cursor-crosshair"
      @pointerdown.prevent="onDrawStart"
      @pointermove="onDrawMove"
      @pointerup="onDrawEnd"
    >
      <!-- Live preview of the real shape being drawn (not a selection box). -->
      <svg
        v-if="preview"
        class="pointer-events-none absolute overflow-visible"
        :style="{
          left: preview.left + 'px',
          top: preview.top + 'px',
          width: preview.width + 'px',
          height: preview.height + 'px',
        }"
        viewBox="0 0 100 100"
        preserveAspectRatio="none"
      >
        <rect
          v-if="activeTool === 'rectangle'"
          x="1" y="1" width="98" height="98" rx="5"
          fill="rgba(245,158,11,0.1)" stroke="#f59e0b" stroke-width="2"
          vector-effect="non-scaling-stroke"
        />
        <ellipse
          v-else-if="activeTool === 'ellipse'"
          cx="50" cy="50" rx="49" ry="49"
          fill="rgba(245,158,11,0.1)" stroke="#f59e0b" stroke-width="2"
          vector-effect="non-scaling-stroke"
        />
        <polygon
          v-else-if="activeTool === 'diamond'"
          points="50,1 99,50 50,99 1,50"
          fill="rgba(245,158,11,0.1)" stroke="#f59e0b" stroke-width="2"
          vector-effect="non-scaling-stroke"
        />
        <line
          v-else-if="activeTool === 'line'"
          x1="1" :y1="drawFlip ? 1 : 99" x2="99" :y2="drawFlip ? 99 : 1"
          stroke="#f59e0b" stroke-width="2"
          vector-effect="non-scaling-stroke"
        />
        <rect
          v-else-if="activeTool === 'text'"
          x="1" y="1" width="98" height="98" rx="3"
          fill="rgba(245,158,11,0.05)" stroke="#f59e0b" stroke-width="2" stroke-dasharray="4 3"
          vector-effect="non-scaling-stroke"
        />
      </svg>
    </div>
  </div>
</template>
