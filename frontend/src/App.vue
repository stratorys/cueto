<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
import { onMounted } from "vue";
import CodePane from "./components/CodePane.vue";
import DiagramCanvas from "./components/DiagramCanvas.vue";
import InspectorPanel from "./components/InspectorPanel.vue";
import ReplPane from "./components/ReplPane.vue";
import AppModal from "./components/AppModal.vue";
import { usePaneResize } from "./composables/usePaneResize";
import { useDiagramCanvas } from "./composables/useDiagramCanvas";
import { useProjects } from "./composables/useProjects";
import { initMode } from "./composables/useMode";
import { loadWorkspaceFile } from "./composables/useCueSync";

const {
  paneWidth,
  collapsed: editorCollapsed,
  startResize,
  toggleCollapse: toggleEditor,
} = usePaneResize(560, "left", "cueto.editorPaneWidth");
const {
  paneWidth: inspectorWidth,
  collapsed: inspectorCollapsed,
  startResize: startInspectorResize,
  toggleCollapse: toggleInspector,
} = usePaneResize(340, "right", "cueto.inspectorPaneWidth");
const {
  paneWidth: replHeight,
  collapsed: replCollapsed,
  startResize: startReplResize,
  toggleCollapse: toggleRepl,
} = usePaneResize(220, "bottom", "cueto.replPaneHeight");
const {
  files,
  activeFileName,
  activeText,
  setActiveFile,
  addFile,
  renameFile,
  closeFile,
  evalError,
  diagnostics,
  hints,
  showHints,
  toggleHints,
  onCueEdit,
  save,
  format,
  saveState,
  selectedElementId,
} = useDiagramCanvas();

const { init: initProjects } = useProjects();

// Resolve the persistence mode, then bootstrap the matching data source: in
// workspace mode load the working-tree file (git is the history, there are no
// projects); in playground mode resolve the current project and load its version.
onMounted(async () => {
  const mode = await initMode();
  if (mode === "workspace") await loadWorkspaceFile();
  else await initProjects();
});
</script>

<template>
  <div class="flex h-screen w-screen flex-col">
    <!-- Top row: editor | canvas | inspector. -->
    <div class="flex min-h-0 flex-1">
      <aside
        class="flex flex-none flex-col overflow-hidden"
        :style="{ width: (editorCollapsed ? 0 : paneWidth) + 'px' }"
      >
        <CodePane
          :code="activeText"
          :files="files"
          :active-file="activeFileName"
          :error="evalError"
          :save-state="saveState"
          :diagnostics="diagnostics"
          :hints="hints"
          :show-hints="showHints"
          :selected-element-id="selectedElementId"
          @update:code="onCueEdit"
          @toggle-hints="toggleHints"
          @format="format"
          @save="save"
          @set-active="setActiveFile"
          @add-file="addFile"
          @rename-file="renameFile"
          @close-file="closeFile"
        />
      </aside>
      <div
        class="group relative w-1.5 flex-none bg-slate-200 transition-colors hover:bg-amber-500"
        :class="editorCollapsed ? '' : 'cursor-col-resize'"
        @pointerdown.prevent="startResize"
      >
        <button
          type="button"
          class="absolute top-3 left-1/2 z-10 -translate-x-1/2 rounded border border-slate-300 bg-white px-1 py-0.5 text-xs leading-none text-slate-500 shadow-sm hover:bg-amber-500 hover:text-white"
          :title="editorCollapsed ? 'Expand editor' : 'Collapse editor'"
          @pointerdown.stop
          @click="toggleEditor"
        >
          {{ editorCollapsed ? "›" : "‹" }}
        </button>
      </div>
      <div class="relative h-full flex-1 overflow-hidden bg-slate-50">
        <DiagramCanvas />
      </div>
      <div
        class="group relative w-1.5 flex-none bg-slate-200 transition-colors hover:bg-amber-500"
        :class="inspectorCollapsed ? '' : 'cursor-col-resize'"
        @pointerdown.prevent="startInspectorResize"
      >
        <button
          type="button"
          class="absolute top-3 left-1/2 z-10 -translate-x-1/2 rounded border border-slate-300 bg-white px-1 py-0.5 text-xs leading-none text-slate-500 shadow-sm hover:bg-amber-500 hover:text-white"
          :title="inspectorCollapsed ? 'Expand inspector' : 'Collapse inspector'"
          @pointerdown.stop
          @click="toggleInspector"
        >
          {{ inspectorCollapsed ? "‹" : "›" }}
        </button>
      </div>
      <aside
        class="flex flex-none flex-col overflow-hidden"
        :style="{ width: (inspectorCollapsed ? 0 : inspectorWidth) + 'px' }"
      >
        <InspectorPanel />
      </aside>
    </div>
    <!-- Bottom row: full-width ephemeral REPL, resizable and collapsible. -->
    <div
      class="group relative h-1.5 flex-none bg-slate-200 transition-colors hover:bg-amber-500"
      :class="replCollapsed ? '' : 'cursor-row-resize'"
      @pointerdown.prevent="startReplResize"
    >
      <button
        type="button"
        class="absolute top-1/2 left-3 z-10 -translate-y-1/2 rounded border border-slate-300 bg-white px-1 py-0.5 text-xs leading-none text-slate-500 shadow-sm hover:bg-amber-500 hover:text-white"
        :title="replCollapsed ? 'Expand REPL' : 'Collapse REPL'"
        @pointerdown.stop
        @click="toggleRepl"
      >
        {{ replCollapsed ? "▲ REPL" : "▼ REPL" }}
      </button>
    </div>
    <aside
      class="flex flex-none flex-col overflow-hidden"
      :style="{ height: (replCollapsed ? 0 : replHeight) + 'px' }"
    >
      <ReplPane :collapsed="replCollapsed" />
    </aside>
    <AppModal />
  </div>
</template>
