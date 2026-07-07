<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// The legend for a derived diagram: one row per registry drawn in the rendered view,
// read from the backend-authoritative /eval legend (its node kind and node count).
// Shown only when the rendered view was inferred; a declared view carries no legend and
// this panel renders nothing. It answers "what are the boxes?" before a user selects any
// single element.
import { computed } from "vue";
import { legend } from "../composables/useCueSync";

// A small identifying palette so each registry gets a stable swatch. Colors identify,
// they do not encode meaning; assignment is by sorted registry order for determinism.
const PALETTE = [
  "bg-sky-500",
  "bg-emerald-500",
  "bg-amber-500",
  "bg-violet-500",
  "bg-rose-500",
  "bg-teal-500",
];

// Each legend entry (already registry-sorted by the backend) paired with a stable
// swatch. count is shown so a table (one node) and an instance set (many) read apart.
const rows = computed(() =>
  legend.value.map((entry, index) => ({
    ...entry,
    color: PALETTE[index % PALETTE.length],
  })),
);
</script>

<template>
  <div v-if="rows.length" class="flex flex-col gap-3 p-4 text-sm">
    <div class="flex flex-col gap-0.5">
      <span class="font-medium text-slate-700">Legend</span>
      <span class="text-xs text-slate-400">Node kinds derived from your registries.</span>
    </div>
    <ul class="flex flex-col gap-1.5">
      <li v-for="row in rows" :key="row.field" class="flex items-center gap-2">
        <span class="h-2.5 w-2.5 flex-none rounded-full" :class="row.color" />
        <span class="font-mono text-slate-700">{{ row.field }}</span>
        <span class="ml-auto text-xs tabular-nums text-slate-400">{{ row.count }}</span>
      </li>
    </ul>
  </div>
</template>
