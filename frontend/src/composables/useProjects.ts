// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// The workspace projects: the list under the projects root (each a git repo plus a
// CUE module) and the open one. Module-level singleton like the other canvas
// composables. The current id is mirrored into the URL (?project=) and localStorage
// so a reload / shared link lands on the same project, set on the api layer so every
// project-scoped request targets it, and switching reloads the canvas from the
// project's files.

import { computed, ref } from "vue";
import type { ProjectMeta } from "../api";
import { createProject as apiCreate, listProjects, setProject } from "../api";
import { loadProject } from "./useCueSync";

const STORAGE_KEY = "cueto.currentProject";

// The projects (sorted by id) and the id of the open one. currentProjectId is
// exported at module level so other composables can read it without a circular
// use*() call (same pattern as useEditorFiles).
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

// Point the api layer and navigation state at a project, then load its files.
async function open(id: string): Promise<void> {
  currentProjectId.value = id;
  setProject(id);
  persistCurrent();
  await loadProject();
}

// Bootstrap on app mount: load the list, pick the current project (URL/localStorage
// -> else the first), then load it. With no projects yet, nothing is opened (the UI
// shows an empty state prompting a first project).
async function init(): Promise<void> {
  await refresh();
  const preferred = preferredProjectId();
  const known = (id: string | null): id is string =>
    !!id && projects.value.some((p) => p.id === id);
  const target = known(preferred) ? preferred : (projects.value[0]?.id ?? "");
  if (!target) return;
  await open(target);
}

// Open a different project: switch the current id and reload the canvas from it.
async function switchProject(id: string): Promise<void> {
  if (!id || id === currentProjectId.value) return;
  await open(id);
}

// Create a project (git init + scaffold + initial commit on the backend), then open
// it. A name collision surfaces as a failed result and is ignored here.
async function create(name: string): Promise<void> {
  const result = await apiCreate(name);
  if (!result.ok) return;
  await refresh();
  await open(result.project.id);
}

export function useProjects() {
  return {
    projects,
    currentProjectId,
    currentProject,
    init,
    switchProject,
    createProject: create,
  };
}
