<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
defineProps<{ canUndo: boolean; canRedo: boolean }>();
defineEmits<{ undo: []; redo: []; layout: [] }>();

const button =
  "rounded-md border border-slate-300 bg-white px-2.5 py-1 text-sm text-slate-800 cursor-pointer hover:border-amber-500 disabled:cursor-default disabled:opacity-40";
</script>

<template>
  <div class="flex items-center gap-2">
    <button :class="button" :disabled="!canUndo" @click="$emit('undo')">Undo</button>
    <button :class="button" :disabled="!canRedo" @click="$emit('redo')">Redo</button>
    <button :class="button" title="Tidy the diagram with auto-layout" @click="$emit('layout')">
      Auto-layout
    </button>
  </div>
</template>
