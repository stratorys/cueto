<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
import CodePane from "./components/CodePane.vue";
import DiagramCanvas from "./components/DiagramCanvas.vue";
import InspectorPanel from "./components/InspectorPanel.vue";
import { usePaneResize } from "./composables/usePaneResize";
import { useDiagramCanvas } from "./composables/useDiagramCanvas";

const { paneWidth, startResize } = usePaneResize();
const { paneWidth: inspectorWidth, startResize: startInspectorResize } = usePaneResize(340, "right");
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
</script>

<template>
  <div class="flex h-screen w-screen">
    <aside class="flex flex-none flex-col overflow-hidden" :style="{ width: paneWidth + 'px' }">
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
      class="w-1.5 flex-none cursor-col-resize bg-slate-200 transition-colors hover:bg-amber-500"
      @pointerdown.prevent="startResize"
    />
    <div class="relative h-full flex-1 bg-slate-50">
      <DiagramCanvas />
    </div>
    <div
      class="w-1.5 flex-none cursor-col-resize bg-slate-200 transition-colors hover:bg-amber-500"
      @pointerdown.prevent="startInspectorResize"
    />
    <aside class="flex flex-none flex-col overflow-hidden" :style="{ width: inspectorWidth + 'px' }">
      <InspectorPanel />
    </aside>
  </div>
</template>
