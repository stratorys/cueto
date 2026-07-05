<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import CodeEditor from "./CodeEditor.vue";
import type { SaveState } from "../composables/useDiagramCanvas";
import type { EditorFile } from "../model";
import type { Diagnostic, Hint } from "../api";
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

// Rename via a prompt (POC-simple). The composable re-validates and ignores an
// invalid or colliding name.
function promptRename(name: string) {
  const next = window.prompt("Rename file", name);
  if (next && next !== name) emit("renameFile", name, next);
}

const tab =
  "flex items-center gap-1.5 border-r border-b-2 border-slate-800 px-3 py-2 font-mono text-xs cursor-pointer text-slate-500 aria-selected:border-b-amber-500 aria-selected:text-slate-200";
const button =
  "rounded border border-slate-700 px-2 py-0.5 font-mono text-xs text-slate-300 cursor-pointer hover:border-amber-500 disabled:cursor-default disabled:opacity-40";
</script>

<template>
  <div class="flex h-full flex-col overflow-hidden bg-slate-900 text-slate-200">
    <div class="flex items-stretch overflow-x-auto border-b border-slate-800">
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
      <div class="ml-auto flex items-center gap-2 pr-2.5">
        <template v-if="!viewingSchema">
          <button
            :class="button"
            :aria-pressed="showHints"
            :title="showHints ? 'Hide type hints' : 'Show type hints'"
            @click="emit('toggleHints')"
          >{{ showHints ? "Hints on" : "Hints off" }}</button>
          <button :class="button" @click="emit('format')">Format</button>
          <button
            :class="button"
            :disabled="saveState.status === 'saving'"
            @click="emit('save')"
          >Save</button>
          <span v-if="saveState.status === 'saving'" class="font-mono text-xs text-slate-500">Saving…</span>
          <span
            v-else-if="saveState.status === 'saved'"
            class="font-mono text-xs text-emerald-400"
            :title="saveState.version"
          >Saved {{ saveState.version.slice(0, 7) }}</span>
          <span v-else-if="saveState.status === 'error'" class="font-mono text-xs text-red-400">Save failed</span>
        </template>
        <span
          v-if="error"
          class="rounded-sm bg-red-500/15 px-2 py-0.5 font-mono text-xs text-red-400"
        >invalid</span>
        <span
          v-else
          class="rounded-sm bg-emerald-500/15 px-2 py-0.5 font-mono text-xs text-emerald-400"
        >valid</span>
      </div>
    </div>

    <div class="min-h-0 flex-1">
      <CodeEditor
        v-show="!viewingSchema"
        :model-value="code"
        :diagnostics="diagnostics"
        :hints="hints"
        :show-hints="showHints"
        :focus-id="selectedElementId"
        @update:model-value="emit('update:code', $event)"
        @save="emit('save')"
      />
      <CodeEditor
        v-show="viewingSchema"
        :model-value="schemaSource"
        read-only
      />
    </div>

    <pre
      v-if="error"
      class="m-0 max-h-72 flex-none overflow-auto border-t border-red-900 bg-red-950 px-3 py-2.5 font-mono text-xs leading-snug whitespace-pre-wrap text-red-300"
    >{{ error }}</pre>
  </div>
</template>
