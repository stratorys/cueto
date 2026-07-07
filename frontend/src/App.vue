<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// Root shell: bootstrap the projects list on mount, then switch between the two
// pages - Onboarding when the projects root is empty, the Editor once a project is
// open. AppModal is mounted at this level so both pages can use the prompt/confirm
// dialogs (onboarding's "New project" needs it before any editor exists).
import { onMounted } from "vue";
import Editor from "./pages/Editor.vue";
import Onboarding from "./pages/Onboarding.vue";
import AppModal from "./components/AppModal.vue";
import { useProjects } from "./composables/useProjects";

const { init: initProjects, currentProjectId } = useProjects();

// Load the projects list, pick the current one (URL/localStorage, else the first),
// and load its files. git is the only history.
onMounted(async () => {
  await initProjects();
});
</script>

<template>
  <div class="h-screen w-screen">
    <Onboarding v-if="!currentProjectId" />
    <Editor v-else />
    <AppModal />
  </div>
</template>
