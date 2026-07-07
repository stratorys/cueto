<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
import { computed, nextTick, ref } from "vue";
import { NodeResizer } from "@vue-flow/node-resizer";
import { Maximize2, Pencil } from "lucide-vue-next";
import ColorPopover from "../components/ColorPopover.vue";
import EdgeHandles from "./EdgeHandles.vue";
import {
  commitNodeLabel,
  commitNodeResize,
  setFocus,
  connecting,
} from "../composables/useDiagramCanvas";

// A container: a labeled frame that holds other nodes. Children point at it via
// their `parent` field and render inside its box (Vue Flow positions them at the
// container's position + their own; `extent: 'parent'` clips them to the frame).
// The frame itself carries no children in the DOM - it is purely the visual
// region. Double-click anywhere on the frame to drill in (the canvas focuses on
// its subtree); the header pencil renames; select to resize or recolor; the four
// side handles let relations attach to the container as a whole.
const props = defineProps<{
  id: string;
  selected?: boolean;
  data: { label?: string; fill?: string; stroke?: string };
}>();

function enter() {
  setFocus(props.id);
}

// --- inline header rename ----------------------------------------------------
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
  if (next && next !== (props.data.label ?? "")) commitNodeLabel(props.id, next);
}
function cancelEdit() {
  editing.value = false;
}

// --- colors ------------------------------------------------------------------
// fill tints the frame body; stroke sets the border. An absent field leaves the
// default (translucent body, slate/amber border) untouched.
const boxStyle = computed(() => ({
  backgroundColor: props.data.fill,
  borderColor: props.data.stroke,
}));

function onResizeEnd(event: { params: { x: number; y: number; width: number; height: number } }) {
  commitNodeResize(props.id, event.params);
}
</script>

<template>
  <div class="group relative h-full w-full" :class="{ connecting }" @dblclick.stop="enter">
    <NodeResizer
      v-if="selected"
      :min-width="120"
      :min-height="90"
      color="#d97706"
      @resize-end="onResizeEnd"
    />

    <!-- Color popover: shown while selected, one row for fill, one for border. -->
    <ColorPopover v-if="selected" :id="id" />

    <div
      class="flex h-full w-full flex-col overflow-hidden rounded-lg border-2 border-dashed bg-slate-500/5 transition-colors"
      :class="selected ? 'border-amber-500' : 'border-slate-400 group-hover:border-amber-500'"
      :style="boxStyle"
    >
      <div
        class="flex shrink-0 items-center gap-1 border-b border-slate-200 bg-white/70 px-2.5 py-1 text-xs font-semibold uppercase tracking-wide text-slate-500 backdrop-blur"
      >
        <input
          v-if="editing"
          ref="input"
          v-model="draft"
          class="nodrag nopan min-w-0 flex-1 border-none bg-transparent font-semibold uppercase tracking-wide text-slate-600 outline-none"
          @blur="commitEdit"
          @keydown.enter.prevent="commitEdit"
          @keydown.escape.prevent="cancelEdit"
          @pointerdown.stop
          @dblclick.stop
        />
        <span v-else class="min-w-0 flex-1 truncate">{{ data.label || "Container" }}</span>
        <!-- Rename this container. -->
        <button
          class="nodrag nopan flex h-5 w-5 shrink-0 items-center justify-center rounded text-slate-400 hover:bg-slate-200 hover:text-amber-600"
          title="Rename container"
          @pointerdown.stop
          @dblclick.stop
          @click.stop="startEdit"
        >
          <Pencil class="h-3 w-3" />
        </button>
        <!-- Drill into this container: the canvas focuses on its subtree. -->
        <button
          class="nodrag nopan flex h-5 w-5 shrink-0 items-center justify-center rounded text-slate-400 hover:bg-slate-200 hover:text-amber-600"
          title="Open container (or double-click the frame)"
          @pointerdown.stop
          @dblclick.stop
          @click.stop="enter"
        >
          <Maximize2 class="h-3.5 w-3.5" />
        </button>
      </div>
      <!-- Body is empty in the DOM; nested nodes float above it, positioned by
           Vue Flow. It exists so the frame reads as a region. -->
      <div class="min-h-0 flex-1" />
    </div>

    <EdgeHandles :node-id="id" />
  </div>
</template>
