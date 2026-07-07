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
import type { EvalErr, ProjectMeta, ProjectOk } from "../api";
import { createProject as apiCreate, listProjects, setProject } from "../api";
import { loadProject } from "./useCueSync";

const STORAGE_KEY = "cueto.currentProject";

// The projects (sorted by id) and the id of the open one. currentProjectId is
// exported at module level so other composables can read it without a circular
// use*() call (same pattern as useEditorFiles).
export const projects = ref<ProjectMeta[]>([]);
export const currentProjectId = ref("");

// Whether the initial bootstrap has finished. Gates the shell vs. the onboarding
// view: while false the app renders nothing (no first-paint flash); once true,
// an empty currentProjectId means "no project open" -> show onboarding.
export const ready = ref(false);

// Whether the onboarding hub is showing over an open project. Distinct from "no
// project open": a user with a project open can return home (goHome) to browse
// projects, read the explainer, or create one, then go back (leaveHome). Opening
// any project clears it.
export const atHome = ref(false);

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
// Opening a project always leaves the home hub.
async function open(id: string): Promise<void> {
  currentProjectId.value = id;
  atHome.value = false;
  setProject(id);
  persistCurrent();
  await loadProject();
}

// Show the onboarding hub over the current project (a Home button), and return
// from it. leaveHome is a no-op when no project is open (onboarding stays up
// because currentProjectId is empty).
function goHome(): void {
  atHome.value = true;
}
function leaveHome(): void {
  atHome.value = false;
}

// Bootstrap on app mount: load the list, then open the previously chosen project
// (URL/localStorage) if it still exists. With no saved pick - or one that no longer
// resolves - nothing is opened and the onboarding view takes over, listing the
// projects to load or offering to create one. `ready` flips once the pick (if any)
// has loaded, so the shell is never shown before its project is in.
async function init(): Promise<void> {
  await refresh();
  const preferred = preferredProjectId();
  const known = (id: string | null): id is string =>
    !!id && projects.value.some((p) => p.id === id);
  const target = known(preferred) ? preferred : "";
  if (target) await open(target);
  ready.value = true;
}

// Open a different project: switch the current id and reload the canvas from it.
async function switchProject(id: string): Promise<void> {
  if (!id || id === currentProjectId.value) return;
  await open(id);
}

// Create a project (git init + scaffold + initial commit on the backend), then open
// it. The result is returned so the caller can surface a failure (an invalid name
// is 400, a slug collision is 409) inline rather than swallowing it.
async function create(name: string): Promise<ProjectOk | EvalErr> {
  const result = await apiCreate(name);
  if (!result.ok) return result;
  await refresh();
  await open(result.project.id);
  return result;
}

export function useProjects() {
  return {
    projects,
    currentProjectId,
    currentProject,
    ready,
    atHome,
    goHome,
    leaveHome,
    init,
    switchProject,
    createProject: create,
  };
}
