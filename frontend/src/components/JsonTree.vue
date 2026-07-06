<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// Recursive syntax-colored JSON renderer for REPL output. Colors mirror the CUE
// editor's cueHighlightStyle so the REPL reads as one surface. Containers past a
// few entries (or nested deep) render collapsed behind a ▸/▾ toggle; primitives
// and short containers render inline.
import { computed, ref } from "vue";

const props = withDefaults(defineProps<{ value: unknown; keyName?: string; depth?: number }>(), {
  depth: 0,
});

type Kind = "string" | "number" | "boolean" | "null" | "array" | "object";

const kind = computed<Kind>(() => {
  const v = props.value;
  if (v === null) return "null";
  if (Array.isArray(v)) return "array";
  const t = typeof v;
  if (t === "object") return "object";
  if (t === "number") return "number";
  if (t === "boolean") return "boolean";
  return "string";
});

const isContainer = computed(() => kind.value === "array" || kind.value === "object");

const entries = computed<[string, unknown][]>(() => {
  if (kind.value === "array") return (props.value as unknown[]).map((v, i) => [String(i), v]);
  if (kind.value === "object") return Object.entries(props.value as Record<string, unknown>);
  return [];
});

const empty = computed(() => isContainer.value && entries.value.length === 0);
const openBracket = computed(() => (kind.value === "array" ? "[" : "{"));
const closeBracket = computed(() => (kind.value === "array" ? "]" : "}"));

// Show the immediate answer, but collapse large or deeply nested containers so a
// big result doesn't flood the log.
const open = ref(props.depth <= 1 && entries.value.length <= 6);

const primitiveText = computed(() =>
  kind.value === "string" ? JSON.stringify(props.value) : String(props.value),
);
const primitiveClass = computed(() => {
  switch (kind.value) {
    case "string":
      return "text-[#86efac]";
    case "number":
      return "text-[#fca5a5]";
    case "boolean":
    case "null":
      return "text-[#c4b5fd]";
    default:
      return "";
  }
});
</script>

<template>
  <div class="leading-5">
    <template v-if="keyName !== undefined">
      <span class="text-[#93c5fd]">{{ keyName }}</span
      ><span class="text-[#64748b]">: </span>
    </template>

    <span v-if="!isContainer" :class="primitiveClass">{{ primitiveText }}</span>

    <span v-else-if="empty" class="text-[#64748b]">{{ openBracket }}{{ closeBracket }}</span>

    <template v-else>
      <button class="select-none text-slate-500 hover:text-slate-300" @click="open = !open">
        {{ open ? "▾" : "▸" }}
      </button>
      <span class="text-[#64748b]">{{ openBracket }}</span>
      <span v-if="!open" class="text-slate-600">
        … {{ entries.length }} {{ kind === "array" ? "items" : "keys" }} {{ closeBracket }}
      </span>
      <div v-if="open" class="ml-1 border-l border-slate-800 pl-3">
        <JsonTree
          v-for="[k, v] in entries"
          :key="k"
          :value="v"
          :key-name="kind === 'object' ? k : undefined"
          :depth="depth + 1"
        />
      </div>
      <span v-if="open" class="text-[#64748b]">{{ closeBracket }}</span>
    </template>
  </div>
</template>
