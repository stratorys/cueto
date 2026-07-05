// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// The model <-> CUE-text bridge: eval (text -> model), rewrite/flush (model ->
// files), save, and format, plus the editor x-ray (diagnostics + inlay hints).
//
// Sync ordering is deliberate and NOT driven by a watcher:
//   graph edit  -> mutate model -> rebuildGraph() + syncTextFromModel()
//   text typed  -> debounce -> runEval() -> replace(model) -> rebuildGraph()
// A text-originated eval must never clobber what the user is typing, hence the
// explicit calls.

import { ref } from "vue";
import type { Diagnostic, Hint } from "../api";
import {
  evalFiles,
  formatCue,
  fromEval,
  listVersions,
  readSeed,
  readVersion,
  rewriteFile,
  saveCue,
} from "../api";
import { edgesBody, nodeBody } from "../mapping";
import { useDiagram } from "../useDiagram";
import {
  activeFileName,
  edgeOwnerFile,
  files,
  newNodeOwner,
  ownerOf,
  primaryFile,
  provenance,
} from "./useEditorFiles";
import { rebuildGraph } from "./useGraphView";

const { diagram, replace, resetHistory } = useDiagram();

// nodeId -> owner from the last model->file flush, to compute deletions/moves.
let prevOwned = new Map<string, string>();

export const evalError = ref<string | null>(null);

// The editor x-ray: positioned diagnostics (on eval failure) and inlay hints (on
// success), from the last text-originated eval. Both are cleared whenever the CUE
// is regenerated from the graph, since they no longer match the new text until the
// next eval.
export const diagnostics = ref<Diagnostic[]>([]);
export const hints = ref<Hint[]>([]);

// Whether the editor draws inlay hints (types / optional fields). Diagnostics are
// never gated. On by default; the toggle lets a user quiet a dense x-ray.
export const showHints = ref(true);
function toggleHints() {
  showHints.value = !showHints.value;
}

// Outcome of the last /save. Reset to idle whenever the CUE content changes, so a
// "saved" badge never lingers over text that no longer matches the stored version.
export type SaveState =
  | { status: "idle" }
  | { status: "saving" }
  | { status: "saved"; version: string }
  | { status: "error" };
export const saveState = ref<SaveState>({ status: "idle" });

// A graph edit invalidates the CUE text. The canvas is already updated from the
// model (instant); the file text follows via a debounced, per-file /rewrite so a
// burst of edits (a drag) collapses into one round-trip. Kept synchronous in name
// and effect for its ~15 call sites; only the text regeneration is deferred.
let syncTimer: ReturnType<typeof setTimeout> | undefined;
export function syncTextFromModel() {
  saveState.value = { status: "idle" };
  // The regenerated text invalidates the x-ray until the next eval.
  diagnostics.value = [];
  hints.value = [];
  clearTimeout(syncTimer);
  // Regenerate the file text, then re-eval for hints only. Chaining both onto the
  // one timer means a burst of edits (a drag) collapses into a single rewrite + a
  // single eval.
  syncTimer = setTimeout(() => void syncFilesFromModel().then(refreshXray), 150);
}

// Hints-only re-eval after a graph edit. Reads the regenerated file text and
// writes ONLY diagnostics/hints - never the model, graph, or provenance - so the
// text->model direction the sync ordering forbids after graph edits stays closed.
// Graph-generated CUE is always valid, so this only ever populates hints, never
// errors; on any failure the x-ray simply stays empty.
async function refreshXray() {
  const result = await evalFiles(files.value);
  if (!result.ok) return;
  diagnostics.value = [];
  hints.value = result.hints;
}

// Write the model back into the owning files via /rewrite, preserving each file's
// hand-written CUE. Only files with upserts, deletions, or the edge list are
// touched. Deletions come from the previous flush's ownership snapshot, so a node
// the model does not know about (e.g. hand-typed but not yet eval'd) is never
// removed.
async function syncFilesFromModel() {
  const model = diagram.value;
  const owner = new Map<string, string>();
  for (const node of model.nodes) owner.set(node.id, ownerOf(node));
  const edgeFile = edgeOwnerFile();

  const upserts = new Map<string, Record<string, string>>();
  for (const node of model.nodes) {
    const file = owner.get(node.id)!;
    const bucket = upserts.get(file) ?? {};
    bucket[node.id] = nodeBody(node);
    upserts.set(file, bucket);
  }

  const deletes = new Map<string, string[]>();
  for (const [id, prevFile] of prevOwned) {
    if (owner.get(id) !== prevFile) {
      deletes.set(prevFile, [...(deletes.get(prevFile) ?? []), id]);
    }
  }

  for (const file of files.value) {
    const nodes = upserts.get(file.name) ?? {};
    const dels = deletes.get(file.name) ?? [];
    const isEdgeFile = file.name === edgeFile;
    if (Object.keys(nodes).length === 0 && dels.length === 0 && !isEdgeFile) continue;
    const result = await rewriteFile({
      name: file.name,
      content: file.text,
      nodes,
      deletes: dels.length ? dels : undefined,
      edges: isEdgeFile ? edgesBody(model.edges) : undefined,
    });
    if (result.ok) {
      const target = files.value.find((f) => f.name === file.name);
      if (target) target.text = result.content;
    } else {
      evalError.value = result.error;
    }
  }
  prevOwned = owner;
}

// Typing in the CUE pane edits the active file, then debounces a re-evaluation of
// the whole package.
let evalTimer: ReturnType<typeof setTimeout> | undefined;
function onCueEdit(value: string) {
  const target = files.value.find((f) => f.name === activeFileName.value);
  if (target) target.text = value;
  saveState.value = { status: "idle" };
  clearTimeout(evalTimer);
  evalTimer = setTimeout(runEval, 400);
}

export async function runEval() {
  const result = await evalFiles(files.value);
  if (!result.ok) {
    evalError.value = result.error;
    diagnostics.value = result.diagnostics;
    hints.value = [];
    return;
  }
  evalError.value = null;
  diagnostics.value = [];
  hints.value = result.hints;
  provenance.value = result.provenance;
  // Eval is now authoritative for ownership, so drop the creation-time overrides.
  newNodeOwner.clear();
  replace(fromEval(result.diagram, result.provenance));
  // Seed the flush baseline from the fresh model so the next model->file flush
  // computes deletes against what eval just produced, not a stale snapshot.
  prevOwned = new Map(diagram.value.nodes.map((node) => [node.id, ownerOf(node)]));
  rebuildGraph();
}

// Load the persisted diagram on mount: the newest saved version if any exist,
// else the on-disk seed data.cue. The text goes through runEval - the same
// text -> model -> canvas pipeline typing uses - then history is cleared so the
// first Undo can't revert to the sample the store was seeded with. On a total
// read failure (backend unreachable) the sample seed is left in place as an
// offline fallback.
export async function loadInitialDiagram() {
  let text: string | null = null;
  const list = await listVersions();
  if (list.ok && list.versions.length) {
    const version = await readVersion(list.versions[0].version);
    if (version.ok) text = version.data;
  }
  if (text === null) {
    const seed = await readSeed();
    if (seed.ok) text = seed.data;
  }
  if (text === null) return;
  files.value = [{ name: "data.cue", text }];
  activeFileName.value = "data.cue";
  await runEval();
  resetHistory();
}

// Persist the current CUE as an immutable version. The backend re-validates, so
// invalid text surfaces the same diagnostics as a live eval rather than saving.
async function save() {
  saveState.value = { status: "saving" };
  // Versions remain single-file: persist the primary data.cue.
  const primary = files.value.find((f) => f.name === primaryFile());
  const result = await saveCue(primary?.text ?? "");
  if (!result.ok) {
    evalError.value = result.error;
    diagnostics.value = result.diagnostics;
    hints.value = [];
    saveState.value = { status: "error" };
    return;
  }
  saveState.value = { status: "saved", version: result.version };
}

// Reformat the CUE text in place via `cue fmt`. Semantics are unchanged, so no
// re-eval is triggered; a parse error surfaces in the error pane.
async function format() {
  const target = files.value.find((f) => f.name === activeFileName.value);
  if (!target) return;
  const result = await formatCue(target.text);
  if (!result.ok) {
    evalError.value = result.error;
    diagnostics.value = result.diagnostics;
    hints.value = [];
    return;
  }
  target.text = result.formatted;
  saveState.value = { status: "idle" };
  // Reformatted text drops the old x-ray positions; next eval refreshes them.
  diagnostics.value = [];
  hints.value = [];
}

export function useCueSync() {
  return {
    evalError,
    diagnostics,
    hints,
    showHints,
    toggleHints,
    onCueEdit,
    save,
    format,
    saveState,
    loadInitialDiagram,
  };
}
