<script setup lang="ts">
import type { Component } from "vue";
import { Square, Circle, Diamond, Slash, Type, Table, Box, Waypoints } from "lucide-vue-next";
import type { ShapeKind, Tool } from "../model";

// Floating shape palette. A shape item can be dragged onto the canvas or clicked
// to arm a draw tool (then draw on the canvas). The table item is place-only:
// drag it onto the canvas, or click to drop one at the canvas center. Connect is
// a click-only mode toggle: it reveals node handles so a handle-to-handle drag
// makes a relation. Icons are just the button faces - placed shapes carry no icon.
defineProps<{ active: Tool | null }>();
const emit = defineEmits<{ arm: [tool: Tool]; addTable: []; addContainer: [] }>();

const items: { shape: ShapeKind; icon: Component; title: string }[] = [
  { shape: "rectangle", icon: Square, title: "Rectangle" },
  { shape: "ellipse", icon: Circle, title: "Ellipse" },
  { shape: "diamond", icon: Diamond, title: "Diamond" },
  { shape: "line", icon: Slash, title: "Line" },
  { shape: "text", icon: Type, title: "Text" },
];

function onDragStart(event: DragEvent, kind: string) {
  event.dataTransfer?.setData("application/shape", kind);
  if (event.dataTransfer) event.dataTransfer.effectAllowed = "copy";
}
</script>

<template>
  <div class="absolute left-3 top-1/2 z-10 flex -translate-y-1/2 flex-col gap-1 rounded-xl border border-slate-200 bg-white/95 p-1.5 shadow-md backdrop-blur">
    <button
      v-for="item in items"
      :key="item.shape"
      :title="item.title"
      draggable="true"
      class="flex h-9 w-9 cursor-grab items-center justify-center rounded-lg text-slate-600 transition-colors hover:bg-slate-100 active:cursor-grabbing"
      :class="active === item.shape ? 'bg-amber-100 text-amber-700 ring-1 ring-amber-400' : ''"
      @dragstart="onDragStart($event, item.shape)"
      @click="emit('arm', item.shape)"
    >
      <component :is="item.icon" class="h-5 w-5" />
    </button>
    <div class="my-0.5 h-px bg-slate-200" />
    <button
      title="Connect - drag between two node handles to make a relation"
      class="flex h-9 w-9 cursor-pointer items-center justify-center rounded-lg text-slate-600 transition-colors hover:bg-slate-100"
      :class="active === 'connect' ? 'bg-amber-100 text-amber-700 ring-1 ring-amber-400' : ''"
      @click="emit('arm', 'connect')"
    >
      <Waypoints class="h-5 w-5" />
    </button>
    <div class="my-0.5 h-px bg-slate-200" />
    <button
      title="Table"
      draggable="true"
      class="flex h-9 w-9 cursor-grab items-center justify-center rounded-lg text-slate-600 transition-colors hover:bg-slate-100 active:cursor-grabbing"
      @dragstart="onDragStart($event, 'table')"
      @click="emit('addTable')"
    >
      <Table class="h-5 w-5" />
    </button>
    <button
      title="Container"
      draggable="true"
      class="flex h-9 w-9 cursor-grab items-center justify-center rounded-lg text-slate-600 transition-colors hover:bg-slate-100 active:cursor-grabbing"
      @dragstart="onDragStart($event, 'container')"
      @click="emit('addContainer')"
    >
      <Box class="h-5 w-5" />
    </button>
  </div>
</template>
