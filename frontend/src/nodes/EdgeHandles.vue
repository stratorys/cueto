<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
import { computed } from "vue";
import { Handle, Position } from "@vue-flow/core";
import { connecting, hoveredNodeId } from "../composables/useDiagramCanvas";

const props = defineProps<{ nodeId: string }>();

// Force the handles visible without a CSS :hover: connect mode reveals every
// node's handles; the line tool reveals only the node under the cursor.
const revealed = computed(() => connecting.value || hoveredNodeId.value === props.nodeId);

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
    :id="h.id"
    :key="h.id"
    type="source"
    :position="h.position"
    class="cursor-crosshair rounded-md! border-0! bg-amber-500/30! opacity-0 transition-opacity group-hover:opacity-100"
    :class="[h.size, revealed ? 'opacity-100!' : '']"
  />
</template>
