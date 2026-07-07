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

import { nextTick, ref } from "vue";
import type { Diagnostic, Hint } from "../api";
import {
  evalFiles,
  formatCue,
  fromEval,
  listVersions,
  readVersion,
  rewriteFile,
  saveCue,
} from "../api";
import { CANVAS_SENTINEL, canvasBlock, edgesBody, nodeBody, toCue } from "../mapping";
import { useDiagram } from "../useDiagram";
import { currentProjectId } from "./useProjects";
import {
  activeFileName,
  edgeOwnerFile,
  files,
  newNodeOwner,
  ownerOf,
  primaryFile,
  provenance,
  snapshotSaved,
} from "./useEditorFiles";
import { isAutoLayout, layoutAuto, rebuildGraph } from "./useGraphView";

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

// Names of every diagram view the last eval discovered, and which one is rendered.
// A module can expose zero (knowledge-only), one, or many views; the switcher only
// shows above one. activeView is passed back on the next eval so the backend
// renders that view, and is re-pinned to the rendered view after each eval so a
// view that disappears falls back to the default rather than leaving a stale tab.
export const views = ref<string[]>([]);
export const activeView = ref<string>("");

// pickActiveView mirrors the backend's default-view choice (the one named
// "diagram", else the first by name) so the switcher highlights the tab that was
// actually rendered, keeping a still-valid selection.
function pickActiveView(names: string[], current: string): string {
  if (current && names.includes(current)) return current;
  if (names.includes("diagram")) return "diagram";
  return names[0] ?? "";
}

// selectView switches the rendered view and re-evaluates. A no-op when the view is
// already active, so clicking the current tab does not thrash the canvas.
export function selectView(name: string) {
  if (name === activeView.value) return;
  activeView.value = name;
  void runEval();
}

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
// Debounce for the text->model eval (armed by onCueEdit). Declared here so a graph
// edit can cancel a pending text eval before it clobbers the fresh model.
let evalTimer: ReturnType<typeof setTimeout> | undefined;
export function syncTextFromModel() {
  saveState.value = { status: "idle" };
  // The regenerated text invalidates the x-ray until the next eval.
  diagnostics.value = [];
  hints.value = [];
  clearTimeout(syncTimer);
  // A graph edit supersedes any pending text-originated eval; drop it so a stale
  // eval firing after this edit can't overwrite the model the user just changed.
  clearTimeout(evalTimer);
  // Derived diagram: the nodes come from a CUE comprehension, so we never splice
  // them back via /rewrite. Instead regenerate only the managed trailing block of
  // hand-drawn shapes. A normal diagram round-trips every node through /rewrite.
  const flush = isAutoLayout.value ? syncCanvasBlock : syncFilesFromModel;
  // Regenerate the file text, then re-eval for hints only. Chaining both onto the
  // one timer means a burst of edits (a drag) collapses into a single write + a
  // single eval.
  syncTimer = setTimeout(() => void flush().then(refreshXray), 150);
}

// Strip the app-managed trailing region (sentinel to EOF) and trailing whitespace,
// leaving the hand-authored derivation above it untouched.
function stripCanvasBlock(text: string): string {
  const idx = text.indexOf(CANVAS_SENTINEL);
  return (idx === -1 ? text : text.slice(0, idx)).replace(/\s+$/, "");
}

// Derived-diagram write-back: regenerate the managed `diagram: nodes: { … }` block
// at the end of the primary file from the hand-drawn (coordinate-bearing) nodes.
// Derived nodes (coordinate-free) and everything above the sentinel are never
// touched, so the comprehension stays authoritative. The model already holds the
// edit, so this only rewrites text - refreshXray re-evals for hints.
async function syncCanvasBlock() {
  const primary = files.value.find((f) => f.name === primaryFile());
  if (!primary) return;
  const drawn = diagram.value.nodes.filter((n) => n.x !== undefined && n.y !== undefined);
  const base = stripCanvasBlock(primary.text);
  const block = canvasBlock(drawn);
  primary.text = block ? `${base}\n\n${block}` : `${base}\n`;
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
function onCueEdit(value: string) {
  const target = files.value.find((f) => f.name === activeFileName.value);
  if (target) target.text = value;
  saveState.value = { status: "idle" };
  clearTimeout(evalTimer);
  evalTimer = setTimeout(runEval, 400);
}

// Monotonic token so only the newest eval wins the last write. A slower earlier
// eval (or the mount-load eval racing a fast first keystroke) resolving after a
// newer one is dropped instead of overwriting the fresher model/provenance.
let evalGeneration = 0;
export async function runEval() {
  const generation = ++evalGeneration;
  const result = await evalFiles(files.value, activeView.value);
  if (generation !== evalGeneration) return;
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
  views.value = result.views;
  activeView.value = pickActiveView(result.views, activeView.value);
  // Eval is now authoritative for ownership, so drop the creation-time overrides.
  newNodeOwner.clear();
  replace(fromEval(result.diagram, result.provenance));
  // Seed the flush baseline from the fresh model so the next model->file flush
  // computes deletes against what eval just produced, not a stale snapshot.
  prevOwned = new Map(diagram.value.nodes.map((node) => [node.id, ownerOf(node)]));
  rebuildGraph();
  // A data-derived diagram carries no coordinates; lay it out into ephemeral view
  // state once the nodes have rendered (so elk can measure their card size).
  if (isAutoLayout.value) void nextTick(layoutAuto);
}

// Load a project's diagram into the canvas: its newest saved version, or a blank
// diagram when the project has no versions yet. The text goes through runEval -
// the same text -> model -> canvas pipeline typing uses - then history is cleared
// so the first Undo can't revert to the previously shown project. A transport
// failure leaves whatever is currently shown in place.
export async function loadProjectDiagram(projectId: string) {
  const list = await listVersions(projectId);
  if (!list.ok) return;
  let text: string;
  if (list.versions.length) {
    const version = await readVersion(projectId, list.versions[0].version);
    if (!version.ok) return;
    text = version.data;
  } else {
    text = toCue({ nodes: [], edges: [] });
  }
  files.value = [{ name: "data.cue", text }];
  activeFileName.value = "data.cue";
  snapshotSaved();
  await runEval();
  resetHistory();
}

// Persist the current CUE as an immutable version. The backend re-validates, so
// invalid text surfaces the same diagnostics as a live eval rather than saving.
async function save() {
  saveState.value = { status: "saving" };
  // Versions remain single-file: persist the primary data.cue into the current
  // project's store.
  const primary = files.value.find((f) => f.name === primaryFile());
  const result = await saveCue(currentProjectId.value, primary?.text ?? "");
  if (!result.ok) {
    evalError.value = result.error;
    diagnostics.value = result.diagnostics;
    hints.value = [];
    saveState.value = { status: "error" };
    return;
  }
  saveState.value = { status: "saved", version: result.version };
  snapshotSaved();
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
    loadProjectDiagram,
  };
}
