<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
import { computed } from "vue";
import { Undo2, Redo2, Maximize, Network } from "lucide-vue-next";
import { fitView, viewport } from "../composables/flowStore";

defineProps<{ canUndo: boolean; canRedo: boolean }>();
defineEmits<{ undo: []; redo: []; layout: [] }>();

// Live zoom readout from the Vue Flow viewport.
const zoomPercent = computed(() => `${Math.round((viewport.value?.zoom ?? 1) * 100)}%`);

const button =
  "flex h-7 w-7 items-center justify-center rounded-md text-slate-600 cursor-pointer hover:bg-slate-100 disabled:cursor-default disabled:opacity-40";
</script>

<template>
  <div class="flex items-center gap-0.5">
    <button :class="button" title="Undo (⌘Z)" :disabled="!canUndo" @click="$emit('undo')">
      <Undo2 class="h-4 w-4" />
    </button>
    <button :class="button" title="Redo (⇧⌘Z)" :disabled="!canRedo" @click="$emit('redo')">
      <Redo2 class="h-4 w-4" />
    </button>
    <div class="mx-0.5 h-5 w-px bg-slate-200" />
    <button :class="button" title="Fit view" @click="fitView({ padding: 0.2 })">
      <Maximize class="h-4 w-4" />
    </button>
    <button :class="button" title="Tidy the diagram with auto-layout" @click="$emit('layout')">
      <Network class="h-4 w-4" />
    </button>
    <span class="min-w-[3rem] px-1 text-center font-mono text-xs text-slate-500 tabular-nums">{{ zoomPercent }}</span>
  </div>
</template>
