<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// The onboarding page: shown when the projects root has no projects yet. A project
// is a git repository with its own CUE module; creating one git-inits a new module
// under the projects root and opens it, at which point App switches to the Editor.
import { FolderGit2 } from "lucide-vue-next";
import { useProjects } from "../composables/useProjects";
import { promptDialog } from "../composables/useModal";

const { createProject } = useProjects();

async function newProject() {
  const name = await promptDialog({
    title: "New project",
    message: "cueto will git-init a new module for it.",
    placeholder: "Project name",
    confirmLabel: "Create",
  });
  if (name) await createProject(name);
}
</script>

<template>
  <div class="flex h-full w-full flex-col items-center justify-center gap-4 bg-slate-50 text-center">
    <FolderGit2 class="h-10 w-10 text-slate-300" />
    <p class="text-lg font-medium text-slate-700">No projects yet</p>
    <p class="max-w-sm text-sm text-slate-500">
      A project is a git repository with its own CUE module, created under your projects
      directory. Create the first one - cueto will git-init a new module for it and open
      it.
    </p>
    <button
      type="button"
      class="rounded bg-amber-500 px-4 py-2 text-sm font-medium text-white hover:bg-amber-600"
      @click="newProject"
    >
      New project
    </button>
  </div>
</template>
