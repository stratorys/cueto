<script setup lang="ts">
defineProps<{ canUndo: boolean; canRedo: boolean }>();
defineEmits<{ undo: []; redo: []; layout: [] }>();

const button =
  "rounded-md border border-slate-300 bg-white px-2.5 py-1 text-sm text-slate-800 cursor-pointer hover:border-amber-500 disabled:cursor-default disabled:opacity-40";
</script>

<template>
  <div class="absolute left-3 top-3 z-10 flex items-center gap-2 rounded-lg border border-slate-200 bg-white/90 px-2.5 py-1.5 shadow-sm backdrop-blur">
    <button :class="button" :disabled="!canUndo" @click="$emit('undo')">Undo</button>
    <button :class="button" :disabled="!canRedo" @click="$emit('redo')">Redo</button>
    <button :class="button" title="Tidy the diagram with auto-layout" @click="$emit('layout')">
      Auto-layout
    </button>
    <span class="ml-1 text-xs text-slate-400">
      Hover a shape's border, drag to another to connect
    </span>
  </div>
</template>
