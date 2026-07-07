<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// The onboarding / home hub. Shown when no project is open (empty root or no saved
// pick), and reachable at any time from the editor's Home button (atHome). Lets the
// user load an existing project, create one with an inline validated form, and read
// how the app works. A project is a git repository with its own CUE module; creating
// one git-inits a new module under the projects root and opens it, at which point App
// switches to the Editor. Self-contained - it drives the shared useProjects()
// singleton directly, same as ProjectSwitcher.
import { computed, ref } from "vue";
import {
  ArrowLeft,
  Boxes,
  Check,
  FolderGit2,
  FolderPlus,
  GitBranch,
  SquareTerminal,
} from "lucide-vue-next";
import { useProjects } from "../composables/useProjects";

const { projects, currentProjectId, currentProject, switchProject, createProject, leaveHome } =
  useProjects();

const name = ref("");
const creating = ref(false);
// A backend failure from the last create attempt (invalid name 400, collision 409).
const serverError = ref<string | null>(null);

// Client-side mirror of the backend slugify (projects.go): lower-case, non-alnum runs
// collapsed to one hyphen, ends trimmed. Used only for live validation and the "will
// be created as" hint; the backend stays authoritative for the actual id.
function slugify(input: string): string {
  let out = "";
  let lastHyphen = false;
  for (const ch of input.trim().toLowerCase()) {
    if ((ch >= "a" && ch <= "z") || (ch >= "0" && ch <= "9")) {
      out += ch;
      lastHyphen = false;
    } else if (!lastHyphen && out.length > 0) {
      out += "-";
      lastHyphen = true;
    }
  }
  return out.replace(/^-+|-+$/g, "");
}

const slug = computed(() => slugify(name.value));
const duplicate = computed(() => projects.value.some((p) => p.id === slug.value));
// A name that is non-empty but has no usable characters (e.g. "!!!") slugs to "".
const unusable = computed(() => name.value.trim() !== "" && slug.value === "");

// The live validation message, if any. Empty input shows nothing (the button is just
// disabled); a typed-but-invalid name explains why.
const validation = computed(() => {
  if (duplicate.value) return "A project with that name already exists.";
  if (unusable.value) return "Use letters or numbers.";
  return null;
});

const canCreate = computed(() => !creating.value && slug.value !== "" && !duplicate.value);

async function submit() {
  if (!canCreate.value) return;
  creating.value = true;
  serverError.value = null;
  const result = await createProject(name.value);
  creating.value = false;
  if (!result.ok) {
    serverError.value = result.error;
    return;
  }
  // Success: createProject opened the new project and left the hub, so this view
  // unmounts. Reset the field for good measure.
  name.value = "";
}

// Clear any stale error as the user edits the name.
function onInput() {
  serverError.value = null;
}

// Open a project from the list. Clicking the already-open one just returns to the
// editor (switchProject early-returns on the current id, so it wouldn't leave home).
function open(id: string) {
  if (id === currentProjectId.value) leaveHome();
  else void switchProject(id);
}
</script>

<template>
  <div class="flex h-full w-full items-center justify-center overflow-y-auto bg-slate-50 p-6">
    <div class="w-full max-w-2xl">
      <!-- Intro + back-to-editor (only when a project is already open). -->
      <div class="mb-5 flex items-center gap-2.5">
        <FolderGit2 class="h-6 w-6 text-amber-500" />
        <h1 class="font-mono text-lg font-semibold text-slate-800">cueto</h1>
        <button
          v-if="currentProject"
          type="button"
          class="ml-auto flex items-center gap-1.5 rounded px-2.5 py-1 text-xs font-medium text-slate-500 hover:bg-slate-200 hover:text-slate-700"
          @click="leaveHome"
        >
          <ArrowLeft class="h-3.5 w-3.5" />
          Back to editor
        </button>
      </div>
      <p class="mb-6 text-sm text-slate-600">
        Design diagrams whose source of truth is CUE. Open a project to start, or create one - cueto
        will git-init a new module for it.
      </p>

      <!-- Explainer: how the app works -->
      <ul class="mb-8 space-y-3">
        <li class="flex gap-3">
          <Boxes class="mt-0.5 h-4 w-4 shrink-0 text-slate-400" />
          <span class="text-sm text-slate-600">
            <span class="font-medium text-slate-800">CUE is the source of truth.</span>
            You write CUE; the diagram is evaluated from it and stays in sync as you type.
          </span>
        </li>
        <li class="flex gap-3">
          <FolderGit2 class="mt-0.5 h-4 w-4 shrink-0 text-slate-400" />
          <span class="text-sm text-slate-600">
            <span class="font-medium text-slate-800">Two-way editing.</span>
            Drag and edit nodes on the canvas; cueto rewrites your CUE, preserving hand-written code
            and comments.
          </span>
        </li>
        <li class="flex gap-3">
          <GitBranch class="mt-0.5 h-4 w-4 shrink-0 text-slate-400" />
          <span class="text-sm text-slate-600">
            <span class="font-medium text-slate-800">Git-backed history.</span>
            Every project is a git repository with its own CUE module; saves write real files. cueto
            never commits for you.
          </span>
        </li>
        <li class="flex gap-3">
          <SquareTerminal class="mt-0.5 h-4 w-4 shrink-0 text-slate-400" />
          <span class="text-sm text-slate-600">
            <span class="font-medium text-slate-800">REPL.</span>
            Evaluate CUE expressions against your live data in the bottom pane.
          </span>
        </li>
      </ul>

      <!-- Existing projects -->
      <div v-if="projects.length" class="mb-6">
        <h2 class="mb-2 font-mono text-xs uppercase tracking-wide text-slate-400">
          Open a project
        </h2>
        <div class="overflow-hidden rounded-md border border-slate-200 bg-white">
          <button
            v-for="p in projects"
            :key="p.id"
            type="button"
            class="flex w-full items-center gap-2.5 border-b border-slate-100 px-4 py-2.5 text-left last:border-b-0 hover:bg-slate-50"
            @click="open(p.id)"
          >
            <Check
              class="h-4 w-4 shrink-0"
              :class="p.id === currentProjectId ? 'text-amber-500' : 'text-transparent'"
            />
            <span class="truncate font-mono text-sm text-slate-700">{{ p.name }}</span>
          </button>
        </div>
      </div>

      <!-- Create: inline validated form -->
      <div>
        <h2 class="mb-2 font-mono text-xs uppercase tracking-wide text-slate-400">New project</h2>
        <form class="flex items-start gap-2" @submit.prevent="submit">
          <div class="flex-1">
            <input
              v-model="name"
              type="text"
              placeholder="Project name"
              autocomplete="off"
              class="w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm text-slate-800 placeholder:text-slate-400 focus:border-amber-500 focus:outline-none"
              :class="validation ? 'border-red-400' : ''"
              @input="onInput"
            />
            <!-- One hint line: validation error, backend error, else the derived id. -->
            <p v-if="validation" class="mt-1 text-xs text-red-500">{{ validation }}</p>
            <p v-else-if="serverError" class="mt-1 text-xs text-red-500">{{ serverError }}</p>
            <p v-else-if="slug" class="mt-1 font-mono text-xs text-slate-400">
              Creates <span class="text-slate-500">{{ slug }}/</span>
            </p>
          </div>
          <button
            type="submit"
            :disabled="!canCreate"
            class="flex items-center gap-2 rounded-md bg-amber-500 px-4 py-2 text-sm font-medium text-white hover:bg-amber-600 disabled:cursor-default disabled:opacity-40"
          >
            <FolderPlus class="h-4 w-4" />
            {{ creating ? "Creating…" : "Create" }}
          </button>
        </form>
      </div>
    </div>
  </div>
</template>
