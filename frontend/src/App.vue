<script setup lang="ts">
import { markRaw, ref } from "vue";
import { useVueFlow, VueFlow } from "@vue-flow/core";
import type { NodeTypesObject } from "@vue-flow/core";
import { sampleDiagram } from "./model";
import { toFlowEdges, toFlowNodes } from "./toFlow";
import { toCue } from "./toCue";
import { fromEval } from "./fromEval";
import { evalCue } from "./api";
import { useDiagram } from "./useDiagram";
import TableNode from "./nodes/TableNode.vue";

// Custom node components. markRaw keeps Vue from making them reactive.
const nodeTypes = { table: markRaw(TableNode) } as unknown as NodeTypesObject;

const { diagram, commit, replace, undo, redo, canUndo, canRedo } = useDiagram();

// The CUE text is editable and is the source for the text → graph direction.
// Graph edits regenerate it (graph → CUE); typing re-evaluates it (CUE → graph).
const cueText = ref(toCue(diagram.value));
const evalError = ref<string | null>(null);

// Regenerate the text from the model after a graph edit. Called explicitly (not
// via a watcher) so a text-originated eval never overwrites what the user typed.
function syncTextFromModel() {
  cueText.value = toCue(diagram.value);
}

function rebuildGraph() {
  nodes.value = toFlowNodes(diagram.value);
  edges.value = toFlowEdges(diagram.value);
}

// Typing in the CUE pane: debounce, evaluate via the backend, rebuild the graph.
let evalTimer: ReturnType<typeof setTimeout> | undefined;
function onCueInput() {
  clearTimeout(evalTimer);
  evalTimer = setTimeout(runEval, 400);
}

async function runEval() {
  const result = await evalCue(cueText.value);
  if (!result.ok) {
    evalError.value = result.error;
    return;
  }
  evalError.value = null;
  replace(fromEval(result.diagram));
  rebuildGraph();
}

// Resizable split between the CUE pane and the canvas.
const paneWidth = ref(380);
function startResize() {
  window.addEventListener("pointermove", onResize);
  window.addEventListener("pointerup", stopResize);
  document.body.style.userSelect = "none";
  document.body.style.cursor = "col-resize";
}
function onResize(event: PointerEvent) {
  paneWidth.value = Math.min(Math.max(event.clientX, 220), window.innerWidth - 320);
}
function stopResize() {
  window.removeEventListener("pointermove", onResize);
  window.removeEventListener("pointerup", stopResize);
  document.body.style.userSelect = "";
  document.body.style.cursor = "";
}

// Controlled view state: bound with v-model so the arrays ARE the view.
// We mutate them directly; Vue Flow keeps its store in sync both ways.
const nodes = ref(toFlowNodes(sampleDiagram));
const edges = ref(toFlowEdges(sampleDiagram));

// Explicit id so this composable and <VueFlow> below share ONE store instance.
const { onNodeDragStop, onConnect } = useVueFlow("diagram");

// Drag: commit the final position once, not on every move.
onNodeDragStop(({ node }) => {
  commit((draft) => {
    const target = draft.nodes.find((n) => n.id === node.id);
    if (target) {
      target.x = node.position.x;
      target.y = node.position.y;
    }
  });
  syncTextFromModel();
});

// Connect: push straight into the v-model array (renders it) and the model,
// keyed by one stable id.
onConnect((params) => {
  const id = crypto.randomUUID();
  edges.value = [
    ...edges.value,
    {
      id,
      source: params.source,
      target: params.target,
      sourceHandle: params.sourceHandle ?? undefined,
      targetHandle: params.targetHandle ?? undefined,
      label: "relation",
    },
  ];
  commit((draft) => {
    draft.edges.push({
      id,
      source: params.source,
      target: params.target,
      sourceHandle: params.sourceHandle ?? undefined,
      targetHandle: params.targetHandle ?? undefined,
      kind: "relation",
    });
  });
  syncTextFromModel();
});

// Undo/redo mutate the model; re-seed the view and text from it.
function onUndo() {
  undo();
  rebuildGraph();
  syncTextFromModel();
}

function onRedo() {
  redo();
  rebuildGraph();
  syncTextFromModel();
}
</script>

<template>
  <div class="app">
    <aside class="code-pane" :style="{ width: paneWidth + 'px' }">
      <div class="code-header">
        <span>data.cue</span>
        <span v-if="evalError" class="status error">invalid</span>
        <span v-else class="status ok">valid</span>
      </div>
      <textarea
        v-model="cueText"
        class="code"
        spellcheck="false"
        @input="onCueInput"
      />
      <pre v-if="evalError" class="diag">{{ evalError }}</pre>
    </aside>
    <div class="resizer" @pointerdown.prevent="startResize" />
    <div class="canvas">
      <div class="toolbar">
        <button :disabled="!canUndo" @click="onUndo">Undo</button>
        <button :disabled="!canRedo" @click="onRedo">Redo</button>
        <span class="hint">Drag nodes · drag between column handles to connect</span>
      </div>
      <VueFlow
        id="diagram"
        v-model:nodes="nodes"
        v-model:edges="edges"
        :node-types="nodeTypes"
        :connect-on-click="false"
        fit-view-on-init
      />
    </div>
  </div>
</template>

<style>
@import "@vue-flow/core/dist/style.css";
@import "@vue-flow/core/dist/theme-default.css";

html,
body,
#app {
  margin: 0;
  height: 100%;
}

.app {
  display: flex;
  width: 100vw;
  height: 100vh;
}

.code-pane {
  display: flex;
  flex-direction: column;
  flex: none;
  background: #0f172a;
  color: #e2e8f0;
  overflow: hidden;
}

.resizer {
  flex: none;
  width: 6px;
  cursor: col-resize;
  background: #e2e8f0;
  transition: background 0.15s;
}

.resizer:hover {
  background: #94a3b8;
}

.code-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  font-size: 12px;
  font-family: ui-monospace, monospace;
  color: #94a3b8;
  border-bottom: 1px solid #1e293b;
  flex: none;
}

.status {
  margin-left: auto;
  padding: 1px 8px;
  border-radius: 4px;
  font-size: 11px;
}

.status.ok {
  background: rgba(34, 197, 94, 0.15);
  color: #4ade80;
}

.status.error {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
}

.code {
  margin: 0;
  padding: 12px;
  font-size: 12.5px;
  line-height: 1.5;
  font-family: ui-monospace, Consolas, monospace;
  white-space: pre;
  overflow: auto;
  flex: 1;
  border: none;
  resize: none;
  background: #0f172a;
  color: #e2e8f0;
  outline: none;
  tab-size: 4;
}

.diag {
  margin: 0;
  flex: none;
  max-height: 30%;
  overflow: auto;
  padding: 10px 12px;
  font-size: 11.5px;
  line-height: 1.45;
  font-family: ui-monospace, monospace;
  white-space: pre-wrap;
  color: #fca5a5;
  background: #1c1417;
  border-top: 1px solid #7f1d1d;
}

.canvas {
  position: relative;
  flex: 1;
  height: 100%;
}

.toolbar {
  position: absolute;
  z-index: 10;
  top: 12px;
  left: 12px;
  display: flex;
  gap: 8px;
  align-items: center;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  padding: 6px 10px;
}

.toolbar button {
  font-size: 13px;
  padding: 4px 10px;
  border: 1px solid #cbd5e1;
  border-radius: 4px;
  background: #fff;
  cursor: pointer;
}

.toolbar button:disabled {
  opacity: 0.4;
  cursor: default;
}

.toolbar .hint {
  font-size: 12px;
  color: #64748b;
}
</style>
