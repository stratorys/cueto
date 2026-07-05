// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// The editable CUE file set and its tab operations. Each file is
// `package diagram`; all unify into one diagram, and the canvas round-trips
// edits back into the file that owns each node. Module-level singleton, shared
// with the other canvas composables.

import { computed, ref } from "vue";
import type { DiagramNode, EditorFile, Provenance } from "../model";
import { toCue } from "../mapping";
import { useDiagram } from "../useDiagram";
import { runEval } from "./useCueSync";

const { diagram } = useDiagram();

// The editable file set. Each file is `package diagram`; all unify into one
// diagram. The canvas round-trips edits back into the file that owns each node.
export const files = ref<EditorFile[]>([{ name: "data.cue", text: toCue(diagram.value) }]);
// Which file the editor is showing, and which receives canvas-created nodes.
export const activeFileName = ref("data.cue");
// The active file's text (for the single editor pane).
export const activeText = computed(
  () => files.value.find((f) => f.name === activeFileName.value)?.text ?? "",
);

// Element -> owner file, from the last eval. Drives writeback targeting.
export const provenance = ref<Provenance>({ nodes: {}, edges: "" });
// Owner file for canvas-created nodes not yet reflected in eval provenance,
// pinned at creation to the then-active file.
export const newNodeOwner = new Map<string, string>();

// The primary file that owns edges and receives fallback ownership. Prefer the
// conventional data.cue, else the first file.
export function primaryFile(): string {
  return files.value.some((f) => f.name === "data.cue")
    ? "data.cue"
    : files.value[0]?.name ?? "data.cue";
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

function setActiveFile(name: string) {
  activeFileName.value = name;
}

// Add a fresh editable file (unique name) seeded with just the package clause,
// and focus it so canvas-created nodes land there.
function addFile() {
  const taken = new Set(files.value.map((f) => f.name));
  let name = "file.cue";
  let k = 2;
  while (taken.has(name)) name = `file_${k++}.cue`;
  files.value.push({ name, text: "package diagram\n" });
  activeFileName.value = name;
}

// A valid editable filename: a bare .cue name that is not the reserved schema.cue.
// Mirrors the backend guard so a rename the backend would reject is refused here.
function validFileName(name: string): boolean {
  return /^[a-zA-Z0-9_-]+\.cue$/.test(name) && name.toLowerCase() !== "schema.cue";
}

// Rename a file, re-pointing ownership and re-evaluating so provenance refreshes.
function renameFile(oldName: string, newName: string) {
  if (!validFileName(newName) || files.value.some((f) => f.name === newName)) return;
  const file = files.value.find((f) => f.name === oldName);
  if (!file) return;
  file.name = newName;
  if (activeFileName.value === oldName) activeFileName.value = newName;
  for (const node of diagram.value.nodes) {
    if (node.sourceFile === oldName) node.sourceFile = newName;
  }
  for (const [id, owner] of newNodeOwner) {
    if (owner === oldName) newNodeOwner.set(id, newName);
  }
  void runEval();
}

// Close a file (never the last one). Its nodes leave the unified diagram on the
// next eval.
function closeFile(name: string) {
  if (files.value.length <= 1) return;
  files.value = files.value.filter((f) => f.name !== name);
  if (activeFileName.value === name) activeFileName.value = primaryFile();
  void runEval();
}

export function useEditorFiles() {
  return {
    files,
    activeFileName,
    activeText,
    setActiveFile,
    addFile,
    renameFile,
    closeFile,
  };
}
