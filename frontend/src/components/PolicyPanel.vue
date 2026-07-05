<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// Policy + drift panel. Re-vets the current CUE (debounced) and groups the
// findings: schema errors, policy-pack violations, and infra drift. Rows anchored
// to a node/edge highlight it on the canvas. The infra-drift control is added in
// the drift phase.
import { computed, onMounted, ref, watch } from "vue";
import type { Diagnostic } from "../api";
import { importCompose, vetFiles } from "../api";
import { useDiagramCanvas } from "../composables/useDiagramCanvas";
import { useHighlight } from "../composables/useHighlight";

const { files, diagram, setPolicies } = useDiagramCanvas();
const { setHighlight } = useHighlight();

// The governance packs offered as toggles. There is no pack registry - `security`
// is the sole pack, wired by hand in cue/policy_check.cue. Add packs here when the
// harness gains them.
const AVAILABLE_PACKS = ["security"];
const enabledPacks = computed(() => new Set(diagram.value.policies ?? []));

// Toggle a pack in diagram.policies; the flush to files re-triggers the vet watch.
function togglePack(pack: string, on: boolean) {
  const next = new Set(enabledPacks.value);
  if (on) next.add(pack);
  else next.delete(pack);
  setPolicies([...next]);
}

// Imported infra facts (CUE text) to check drift against; null = no drift check.
const facts = ref<string | null>(null);
const infraName = ref<string | null>(null);
const infraError = ref<string | null>(null);

// Load a docker-compose file, import it to facts, and re-vet (the facts watcher
// fires the drift check).
async function loadInfra(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = ""; // allow re-selecting the same file
  if (!file) return;
  infraError.value = null;
  const source = await file.text();
  const result = await importCompose(source);
  if (!result.ok) {
    infraError.value = result.error;
    return;
  }
  facts.value = result.facts;
  infraName.value = file.name;
}

function clearInfra() {
  facts.value = null;
  infraName.value = null;
  infraError.value = null;
}
const diagnostics = ref<Diagnostic[]>([]);
const passes = ref(true);
const error = ref<string | null>(null);

const policyDiags = computed(() => diagnostics.value.filter((d) => d.kind === "policy"));
const driftDiags = computed(() => diagnostics.value.filter((d) => d.kind === "drift"));
const schemaDiags = computed(() =>
  diagnostics.value.filter((d) => d.kind !== "policy" && d.kind !== "drift"),
);

let timer: ReturnType<typeof setTimeout> | undefined;
function scheduleVet() {
  clearTimeout(timer);
  timer = setTimeout(runVet, 400);
}

async function runVet() {
  const result = await vetFiles(files.value, facts.value ?? undefined);
  if (!result.ok) {
    error.value = result.error;
    diagnostics.value = [];
    return;
  }
  error.value = null;
  passes.value = result.passes;
  diagnostics.value = result.diagnostics;
}

function highlight(d: Diagnostic) {
  if (d.nodeId) setHighlight([d.nodeId]);
  else if (d.edgeId) setHighlight([], [d.edgeId]);
}

onMounted(runVet);
// Deep watch: file text mutates in place, so a shallow ref watch would miss edits.
watch([files, facts], scheduleVet, { deep: true });
</script>

<template>
  <div class="flex flex-col gap-4 p-4 text-sm">
    <!-- Governance packs: opt the diagram into policy checks (diagram.policies). -->
    <div class="flex flex-col gap-1 rounded border border-slate-200 p-2">
      <span class="font-medium text-slate-600">Policy packs</span>
      <label
        v-for="pack in AVAILABLE_PACKS"
        :key="pack"
        class="flex cursor-pointer items-center gap-2 text-slate-600"
      >
        <input
          type="checkbox"
          :checked="enabledPacks.has(pack)"
          @change="togglePack(pack, ($event.target as HTMLInputElement).checked)"
        />
        {{ pack }}
      </label>
    </div>

    <!-- Drift: check the diagram against a live docker-compose topology. -->
    <div class="flex flex-col gap-1 rounded border border-slate-200 p-2">
      <div class="flex items-center justify-between">
        <span class="font-medium text-slate-600">Infra drift</span>
        <button v-if="facts" class="text-xs text-slate-400 hover:text-amber-700" @click="clearInfra">
          clear
        </button>
      </div>
      <label class="cursor-pointer text-xs text-amber-700 hover:underline">
        <input type="file" accept=".yml,.yaml" class="hidden" @change="loadInfra" />
        {{ infraName ? `Loaded ${infraName}` : "Load docker-compose..." }}
      </label>
      <p v-if="infraError" class="whitespace-pre-wrap text-xs text-red-600">{{ infraError }}</p>
      <p v-else class="text-xs text-slate-400">
        Node labels must match compose service names.
      </p>
    </div>

    <p v-if="error" class="whitespace-pre-wrap text-red-600">{{ error }}</p>

    <p
      v-else-if="passes && !diagnostics.length"
      class="rounded bg-emerald-50 px-2 py-1 text-emerald-700"
    >
      Passes schema and all opted-in policies.
    </p>

    <section v-if="policyDiags.length">
      <h3 class="mb-1 font-semibold text-red-700">Policy violations</h3>
      <ul class="space-y-0.5">
        <li v-for="(d, i) in policyDiags" :key="i">
          <button class="w-full rounded px-2 py-1 text-left hover:bg-red-50" @click="highlight(d)">
            <span v-if="d.rule" class="mr-1 rounded bg-red-100 px-1 text-xs text-red-700">{{ d.rule }}</span>
            {{ d.message }}
          </button>
        </li>
      </ul>
    </section>

    <section v-if="driftDiags.length">
      <h3 class="mb-1 font-semibold text-amber-700">Drift</h3>
      <ul class="space-y-0.5">
        <li v-for="(d, i) in driftDiags" :key="i">
          <button class="w-full rounded px-2 py-1 text-left hover:bg-amber-50" @click="highlight(d)">
            {{ d.message }}
          </button>
        </li>
      </ul>
    </section>

    <section v-if="schemaDiags.length">
      <h3 class="mb-1 font-semibold text-slate-700">Schema</h3>
      <ul class="space-y-0.5">
        <li v-for="(d, i) in schemaDiags" :key="i" class="px-2 py-1 text-slate-600">
          <span v-if="d.line" class="mr-1 text-xs text-slate-400">{{ d.line }}:{{ d.column }}</span>
          {{ d.message }}
        </li>
      </ul>
    </section>
  </div>
</template>
