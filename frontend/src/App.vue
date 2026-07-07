<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// Root shell: bootstrap the projects list on mount, then switch between the two
// pages - Onboarding when no project is open (empty root, or no saved pick) or when
// the user has opened the home hub over a project, the Editor otherwise. AppModal is
// mounted at this level so both pages can use the prompt/confirm dialogs.
import { onMounted } from "vue";
import Editor from "./pages/Editor.vue";
import Onboarding from "./pages/Onboarding.vue";
import AppModal from "./components/AppModal.vue";
import { useProjects } from "./composables/useProjects";

const { init: initProjects, currentProjectId, ready, atHome } = useProjects();

// Load the projects list and open the saved pick (URL/localStorage) if it still
// exists; otherwise onboarding takes over. git is the only history.
onMounted(async () => {
  await initProjects();
});
</script>

<template>
  <div class="h-screen w-screen">
    <!-- Until bootstrap finishes, render neither page so a saved project doesn't
         flash the onboarding view before it loads. -->
    <Onboarding v-if="ready && (atHome || !currentProjectId)" />
    <Editor v-else-if="ready" />
    <AppModal />
  </div>
</template>
