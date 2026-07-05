// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Project registry and current-project navigation. Module-level singleton like the
// other canvas composables. The current project id is mirrored into the URL
// (?project=) and localStorage so a reload / shared link lands on the same project;
// switching reloads the canvas through the shared eval pipeline (loadProjectDiagram).

import { computed, ref } from "vue";
import type { ProjectMeta, ProjectSeed } from "../api";
import {
  createProject as apiCreate,
  deleteProject as apiDelete,
  listProjects,
  renameProject as apiRename,
} from "../api";
import { loadProjectDiagram } from "./useCueSync";

const STORAGE_KEY = "cueto.currentProject";

// The registered projects (newest-updated first) and the id of the open one.
// currentProjectId is exported at module level so useCueSync can read it when
// saving, without a circular use*() call (same pattern as useEditorFiles).
export const projects = ref<ProjectMeta[]>([]);
export const currentProjectId = ref("");

export const currentProject = computed(
  () => projects.value.find((p) => p.id === currentProjectId.value) ?? null,
);

// The desired project id from the URL query, else localStorage, else null.
function preferredProjectId(): string | null {
  const fromUrl = new URLSearchParams(window.location.search).get("project");
  return fromUrl || localStorage.getItem(STORAGE_KEY);
}

// Persist the current id to localStorage and reflect it in the URL (without a
// navigation), so the address bar is shareable and a reload lands on it.
function persistCurrent(): void {
  if (!currentProjectId.value) return;
  localStorage.setItem(STORAGE_KEY, currentProjectId.value);
  const url = new URL(window.location.href);
  url.searchParams.set("project", currentProjectId.value);
  window.history.replaceState(null, "", url);
}

async function refresh(): Promise<void> {
  const result = await listProjects();
  projects.value = result.ok ? result.projects : [];
}

// Bootstrap on app mount: load the list, pick the current project (URL/localStorage
// -> else the first), then load its diagram. If the backend is unreachable the seed
// sample stays on screen.
async function init(): Promise<void> {
  await refresh();
  const preferred = preferredProjectId();
  const known = (id: string | null): id is string =>
    !!id && projects.value.some((p) => p.id === id);
  currentProjectId.value = known(preferred) ? preferred : (projects.value[0]?.id ?? "");
  if (!currentProjectId.value) return;
  persistCurrent();
  await loadProjectDiagram(currentProjectId.value);
}

// Open a different project: switch the current id and reload the canvas from it.
async function switchProject(id: string): Promise<void> {
  if (!id || id === currentProjectId.value) return;
  currentProjectId.value = id;
  persistCurrent();
  await loadProjectDiagram(id);
}

async function create(name: string, seed: ProjectSeed): Promise<void> {
  const result = await apiCreate(name, seed);
  if (!result.ok) return;
  await refresh();
  await switchProject(result.project.id);
}

async function rename(id: string, name: string): Promise<void> {
  const result = await apiRename(id, name);
  if (!result.ok) return;
  await refresh();
}

async function remove(id: string): Promise<void> {
  const result = await apiDelete(id);
  if (!result.ok) return;
  const wasCurrent = id === currentProjectId.value;
  await refresh();
  if (wasCurrent) await switchProject(projects.value[0]?.id ?? "");
}

export function useProjects() {
  return {
    projects,
    currentProjectId,
    currentProject,
    init,
    switchProject,
    createProject: create,
    renameProject: rename,
    deleteProject: remove,
  };
}
