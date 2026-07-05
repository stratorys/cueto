<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// The current-project control in the CUE pane's tab bar: shows the open project's
// name and opens a dropdown to switch projects or create / rename / delete one.
// Self-contained - it drives the shared useProjects() singleton directly, so no
// props/emits are threaded through CodePane. Name entry uses the shared modal
// service. The dropdown is teleported to <body> and fixed-positioned from the
// trigger's rect so the tab bar's horizontal overflow can't clip it.
import { ref } from "vue";
import { Check, ChevronDown, FilePlus2, FolderPlus, Pencil, Trash2 } from "lucide-vue-next";
import type { ProjectSeed } from "../api";
import { useProjects } from "../composables/useProjects";
import { confirmDialog, promptDialog } from "../composables/useModal";

const { projects, currentProjectId, currentProject, switchProject, createProject, renameProject, deleteProject } =
  useProjects();

const open = ref(false);
const trigger = ref<HTMLElement | null>(null);
const pos = ref({ left: 0, top: 0 });

function toggle() {
  if (!open.value && trigger.value) {
    const rect = trigger.value.getBoundingClientRect();
    pos.value = { left: rect.left, top: rect.bottom + 2 };
  }
  open.value = !open.value;
}

function pick(id: string) {
  open.value = false;
  void switchProject(id);
}

async function create(seed: ProjectSeed) {
  open.value = false;
  const name = await promptDialog({
    title: seed === "sample" ? "New project from sample" : "New project",
    message: "Choose a name for the project.",
    placeholder: "Project name",
    confirmLabel: "Create",
  });
  if (name) void createProject(name, seed);
}

async function rename() {
  open.value = false;
  const project = currentProject.value;
  if (!project) return;
  const next = await promptDialog({
    title: "Rename project",
    defaultValue: project.name,
    confirmLabel: "Rename",
  });
  if (next && next !== project.name) void renameProject(project.id, next);
}

async function remove() {
  open.value = false;
  const project = currentProject.value;
  if (!project) return;
  const ok = await confirmDialog({
    title: "Delete project",
    message: `Delete "${project.name}"? This removes its saved history.`,
    confirmLabel: "Delete",
    danger: true,
  });
  if (ok) void deleteProject(project.id);
}

const row =
  "flex w-full items-center gap-2 px-3 py-1.5 text-left font-mono text-xs text-slate-300 cursor-pointer hover:bg-slate-800 disabled:cursor-default disabled:opacity-40";
</script>

<template>
  <div class="flex items-stretch border-r border-slate-800">
    <button
      ref="trigger"
      class="flex items-center gap-1.5 px-3 py-2 font-mono text-xs text-slate-300 cursor-pointer hover:bg-slate-800"
      title="Switch project"
      @click="toggle"
    >
      <FolderPlus class="h-3.5 w-3.5 text-slate-500" />
      <span class="max-w-40 truncate">{{ currentProject?.name ?? "Project" }}</span>
      <ChevronDown class="h-3.5 w-3.5 text-slate-500" />
    </button>

    <Teleport to="body">
      <div v-if="open">
        <!-- Backdrop: any outside click closes the dropdown. -->
        <div class="fixed inset-0 z-40" @click="open = false"></div>
        <div
          class="fixed z-50 min-w-56 rounded-md border border-slate-700 bg-slate-900 py-1 shadow-lg"
          :style="{ left: `${pos.left}px`, top: `${pos.top}px` }"
        >
          <div class="max-h-64 overflow-y-auto">
            <button v-for="p in projects" :key="p.id" :class="row" @click="pick(p.id)">
              <Check
                class="h-3.5 w-3.5 shrink-0"
                :class="p.id === currentProjectId ? 'text-amber-500' : 'text-transparent'"
              />
              <span class="truncate">{{ p.name }}</span>
            </button>
            <p v-if="!projects.length" class="px-3 py-1.5 font-mono text-xs text-slate-500">No projects</p>
          </div>

          <div class="my-1 border-t border-slate-800"></div>

          <button :class="row" @click="create('blank')">
            <FolderPlus class="h-3.5 w-3.5 shrink-0 text-slate-500" />
            New blank project
          </button>
          <button :class="row" @click="create('sample')">
            <FilePlus2 class="h-3.5 w-3.5 shrink-0 text-slate-500" />
            New from sample
          </button>
          <button :class="row" :disabled="!currentProject" @click="rename()">
            <Pencil class="h-3.5 w-3.5 shrink-0 text-slate-500" />
            Rename…
          </button>
          <button :class="row" :disabled="!currentProject || projects.length <= 1" @click="remove()">
            <Trash2 class="h-3.5 w-3.5 shrink-0 text-red-400" />
            <span class="text-red-400">Delete…</span>
          </button>
        </div>
      </div>
    </Teleport>
  </div>
</template>
