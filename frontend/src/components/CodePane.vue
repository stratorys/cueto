<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { AlignLeft, Save } from "lucide-vue-next";
import CodeEditor from "./CodeEditor.vue";
import ProjectSwitcher from "./ProjectSwitcher.vue";
import StatusBar from "./StatusBar.vue";
import type { SaveState } from "../composables/useDiagramCanvas";
import type { EditorFile } from "../model";
import type { Diagnostic, Hint } from "../api";
import { promptDialog } from "../composables/useModal";
// The hand-owned schema, inlined at build time. The dev server needs
// server.fs.allow: ['..'] to read it from the sibling cue/ dir.
import schemaSource from "../../../cue/schema.cue?raw";

// Editable files round-trip through /eval and /rewrite (owned by the composable);
// schema.cue is a static read-only reference pinned as the last tab.
const props = defineProps<{
  code: string;
  files: EditorFile[];
  activeFile: string;
  error: string | null;
  saveState: SaveState;
  diagnostics: Diagnostic[];
  hints: Hint[];
  // Whether the editor draws inlay hints; forwarded to the editor and reflected in
  // the toggle button's label.
  showHints: boolean;
  // Id of the node or edge selected on the canvas, forwarded to the editor to
  // tint its block.
  selectedElementId: string | null;
}>();
const emit = defineEmits<{
  "update:code": [value: string];
  toggleHints: [];
  format: [];
  save: [];
  setActive: [name: string];
  addFile: [];
  renameFile: [oldName: string, newName: string];
  closeFile: [name: string];
}>();

// The schema tab is a separate read-only view, toggled independently of which
// editable file is active.
const viewingSchema = ref(false);

// The editable editor instance (for jump-to-line), the live caret position (shown
// in the status bar), and whether the problems strip is expanded.
const editorRef = ref<InstanceType<typeof CodeEditor> | null>(null);
const cursor = ref<{ line: number; col: number }>({ line: 1, col: 1 });
const showProblems = ref(false);

// Positioned diagnostics become clickable rows; an unpositioned eval/rewrite error
// (which produces no diagnostics) still counts as one problem for the status bar.
const problems = computed(() => (props.diagnostics ?? []).filter((d) => d.line));
const problemCount = computed(() => problems.value.length || (props.error ? 1 : 0));

function jumpTo(line: number, column: number) {
  editorRef.value?.revealLine(line, column || 1);
}

// Status-bar click toggles the strip; opening it also jumps to the first problem.
function openProblems() {
  if (!problemCount.value) return;
  showProblems.value = !showProblems.value;
  if (!showProblems.value) return;
  const first = problems.value[0];
  if (first) jumpTo(first.line, first.column);
}

// A canvas selection tints an editable file, so leave the schema view when one
// arrives.
watch(
  () => props.selectedElementId,
  (id) => {
    if (id) viewingSchema.value = false;
  },
);

function selectFile(name: string) {
  viewingSchema.value = false;
  emit("setActive", name);
}

// A sentinel that stands in for the read-only schema tab within the cycle order,
// since it has no entry in `files`.
const SCHEMA_TAB = "\0schema";

// Ctrl+Tab / Ctrl+Shift+Tab move between tabs, matching the visible bar order:
// the editable files followed by the schema tab, wrapping at both ends. Note:
// Chrome and Firefox reserve Ctrl+Tab for their own tab switching and may consume
// it before the page sees it, in which case this is a no-op.
function cycleTab(event: KeyboardEvent) {
  if (!event.ctrlKey || event.key !== "Tab") return;
  event.preventDefault();
  const order = [...props.files.map((f) => f.name), SCHEMA_TAB];
  const current = viewingSchema.value ? SCHEMA_TAB : props.activeFile;
  const at = order.indexOf(current);
  const step = event.shiftKey ? -1 : 1;
  const next = order[(at + step + order.length) % order.length];
  if (next === SCHEMA_TAB) viewingSchema.value = true;
  else selectFile(next);
}

onMounted(() => window.addEventListener("keydown", cycleTab));
onBeforeUnmount(() => window.removeEventListener("keydown", cycleTab));

// Rename via the shared modal. The composable re-validates and ignores an invalid
// or colliding name.
async function promptRename(name: string) {
  const next = await promptDialog({
    title: "Rename file",
    defaultValue: name,
    confirmLabel: "Rename",
  });
  if (next && next !== name) emit("renameFile", name, next);
}

const tab =
  "flex items-center gap-1.5 border-r border-b-2 border-slate-800 px-3 py-2 font-mono text-xs cursor-pointer text-slate-500 aria-selected:border-b-amber-500 aria-selected:text-slate-200";
// Icon-only tab-bar action (Format, Save); tooltip carries the name.
const iconButton =
  "flex h-7 w-7 items-center justify-center rounded text-slate-400 cursor-pointer hover:bg-slate-800 hover:text-slate-200 disabled:cursor-default disabled:opacity-40";
</script>

<template>
  <div class="flex h-full flex-col overflow-hidden bg-slate-900 text-slate-200">
    <div class="flex items-stretch overflow-x-auto border-b border-slate-800">
      <ProjectSwitcher />
      <button
        v-for="file in files"
        :key="file.name"
        :class="tab"
        :aria-selected="!viewingSchema && file.name === activeFile"
        @click="selectFile(file.name)"
        @dblclick="promptRename(file.name)"
        :title="'Double-click to rename'"
      >
        {{ file.name }}
        <span
          v-if="files.length > 1"
          class="rounded px-1 text-slate-600 hover:text-red-400"
          role="button"
          title="Close file"
          @click.stop="emit('closeFile', file.name)"
        >×</span>
      </button>
      <button :class="tab" title="Add file" @click="emit('addFile')">+</button>
      <button
        :class="tab"
        :aria-selected="viewingSchema"
        @click="viewingSchema = true"
      >
        schema.cue
        <span class="rounded-sm border border-slate-700 px-1 text-xs uppercase tracking-wide text-slate-500">read-only</span>
      </button>
      <div v-if="!viewingSchema" class="ml-auto flex items-center gap-0.5 pr-2">
        <button :class="iconButton" title="Format" @click="emit('format')">
          <AlignLeft class="h-4 w-4" />
        </button>
        <button
          :class="iconButton"
          title="Save"
          :disabled="saveState.status === 'saving'"
          @click="emit('save')"
        >
          <Save class="h-4 w-4" />
        </button>
      </div>
    </div>

    <div class="min-h-0 flex-1">
      <CodeEditor
        v-show="!viewingSchema"
        ref="editorRef"
        :model-value="code"
        :diagnostics="diagnostics"
        :hints="hints"
        :show-hints="showHints"
        :focus-id="selectedElementId"
        @update:model-value="emit('update:code', $event)"
        @save="emit('save')"
        @cursor="cursor = $event"
      />
      <CodeEditor
        v-show="viewingSchema"
        :model-value="schemaSource"
        read-only
      />
    </div>

    <!-- Problems strip: a slim collapsible list, each row jumps the cursor to its
         line:col. Toggled from the status-bar problem count. -->
    <div
      v-if="showProblems && problemCount"
      class="max-h-40 flex-none overflow-auto border-t border-slate-800 bg-slate-950/60 font-mono text-xs"
    >
      <button
        v-for="(d, i) in problems"
        :key="i"
        class="flex w-full items-start gap-2 px-3 py-1 text-left hover:bg-slate-800/60"
        @click="jumpTo(d.line, d.column)"
      >
        <span class="shrink-0 tabular-nums text-slate-500">{{ d.line }}:{{ d.column || 1 }}</span>
        <span :class="d.kind === 'incomplete' ? 'text-amber-300' : 'text-red-300'">{{ d.message }}</span>
      </button>
      <div
        v-if="!problems.length && error"
        class="px-3 py-1.5 leading-snug whitespace-pre-wrap text-red-300"
      >{{ error }}</div>
    </div>

    <StatusBar
      v-if="!viewingSchema"
      :save-state="saveState"
      :problem-count="problemCount"
      :cursor="cursor"
      :show-hints="showHints"
      @toggle-hints="emit('toggleHints')"
      @problems="openProblems"
    />
  </div>
</template>
