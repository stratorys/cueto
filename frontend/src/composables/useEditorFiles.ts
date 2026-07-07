// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// The editable CUE file set and its tab operations. Each file is the default
// project `package main`; all unify into one diagram, and the canvas round-trips
// edits back into the file that owns each node. Module-level singleton, shared
// with the other canvas composables.

import { computed, ref } from "vue";
import type { DiagramNode, EditorFile, Provenance } from "../model";
import { useDiagram } from "../useDiagram";
import { runEval } from "./useCueSync";

const { diagram } = useDiagram();

// The editable file set of the open project (its tree of .cue files). Empty until a
// project loads (loadProject); each file is a module file that unifies into the
// diagram, and the canvas round-trips edits back into the file that owns each node.
export const files = ref<EditorFile[]>([]);
// Which file the editor is showing, and which receives canvas-created nodes.
export const activeFileName = ref("");
// The active file's text (for the single editor pane).
export const activeText = computed(
  () => files.value.find((f) => f.name === activeFileName.value)?.text ?? "",
);

// Which files are open as tabs, in tab order. The tree lists every project file
// (`files`); the tab bar shows only this open subset. Closing a tab drops a name
// here but leaves the file in `files`, so it stays in the tree and in the evaluated
// module - a real removal is a git delete of the file, not a tab close.
export const openTabs = ref<string[]>([]);

// The open files as records, in tab order, for the tab bar. A name with no matching
// file (a just-closed or renamed entry) is skipped.
export const openFiles = computed<EditorFile[]>(() =>
  openTabs.value
    .map((name) => files.value.find((f) => f.name === name))
    .filter((f): f is EditorFile => f !== undefined),
);

// Reset the open tabs to the given files (all of them), for project load.
export function openAllTabs(names: string[]) {
  openTabs.value = [...names];
}

// The last-saved text baseline per file. A file is dirty when its text diverges
// from its baseline; a file with no entry (freshly added) is dirty until a save.
// Re-seeded on project load and after each save (snapshotSaved).
export const savedText = ref<Record<string, string>>({});

// Snapshot the current file set as the saved baseline (clears dirty for all).
export function snapshotSaved() {
  savedText.value = Object.fromEntries(files.value.map((f) => [f.name, f.text]));
}

// Whether a file has unsaved edits relative to its baseline.
export function isDirty(name: string): boolean {
  const file = files.value.find((f) => f.name === name);
  if (!file) return false;
  const base = savedText.value[name];
  return base === undefined || base !== file.text;
}

// Element -> owner file, from the last eval. Drives writeback targeting.
export const provenance = ref<Provenance>({ nodes: {}, edges: "" });
// Owner file for canvas-created nodes not yet reflected in eval provenance,
// pinned at creation to the then-active file.
export const newNodeOwner = new Map<string, string>();

// The primary file that owns edges and receives fallback ownership: the first file
// in the set (the tree's first path).
export function primaryFile(): string {
  return files.value[0]?.name ?? "main.cue";
}

// The single file that owns the (unsplittable) edge list: whatever eval reported,
// else the primary file.
export function edgeOwnerFile(): string {
  return provenance.value.edges || primaryFile();
}

// Which file authors a node: its eval-provenance file, else its creation-time
// file, else the primary file.
export function ownerOf(node: DiagramNode): string {
  return node.sourceFile ?? newNodeOwner.get(node.id) ?? primaryFile();
}

// --- file tabs: add / rename / close / switch ---------------------------------

// Show a file: open a tab for it if not already open (e.g. reopening from the tree),
// then make it active.
function setActiveFile(name: string) {
  if (!openTabs.value.includes(name)) openTabs.value = [...openTabs.value, name];
  activeFileName.value = name;
}

// Add a fresh editable file (unique name) seeded with just the package clause, open
// its tab, and focus it so canvas-created nodes land there.
function addFile() {
  const taken = new Set(files.value.map((f) => f.name));
  let name = "file.cue";
  let k = 2;
  while (taken.has(name)) name = `file_${k++}.cue`;
  files.value.push({ name, text: "package main\n" });
  openTabs.value = [...openTabs.value, name];
  activeFileName.value = name;
}

// A valid editable path: one or more word segments joined by "/", ending in a .cue
// file, with the schema package dir (diagram) reserved as a first segment. Mirrors
// the backend domain guard (cue.mod cannot match the segment charset, so it is
// excluded for free), so a rename the backend would reject is refused here too.
function validFileName(name: string): boolean {
  if (!/^([A-Za-z0-9_-]+\/)*[A-Za-z0-9_-]+\.cue$/.test(name)) return false;
  return !(name.includes("/") && name.split("/")[0].toLowerCase() === "diagram");
}

// Rename a file, re-pointing ownership and re-evaluating so provenance refreshes.
function renameFile(oldName: string, newName: string) {
  if (!validFileName(newName) || files.value.some((f) => f.name === newName)) return;
  const file = files.value.find((f) => f.name === oldName);
  if (!file) return;
  file.name = newName;
  if (oldName in savedText.value) {
    const { [oldName]: base, ...rest } = savedText.value;
    savedText.value = { ...rest, [newName]: base };
  }
  openTabs.value = openTabs.value.map((n) => (n === oldName ? newName : n));
  if (activeFileName.value === oldName) activeFileName.value = newName;
  for (const node of diagram.value.nodes) {
    if (node.sourceFile === oldName) node.sourceFile = newName;
  }
  for (const [id, owner] of newNodeOwner) {
    if (owner === oldName) newNodeOwner.set(id, newName);
  }
  void runEval();
}

// Close a tab (never the last open one). The file stays in `files`, so it remains
// in the tree and in the evaluated module and can be reopened from the tree; only
// the tab view is dropped. Removing a file for real is a git delete on disk.
function closeFile(name: string) {
  if (openTabs.value.length <= 1) return;
  openTabs.value = openTabs.value.filter((n) => n !== name);
  if (activeFileName.value === name) activeFileName.value = openTabs.value[0] ?? primaryFile();
}

// Drop a file from the client set entirely, after it has been deleted on disk.
// Removes it from the project files, its open tab, and its saved baseline, and
// re-points the active file if it was showing. The on-disk delete and re-eval are
// the caller's job (useCueSync.deleteFile).
export function removeFile(name: string) {
  files.value = files.value.filter((f) => f.name !== name);
  openTabs.value = openTabs.value.filter((n) => n !== name);
  if (name in savedText.value) {
    const { [name]: _removed, ...rest } = savedText.value;
    savedText.value = rest;
  }
  if (activeFileName.value === name) activeFileName.value = openTabs.value[0] ?? primaryFile();
}

export function useEditorFiles() {
  return {
    files,
    openFiles,
    activeFileName,
    activeText,
    savedText,
    isDirty,
    setActiveFile,
    addFile,
    renameFile,
    closeFile,
  };
}
