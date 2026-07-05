<script setup lang="ts">
import { Ban } from "lucide-vue-next";
import { commitNodeColor } from "../composables/useDiagramCanvas";
import { FILL_PRESETS, STROKE_PRESETS } from "../nodes/colorPresets";

// The fill/border color popover shared by every node type. Shown while the node
// is selected: one row of fill tints, one row of border colors. The "None"
// preset (undefined fill / transparent border) renders a Ban icon instead of a
// swatch; every other preset shows its color via an inline background (runtime
// value). Colors are persisted through commitNodeColor, whose "key in patch"
// semantics clear a field when its value is undefined.
defineProps<{ id: string }>();
</script>

<template>
  <div
    class="nodrag nopan absolute -top-2 left-1/2 z-30 flex -translate-x-1/2 -translate-y-full flex-col gap-1 rounded-lg border border-slate-200 bg-white p-1.5 shadow-lg"
    @pointerdown.stop
    @dblclick.stop
  >
    <div class="flex items-center gap-1">
      <span class="w-9 text-xs font-medium uppercase tracking-wide text-slate-400">Fill</span>
      <button
        v-for="p in FILL_PRESETS"
        :key="'f' + p.title"
        :title="p.title"
        class="flex h-4 w-4 items-center justify-center rounded-full border border-slate-300"
        :style="p.value ? { backgroundColor: p.value } : {}"
        @click="commitNodeColor(id, { fill: p.value })"
      >
        <Ban v-if="p.value === undefined" class="h-4 w-4 text-red-500" />
      </button>
    </div>
    <div class="flex items-center gap-1">
      <span class="w-9 text-xs font-medium uppercase tracking-wide text-slate-400">Border</span>
      <button
        v-for="p in STROKE_PRESETS"
        :key="'s' + p.title"
        :title="p.title"
        class="flex h-4 w-4 items-center justify-center rounded-full border border-slate-300"
        :style="p.value !== 'transparent' ? { backgroundColor: p.value } : {}"
        @click="commitNodeColor(id, { stroke: p.value })"
      >
        <Ban v-if="p.value === 'transparent'" class="h-4 w-4 text-red-500" />
      </button>
    </div>
  </div>
</template>
