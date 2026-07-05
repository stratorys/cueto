<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
import type { Component } from "vue";
import {
  Square,
  Circle,
  Diamond,
  Slash,
  Type,
  Table,
  Box,
  Boxes,
  Workflow,
  GitBranch,
  Waypoints,
} from "lucide-vue-next";
import type { ShapeKind, Tool, TypedNodeType } from "../model";

// Floating shape palette. A shape item can be dragged onto the canvas or clicked
// to arm a draw tool (then draw on the canvas). The table item is place-only:
// drag it onto the canvas, or click to drop one at the canvas center. Connect is
// a click-only mode toggle: it reveals node handles so a handle-to-handle drag
// makes a relation. Icons are just the button faces - placed shapes carry no icon.
// allowConnect gates the relation tool: hidden on a data-derived diagram, where a
// drawn edge can't yet persist alongside the derived edge comprehension.
withDefaults(defineProps<{ active: Tool | null; allowConnect?: boolean }>(), {
  allowConnect: true,
});
const emit = defineEmits<{
  arm: [tool: Tool];
  addTable: [];
  addContainer: [];
  addTyped: [type: TypedNodeType];
}>();

const items: { shape: ShapeKind; icon: Component; title: string }[] = [
  { shape: "rectangle", icon: Square, title: "Rectangle" },
  { shape: "ellipse", icon: Circle, title: "Ellipse" },
  { shape: "diamond", icon: Diamond, title: "Diamond" },
  { shape: "line", icon: Slash, title: "Line" },
  { shape: "text", icon: Type, title: "Text" },
];

// Typed domain nodes: place-only (drag onto the canvas, or click to drop one at
// the center), like the table and container buttons.
const typedItems: { type: TypedNodeType; icon: Component; title: string }[] = [
  { type: "entity", icon: Boxes, title: "Entity" },
  { type: "process", icon: Workflow, title: "Process" },
  { type: "decision", icon: GitBranch, title: "Decision" },
];

function onDragStart(event: DragEvent, kind: string) {
  event.dataTransfer?.setData("application/shape", kind);
  if (event.dataTransfer) event.dataTransfer.effectAllowed = "copy";
}
</script>

<template>
  <div class="flex items-center gap-1">
    <button
      v-for="item in items"
      :key="item.shape"
      :title="item.title"
      draggable="true"
      class="flex h-8 w-8 cursor-grab items-center justify-center rounded-lg text-slate-600 transition-colors hover:bg-slate-100 active:cursor-grabbing"
      :class="active === item.shape ? 'bg-amber-100 text-amber-700 ring-1 ring-amber-400' : ''"
      @dragstart="onDragStart($event, item.shape)"
      @click="emit('arm', item.shape)"
    >
      <component :is="item.icon" class="h-5 w-5" />
    </button>
    <template v-if="allowConnect">
      <div class="mx-0.5 h-6 w-px bg-slate-200" />
      <button
        title="Connect - drag between two node handles to make a relation"
        class="flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg text-slate-600 transition-colors hover:bg-slate-100"
        :class="active === 'connect' ? 'bg-amber-100 text-amber-700 ring-1 ring-amber-400' : ''"
        @click="emit('arm', 'connect')"
      >
        <Waypoints class="h-5 w-5" />
      </button>
    </template>
    <div class="mx-0.5 h-6 w-px bg-slate-200" />
    <button
      title="Table"
      draggable="true"
      class="flex h-8 w-8 cursor-grab items-center justify-center rounded-lg text-slate-600 transition-colors hover:bg-slate-100 active:cursor-grabbing"
      @dragstart="onDragStart($event, 'table')"
      @click="emit('addTable')"
    >
      <Table class="h-5 w-5" />
    </button>
    <button
      title="Container"
      draggable="true"
      class="flex h-8 w-8 cursor-grab items-center justify-center rounded-lg text-slate-600 transition-colors hover:bg-slate-100 active:cursor-grabbing"
      @dragstart="onDragStart($event, 'container')"
      @click="emit('addContainer')"
    >
      <Box class="h-5 w-5" />
    </button>
    <div class="mx-0.5 h-6 w-px bg-slate-200" />
    <button
      v-for="item in typedItems"
      :key="item.type"
      :title="item.title"
      draggable="true"
      class="flex h-8 w-8 cursor-grab items-center justify-center rounded-lg text-slate-600 transition-colors hover:bg-slate-100 active:cursor-grabbing"
      @dragstart="onDragStart($event, item.type)"
      @click="emit('addTyped', item.type)"
    >
      <component :is="item.icon" class="h-5 w-5" />
    </button>
  </div>
</template>
