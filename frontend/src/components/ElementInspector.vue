<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// Property editor for the selected node or edge. Authors the governance metadata
// (role/owner/region/zone on nodes; card/call/protocol/sync on edges) that the
// policy + drift harness checks - the fields the schema defines but no other
// component ever sets. Every change commits through useDiagramCanvas, which
// re-serializes the CUE so the Policy tab re-vets live.
import { computed } from "vue";
import type {
  EdgeCall,
  EdgeCard,
  EdgeProtocol,
  NodeRole,
} from "../model";
import { commitNodeLabel, useDiagramCanvas } from "../composables/useDiagramCanvas";
import { fieldValue, isChecked } from "../eventTarget";

const {
  selectedElement,
  diagram,
  commitNodeType,
  commitEdgeKind,
  commitEdgeReverse,
  commitNodeGovernance,
  commitEdgeGovernance,
} = useDiagramCanvas();

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

// Option lists mirror the schema unions in model.ts so the dropdowns cannot drift.
const ROLES: NodeRole[] = ["service", "database", "queue", "cache", "gateway", "external"];
const CARDS: EdgeCard[] = ["1-1", "1-n", "n-n"];
const CALLS: EdgeCall[] = ["calls", "reads", "writes", "publishes", "subscribes"];
const PROTOCOLS: EdgeProtocol[] = ["http", "grpc", "amqp", "sql"];
// Free-string `zone` examples from schema.cue; offered as suggestions, not enforced.
const ZONES = ["pci", "public", "dmz"];

// Narrow a <select> value to one of a typed option list, or undefined when blank
// or not a member of the list.
function pick<T extends string>(event: Event, allowed: readonly T[]): T | undefined {
  const value = fieldValue(event);
  return value === undefined ? undefined : allowed.find((option) => option === value);
}

function setNode(patch: Parameters<typeof commitNodeGovernance>[1]) {
  if (node.value) commitNodeGovernance(node.value.id, patch);
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
    <p v-if="!node && !edge" class="text-slate-400">
      Select a node or edge on the canvas to edit its governance metadata.
    </p>

    <!-- Node: domain metadata that drives policy and drift. -->
    <template v-else-if="node">
      <div class="flex flex-col gap-0.5">
        <span class="font-medium text-slate-700">{{ node.label || node.id }}</span>
        <span class="text-xs text-slate-400">
          {{ node.shape ?? node.type }}
          <span class="font-mono">· {{ node.id }}</span>
        </span>
      </div>

      <label class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Label</span>
        <input
          class="rounded border border-slate-300 px-2 py-1"
          placeholder="node label"
          :value="node.label"
          @change="commitNodeLabel(node.id, fieldValue($event) ?? '')"
        />
      </label>

      <label v-if="canRetype" class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Type</span>
        <select
          class="rounded border border-slate-300 px-2 py-1"
          :value="node.type"
          @change="setType($event)"
        >
          <option v-for="t in NODE_TYPES" :key="t" :value="t">{{ t }}</option>
        </select>
      </label>

      <label class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Role</span>
        <select
          class="rounded border border-slate-300 px-2 py-1"
          :value="node.role ?? ''"
          @change="setNode({ role: pick($event, ROLES) })"
        >
          <option value="">(unset)</option>
          <option v-for="r in ROLES" :key="r" :value="r">{{ r }}</option>
        </select>
      </label>

      <label class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Owner</span>
        <input
          class="rounded border border-slate-300 px-2 py-1"
          placeholder="team id, e.g. payments"
          :value="node.owner ?? ''"
          @change="setNode({ owner: fieldValue($event) })"
        />
      </label>

      <label class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Region</span>
        <input
          class="rounded border border-slate-300 px-2 py-1"
          placeholder="e.g. eu-west-1"
          :value="node.region ?? ''"
          @change="setNode({ region: fieldValue($event) })"
        />
      </label>

      <label class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Zone</span>
        <input
          class="rounded border border-slate-300 px-2 py-1"
          list="zone-suggestions"
          placeholder="trust boundary, e.g. pci"
          :value="node.zone ?? ''"
          @change="setNode({ zone: fieldValue($event) })"
        />
        <datalist id="zone-suggestions">
          <option v-for="z in ZONES" :key="z" :value="z" />
        </datalist>
      </label>
    </template>

    <!-- Edge: typed-relationship metadata for architecture modeling and drift. -->
    <template v-else-if="edge">
      <div class="flex flex-col gap-0.5">
        <span class="font-medium text-slate-700">
          {{ nodeLabel(edge.source) }} → {{ nodeLabel(edge.target) }}
        </span>
        <span class="text-xs text-slate-400">{{ edge.kind }} edge</span>
      </div>

      <button
        type="button"
        class="flex items-center justify-center gap-1.5 rounded border border-slate-300 px-2 py-1 text-xs font-medium text-slate-600 hover:bg-slate-50"
        @click="commitEdgeReverse(edge.id)"
      >
        ⇄ Reverse direction
      </button>

      <label class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Kind</span>
        <select
          class="rounded border border-slate-300 px-2 py-1"
          :value="edge.kind"
          @change="setKind($event)"
        >
          <option v-for="k in EDGE_KINDS" :key="k" :value="k">{{ k }}</option>
        </select>
      </label>

      <label class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Cardinality</span>
        <select
          class="rounded border border-slate-300 px-2 py-1"
          :value="edge.card ?? ''"
          @change="setEdge({ card: pick($event, CARDS) })"
        >
          <option value="">(unset)</option>
          <option v-for="c in CARDS" :key="c" :value="c">{{ c }}</option>
        </select>
      </label>

      <label class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Call</span>
        <select
          class="rounded border border-slate-300 px-2 py-1"
          :value="edge.call ?? ''"
          @change="setEdge({ call: pick($event, CALLS) })"
        >
          <option value="">(unset)</option>
          <option v-for="c in CALLS" :key="c" :value="c">{{ c }}</option>
        </select>
      </label>

      <label class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Protocol</span>
        <select
          class="rounded border border-slate-300 px-2 py-1"
          :value="edge.protocol ?? ''"
          @change="setEdge({ protocol: pick($event, PROTOCOLS) })"
        >
          <option value="">(unset)</option>
          <option v-for="p in PROTOCOLS" :key="p" :value="p">{{ p }}</option>
        </select>
      </label>

      <label class="flex items-center gap-2">
        <input
          type="checkbox"
          :checked="edge.sync ?? false"
          @change="setEdge({ sync: isChecked($event) })"
        />
        <span class="font-medium text-slate-600">Synchronous</span>
      </label>
    </template>
  </div>
</template>
