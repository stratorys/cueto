<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// Property editor for the selected node or edge: label, visual type/kind, edge
// direction, and edge cardinality. Every change commits through useDiagramCanvas,
// which re-serializes the CUE.
import { computed } from "vue";
import type { EdgeCard } from "../model";
import type { TraceEntry } from "../api";
import { commitNodeLabel, useDiagramCanvas } from "../composables/useDiagramCanvas";
import { isInferredView, traceById } from "../composables/useCueSync";
import { fieldValue } from "../eventTarget";
import LegendPanel from "./LegendPanel.vue";

const {
  selectedElement,
  diagram,
  commitNodeType,
  commitEdgeKind,
  commitEdgeReverse,
  commitEdgeGovernance,
} = useDiagramCanvas();

// The id of whatever is selected, matched directly against the inference trace (node
// ids and edge ids are the same strings the backend traces).
const selectedId = computed(() => {
  const el = selectedElement.value;
  if (!el) return "";
  return el.kind === "node" ? el.node.id : el.edge.id;
});

// The trace entry explaining the selected element, when the view was derived. Drives
// the read-only "why is this here" block.
const why = computed(() => traceById.value.get(selectedId.value));

// Human-readable name for each detection rule, for the "why" block.
const RULE_LABEL: Record<TraceEntry["rule"], string> = {
  registry: "Registry",
  "key-set-ref": "Key-set reference",
  "attr-ref": "@ref attribute",
};

// A derived element has no source text to splice edits back into, so its properties are
// read-only. Declared views stay fully editable.
const readOnly = isInferredView;

// Narrowed views so the template stays type-safe without discriminating inline.
const node = computed(() =>
  selectedElement.value?.kind === "node" ? selectedElement.value.node : null,
);
const edge = computed(() =>
  selectedElement.value?.kind === "edge" ? selectedElement.value.edge : null,
);

// A node's display name for edge endpoints; falls back to its id when unlabeled.
function nodeLabel(id: string): string {
  return diagram.value.nodes.find((n) => n.id === id)?.label || id;
}

// The plain visual node types the type picker offers. Excludes table/container,
// whose payloads (columns / child links) make free switching unsafe; the picker
// is hidden unless the selected node is already one of these.
const NODE_TYPES = ["entity", "process", "decision", "shape"] as const;
const EDGE_KINDS = ["relation", "arrow", "inherit", "line"] as const;
const canRetype = computed(() => NODE_TYPES.some((t) => t === node.value?.type));

// Option list mirrors the schema union in model.ts so the dropdown cannot drift.
const CARDS: EdgeCard[] = ["1-1", "1-n", "n-n"];

// Narrow a <select> value to one of a typed option list, or undefined when blank
// or not a member of the list.
function pick<T extends string>(event: Event, allowed: readonly T[]): T | undefined {
  const value = fieldValue(event);
  return value === undefined ? undefined : allowed.find((option) => option === value);
}

function setEdge(patch: Parameters<typeof commitEdgeGovernance>[1]) {
  if (edge.value) commitEdgeGovernance(edge.value.id, patch);
}
function setType(event: Event) {
  const type = pick(event, NODE_TYPES);
  if (node.value && type) commitNodeType(node.value.id, type);
}
function setKind(event: Event) {
  const kind = pick(event, EDGE_KINDS);
  if (edge.value && kind) commitEdgeKind(edge.value.id, kind);
}
</script>

<template>
  <div class="flex flex-col gap-4 p-4 text-sm">
    <!-- Nothing selected: show the derived-diagram legend, else the edit prompt. -->
    <template v-if="!node && !edge">
      <LegendPanel v-if="readOnly" class="!p-0" />
      <p v-else class="text-slate-400">
        Select a node or edge on the canvas to edit its properties.
      </p>
    </template>

    <!-- Node: label and visual type. -->
    <template v-else-if="node">
      <div class="flex flex-col gap-0.5">
        <span class="font-medium text-slate-700">{{ node.label || node.id }}</span>
        <span class="text-xs text-slate-400">
          {{ node.shape ?? node.type }}
          <span class="font-mono">· {{ node.id }}</span>
        </span>
      </div>

      <!-- Why this element exists, when it was inferred rather than authored. -->
      <div
        v-if="why"
        class="flex flex-col gap-1 rounded border border-slate-200 bg-slate-50 px-2.5 py-2"
      >
        <span class="text-xs font-medium text-slate-500">Why is this here</span>
        <span class="text-slate-700">{{ RULE_LABEL[why.rule] }}</span>
        <span class="font-mono text-xs text-slate-500">{{ why.detail }}</span>
      </div>

      <label class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Label</span>
        <input
          class="rounded border border-slate-300 px-2 py-1 disabled:bg-slate-100 disabled:text-slate-400"
          placeholder="node label"
          :value="node.label"
          :disabled="readOnly"
          @change="commitNodeLabel(node.id, fieldValue($event) ?? '')"
        />
      </label>

      <label v-if="canRetype" class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Type</span>
        <select
          class="rounded border border-slate-300 px-2 py-1 disabled:bg-slate-100 disabled:text-slate-400"
          :value="node.type"
          :disabled="readOnly"
          @change="setType($event)"
        >
          <option v-for="t in NODE_TYPES" :key="t" :value="t">{{ t }}</option>
        </select>
      </label>

      <p v-if="readOnly" class="text-xs text-slate-400">
        Derived from your data - view only. Edit the schema or data to change it.
      </p>
    </template>

    <!-- Edge: visual kind, direction, and cardinality. -->
    <template v-else-if="edge">
      <div class="flex flex-col gap-0.5">
        <span class="font-medium text-slate-700">
          {{ nodeLabel(edge.source) }} → {{ nodeLabel(edge.target) }}
        </span>
        <span class="text-xs text-slate-400">{{ edge.kind }} edge</span>
      </div>

      <!-- Why this edge exists, when it was inferred from a reference field. -->
      <div
        v-if="why"
        class="flex flex-col gap-1 rounded border border-slate-200 bg-slate-50 px-2.5 py-2"
      >
        <span class="text-xs font-medium text-slate-500">Why is this here</span>
        <span class="text-slate-700">{{ RULE_LABEL[why.rule] }}</span>
        <span class="font-mono text-xs text-slate-500">{{ why.detail }}</span>
      </div>

      <button
        type="button"
        class="flex items-center justify-center gap-1.5 rounded border border-slate-300 px-2 py-1 text-xs font-medium text-slate-600 hover:bg-slate-50 disabled:cursor-not-allowed disabled:text-slate-300 disabled:hover:bg-transparent"
        :disabled="readOnly"
        @click="commitEdgeReverse(edge.id)"
      >
        ⇄ Reverse direction
      </button>

      <label class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Kind</span>
        <select
          class="rounded border border-slate-300 px-2 py-1 disabled:bg-slate-100 disabled:text-slate-400"
          :value="edge.kind"
          :disabled="readOnly"
          @change="setKind($event)"
        >
          <option v-for="k in EDGE_KINDS" :key="k" :value="k">{{ k }}</option>
        </select>
      </label>

      <label class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Cardinality</span>
        <select
          class="rounded border border-slate-300 px-2 py-1 disabled:bg-slate-100 disabled:text-slate-400"
          :value="edge.card ?? ''"
          :disabled="readOnly"
          @change="setEdge({ card: pick($event, CARDS) })"
        >
          <option value="">(unset)</option>
          <option v-for="c in CARDS" :key="c" :value="c">{{ c }}</option>
        </select>
      </label>

      <p v-if="readOnly" class="text-xs text-slate-400">
        Derived from your data - view only. Edit the schema or data to change it.
      </p>
    </template>
  </div>
</template>
