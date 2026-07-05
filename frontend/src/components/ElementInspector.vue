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
import { useDiagramCanvas } from "../composables/useDiagramCanvas";

const { selectedElement, commitNodeGovernance, commitEdgeGovernance } =
  useDiagramCanvas();

// Narrowed views so the template stays type-safe without discriminating inline.
const node = computed(() =>
  selectedElement.value?.kind === "node" ? selectedElement.value.node : null,
);
const edge = computed(() =>
  selectedElement.value?.kind === "edge" ? selectedElement.value.edge : null,
);

// Option lists mirror the schema unions in model.ts so the dropdowns cannot drift.
const ROLES: NodeRole[] = ["service", "database", "queue", "cache", "gateway", "external"];
const CARDS: EdgeCard[] = ["1-1", "1-n", "n-n"];
const CALLS: EdgeCall[] = ["calls", "reads", "writes", "publishes", "subscribes"];
const PROTOCOLS: EdgeProtocol[] = ["http", "grpc", "amqp", "sql"];
// Free-string `zone` examples from schema.cue; offered as suggestions, not enforced.
const ZONES = ["pci", "public", "dmz"];

// Value of a <select>/<input>, or undefined when blank (clears the field).
function val(event: Event): string | undefined {
  return (event.target as HTMLSelectElement | HTMLInputElement).value || undefined;
}

function setNode(patch: Parameters<typeof commitNodeGovernance>[1]) {
  if (node.value) commitNodeGovernance(node.value.id, patch);
}
function setEdge(patch: Parameters<typeof commitEdgeGovernance>[1]) {
  if (edge.value) commitEdgeGovernance(edge.value.id, patch);
}
</script>

<template>
  <div class="flex flex-col gap-4 p-4 text-sm">
    <p v-if="!node && !edge" class="text-slate-400">
      Select a node or edge on the canvas to edit its governance metadata.
    </p>

    <!-- Node: domain metadata that drives policy and drift. -->
    <template v-else-if="node">
      <div class="text-xs uppercase tracking-wide text-slate-400">
        Node <span class="font-mono normal-case text-slate-500">{{ node.id }}</span>
      </div>

      <label class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Role</span>
        <select
          class="rounded border border-slate-300 px-2 py-1"
          :value="node.role ?? ''"
          @change="setNode({ role: val($event) as NodeRole | undefined })"
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
          @change="setNode({ owner: val($event) })"
        />
      </label>

      <label class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Region</span>
        <input
          class="rounded border border-slate-300 px-2 py-1"
          placeholder="e.g. eu-west-1"
          :value="node.region ?? ''"
          @change="setNode({ region: val($event) })"
        />
      </label>

      <label class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Zone</span>
        <input
          class="rounded border border-slate-300 px-2 py-1"
          list="zone-suggestions"
          placeholder="trust boundary, e.g. pci"
          :value="node.zone ?? ''"
          @change="setNode({ zone: val($event) })"
        />
        <datalist id="zone-suggestions">
          <option v-for="z in ZONES" :key="z" :value="z" />
        </datalist>
      </label>
    </template>

    <!-- Edge: typed-relationship metadata for architecture modeling and drift. -->
    <template v-else-if="edge">
      <div class="text-xs uppercase tracking-wide text-slate-400">
        Edge <span class="font-mono normal-case text-slate-500">{{ edge.id }}</span>
      </div>

      <label class="flex flex-col gap-1">
        <span class="font-medium text-slate-600">Cardinality</span>
        <select
          class="rounded border border-slate-300 px-2 py-1"
          :value="edge.card ?? ''"
          @change="setEdge({ card: val($event) as EdgeCard | undefined })"
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
          @change="setEdge({ call: val($event) as EdgeCall | undefined })"
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
          @change="setEdge({ protocol: val($event) as EdgeProtocol | undefined })"
        >
          <option value="">(unset)</option>
          <option v-for="p in PROTOCOLS" :key="p" :value="p">{{ p }}</option>
        </select>
      </label>

      <label class="flex items-center gap-2">
        <input
          type="checkbox"
          :checked="edge.sync ?? false"
          @change="setEdge({ sync: ($event.target as HTMLInputElement).checked })"
        />
        <span class="font-medium text-slate-600">Synchronous</span>
      </label>
    </template>
  </div>
</template>
