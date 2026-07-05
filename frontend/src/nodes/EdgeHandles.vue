<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
import { Handle, Position } from "@vue-flow/core";
import { connecting } from "../composables/useDiagramCanvas";

// The four side connection handles shared by shapes, containers and tables. Each
// is one invisible bar per side that glows amber while the node is hovered (or
// while a connection drag is in progress). Dragging from a bar starts a relation.
// vue-flow's own .vue-flow__handle CSS is imported after Tailwind, so the size /
// border / background utilities need the `!` important modifier to win.
const HANDLES = [
  { id: "t", position: Position.Top, size: "w-2/3! h-3!" },
  { id: "r", position: Position.Right, size: "w-3! h-2/3!" },
  { id: "b", position: Position.Bottom, size: "w-2/3! h-3!" },
  { id: "l", position: Position.Left, size: "w-3! h-2/3!" },
];
</script>

<template>
  <Handle
    v-for="h in HANDLES"
    :key="h.id"
    :id="h.id"
    type="source"
    :position="h.position"
    class="cursor-crosshair rounded-md! border-0! bg-amber-500/30! opacity-0 transition-opacity group-hover:opacity-100"
    :class="[h.size, connecting ? 'opacity-100!' : '']"
  />
</template>
