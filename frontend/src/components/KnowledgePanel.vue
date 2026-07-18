<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { getKnowledgeCatalog, type KnowledgeCatalog } from "../api";

const catalog = ref<KnowledgeCatalog | null>(null);
const error = ref("");
async function load() { const result = await getKnowledgeCatalog(); if (result.ok) catalog.value = result.catalog; else error.value = result.error; }
onMounted(() => void load());
</script>

<template>
  <div class="p-4 text-sm">
    <div class="mb-3 flex items-center justify-between"><h3 class="font-semibold text-slate-700">Knowledge</h3><button class="text-xs text-amber-700" @click="load">refresh</button></div>
    <p v-if="error" class="text-red-600">{{ error }}</p>
    <p v-else-if="!catalog" class="text-slate-400">Loading catalog…</p>
    <template v-else>
      <section v-for="domain in catalog.domains" :key="domain.name" class="mb-4">
        <h4 class="font-medium">{{ domain.name }}</h4><p v-if="domain.description" class="text-xs text-slate-500">{{ domain.description }}</p>
        <ul class="mt-1 text-xs text-slate-600"><li v-for="(field, name) in domain.fields" :key="name">{{ name }}: {{ field.type }}<span v-if="!field.required">?</span><span v-if="field.relation"> → {{ field.relation.domain }}</span></li></ul>
      </section>
      <section v-if="catalog.evaluations.length"><h4 class="font-medium">Named evals</h4><ul class="text-xs text-slate-600"><li v-for="item in catalog.evaluations" :key="item.name">{{ item.name }} — {{ item.description }}</li></ul></section>
    </template>
  </div>
</template>
