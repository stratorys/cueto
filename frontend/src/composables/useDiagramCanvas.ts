// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Canvas orchestration: one place that couples the JSON model, the Vue Flow
// view, and the CUE text. Module-level singleton so App (CodePane) and
// DiagramCanvas share one instance.
//
// Sync ordering is deliberate and NOT driven by a watcher:
//   graph edit  -> mutate model -> rebuildGraph() + syncTextFromModel()
//   text typed  -> debounce -> runEval() -> replace(model) -> rebuildGraph()
// A text-originated eval must never clobber what the user is typing, hence the
// explicit calls.

import { computed, nextTick, ref, watch } from "vue";
import { useVueFlow } from "@vue-flow/core";
import type {
  DiagramEdge,
  DiagramNode,
  EditorFile,
  Provenance,
  ShapeKind,
  Tool,
} from "../model";
import type { EdgePoints } from "../mapping";
import { toFlowEdges, toFlowNodes } from "../mapping";
import { edgesBody, nodeBody, toCue } from "../mapping";
import { evalFiles, formatCue, fromEval, rewriteFile, saveCue } from "../api";
import type { Diagnostic, Hint } from "../api";
import { layoutDiagram } from "../useLayout";
import { useDiagram } from "../useDiagram";
import { useHighlight } from "./useHighlight";

const GRID_COLOR = "#e2e8f0";

// Size used when a shape is placed by a click or a palette drag (not drawn out).
const DEFAULT_SIZE: Record<ShapeKind, { width: number; height: number }> = {
  rectangle: { width: 140, height: 72 },
  ellipse: { width: 140, height: 90 },
  diamond: { width: 110, height: 110 },
  line: { width: 160, height: 24 },
  text: { width: 120, height: 32 },
};

// Approximate size of a freshly placed table, used only to center the drop.
const TABLE_DROP_SIZE = { width: 180, height: 90 };

// Default frame size for a container placed by a click or a palette drag.
const CONTAINER_SIZE = { width: 320, height: 220 };

// Below this drawn size (graph units) a draw gesture counts as a click -> default.
const MIN_DRAW = 8;

const {
  diagram,
  commit,
  addShape,
  addTable,
  addContainer,
  replace,
  undo,
  redo,
  canUndo,
  canRedo,
} = useDiagram();

// The editable file set. Each file is `package diagram`; all unify into one
// diagram. The canvas round-trips edits back into the file that owns each node.
const files = ref<EditorFile[]>([{ name: "data.cue", text: toCue(diagram.value) }]);
// Which file the editor is showing, and which receives canvas-created nodes.
const activeFileName = ref("data.cue");
// The active file's text (for the single editor pane).
const activeText = computed(
  () => files.value.find((f) => f.name === activeFileName.value)?.text ?? "",
);

// Element -> owner file, from the last eval. Drives writeback targeting.
const provenance = ref<Provenance>({ nodes: {}, edges: "" });
// Owner file for canvas-created nodes not yet reflected in eval provenance,
// pinned at creation to the then-active file.
const newNodeOwner = new Map<string, string>();
// nodeId -> owner from the last model->file flush, to compute deletions/moves.
let prevOwned = new Map<string, string>();

// The primary file that owns edges and receives fallback ownership. Prefer the
// conventional data.cue, else the first file.
function primaryFile(): string {
  return files.value.some((f) => f.name === "data.cue")
    ? "data.cue"
    : files.value[0]?.name ?? "data.cue";
}

// The single file that owns the (unsplittable) edge list: whatever eval reported,
// else the primary file.
function edgeOwnerFile(): string {
  return provenance.value.edges || primaryFile();
}

// Which file authors a node: its eval-provenance file, else its creation-time
// file, else the primary file.
function ownerOf(node: DiagramNode): string {
  return node.sourceFile ?? newNodeOwner.get(node.id) ?? primaryFile();
}

const evalError = ref<string | null>(null);

// The editor x-ray: positioned diagnostics (on eval failure) and inlay hints (on
// success), from the last text-originated eval. Both are cleared whenever the CUE
// is regenerated from the graph, since they no longer match the new text until the
// next eval.
const diagnostics = ref<Diagnostic[]>([]);
const hints = ref<Hint[]>([]);

// Whether the editor draws inlay hints (types / optional fields). Diagnostics are
// never gated. On by default; the toggle lets a user quiet a dense x-ray.
const showHints = ref(true);
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
const saveState = ref<SaveState>({ status: "idle" });

// Controlled view state: the arrays ARE the view; Vue Flow keeps its store in
// sync both ways.
const nodes = ref(toFlowNodes(diagram.value));
const edges = ref(toFlowEdges(diagram.value));

// Absolute-coordinate edge bend points from the last auto-layout. Ephemeral view
// state (never persisted to CUE); cleared whenever a node moves manually, so
// stale routing falls back to a smooth-step path.
const edgePoints = ref<EdgePoints>({});

// Cross-panel highlight (blast-radius, diff, query). Purely visual: it patches
// Vue Flow node/edge `class` in place, never the model.
const { highlightedNodeIds, highlightedEdgeIds, mode: highlightMode } = useHighlight();

// The armed palette tool (a shape to draw, or "connect" mode); null when nothing
// is armed.
const activeTool = ref<Tool | null>(null);

// True while "connect" mode is armed. Node components read it to force their
// connection handles visible so a handle-to-handle drag is discoverable.
const connecting = ref(false);

// Id of the node or edge currently selected on the canvas, or null. Drives the
// code pane's block tint (canvas -> code focus). Empty selection -> null.
const selectedElementId = ref<string | null>(null);

// Drill-down: id of the container the canvas is focused into (only its subtree is
// shown), or null at the top level.
const focusedContainer = ref<string | null>(null);

// Path from the top level down to the focused container, for the breadcrumb bar.
// Empty at the top level.
const breadcrumb = computed<{ id: string; label: string }[]>(() => {
  const byId = new Map(diagram.value.nodes.map((n) => [n.id, n]));
  const trail: { id: string; label: string }[] = [];
  let cur = focusedContainer.value ? byId.get(focusedContainer.value) : undefined;
  while (cur) {
    trail.unshift({ id: cur.id, label: cur.label || cur.id });
    cur = cur.parent ? byId.get(cur.parent) : undefined;
  }
  return trail;
});

// Explicit id so this composable and <VueFlow id="diagram"> share ONE store.
// screenToFlowCoordinate maps client (screen) coords to graph coords, accounting
// for BOTH the pane offset (the CUE pane shifts the canvas right) and pan/zoom.
const store = useVueFlow("diagram");
const {
  onNodeDragStop,
  onNodesChange,
  onEdgesChange,
  onConnect,
  onEdgeUpdateStart,
  onEdgeUpdate,
  onEdgeUpdateEnd,
  screenToFlowCoordinate,
  updateNode,
  updateNodeData,
  fitView,
  findNode,
} = store;

// Track the selected node or edge so the code pane can tint its block. Both
// getters are reactive, so a click, a box-select, and a deselect all flow through
// here; a node wins if both are somehow selected. Empty selection -> null.
watch(
  [() => store.getSelectedNodes.value, () => store.getSelectedEdges.value],
  ([selNodes, selEdges]) => {
    selectedElementId.value = selNodes[0]?.id ?? selEdges[0]?.id ?? null;
  },
);

// The selected node or edge resolved against the model, for the inspector's
// property editor. null when nothing (or a since-removed element) is selected.
const selectedElement = computed<
  | { kind: "node"; node: DiagramNode }
  | { kind: "edge"; edge: DiagramEdge }
  | null
>(() => {
  const id = selectedElementId.value;
  if (!id) return null;
  const node = diagram.value.nodes.find((n) => n.id === id);
  if (node) return { kind: "node", node };
  const edge = diagram.value.edges.find((e) => e.id === id);
  return edge ? { kind: "edge", edge } : null;
});

// A graph edit invalidates the CUE text. The canvas is already updated from the
// model (instant); the file text follows via a debounced, per-file /rewrite so a
// burst of edits (a drag) collapses into one round-trip. Kept synchronous in name
// and effect for its ~15 call sites; only the text regeneration is deferred.
let syncTimer: ReturnType<typeof setTimeout> | undefined;
function syncTextFromModel() {
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

// Rebuild the Vue Flow view from the model. Edge bend points are dropped unless
// `keepEdgePoints` is set (only the auto-layout keeps the points it just made);
// any other rebuild follows a manual edit that invalidates the old routing.
function rebuildGraph(keepEdgePoints = false) {
  // Self-heal a stale focus: if a CUE edit removed the focused container, drop
  // back to the top level so the canvas never renders an empty view.
  if (
    focusedContainer.value &&
    !diagram.value.nodes.some(
      (n) => n.id === focusedContainer.value && n.type === "container",
    )
  ) {
    focusedContainer.value = null;
  }
  if (!keepEdgePoints) edgePoints.value = {};
  nodes.value = toFlowNodes(diagram.value, focusedContainer.value);
  edges.value = toFlowEdges(diagram.value, focusedContainer.value, edgePoints.value);
  applyHighlightClasses();
}

// The Vue Flow class for one element under the current highlight: the highlighted
// set gets an accent outline/stroke; in "focus" mode everything else dims;
// undefined clears any prior class.
function highlightClass(id: string, kind: "node" | "edge"): string | undefined {
  if (highlightMode.value === "none") return undefined;
  const set = kind === "node" ? highlightedNodeIds.value : highlightedEdgeIds.value;
  if (set.has(id)) return "is-highlighted";
  return "is-dimmed";
}

// Patch node/edge classes in place so a highlight never triggers a model rebuild
// (which would disturb Vue Flow's drag/selection state).
function applyHighlightClasses() {
  for (const node of nodes.value) node.class = highlightClass(node.id, "node");
  for (const edge of edges.value) edge.class = highlightClass(edge.id, "edge");
}

watch(
  [highlightedNodeIds, highlightedEdgeIds, highlightMode],
  applyHighlightClasses,
);

// Auto-layout the whole diagram with elkjs: hierarchy-aware node placement plus
// orthogonal edge routing. Node geometry is written back as one undoable step and
// the routing is kept for the custom edge to draw.
async function layout() {
  const result = await layoutDiagram(diagram.value, (node) => {
    if (node.width && node.height) return { width: node.width, height: node.height };
    const found = findNode(node.id);
    return {
      width: found?.dimensions?.width || 160,
      height: found?.dimensions?.height || 80,
    };
  });
  commit((draft) => {
    for (const node of draft.nodes) {
      const geo = result.nodes[node.id];
      if (!geo) continue;
      node.x = Math.round(geo.x);
      node.y = Math.round(geo.y);
      // Only overwrite an explicit size; leave auto-sized nodes to their content.
      if (node.width !== undefined && node.height !== undefined) {
        node.width = Math.round(geo.width);
        node.height = Math.round(geo.height);
      }
    }
  });
  edgePoints.value = result.edges;
  rebuildGraph(true);
  syncTextFromModel();
  nextTick(() => fitView());
}

// Drill into a container (or back to the top level with null), rebuild the view,
// then fit the shown subtree.
function setFocus(id: string | null) {
  focusedContainer.value = id;
  rebuildGraph();
  nextTick(() => fitView());
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

async function runEval() {
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

// Place a default-sized shape centered on a client point (drop / click).
function placeShape(shape: ShapeKind, clientX: number, clientY: number) {
  const size = DEFAULT_SIZE[shape];
  const point = screenToFlowCoordinate({ x: clientX, y: clientY });
  const id = addShape(shape, { x: point.x - size.width / 2, y: point.y - size.height / 2 }, size);
  newNodeOwner.set(id, activeFileName.value);
  rebuildGraph();
  syncTextFromModel();
}

// Draw a shape from a press-drag-release: the two client corners define its size
// AND its position (it lands exactly on the drawn box). A gesture too small to be
// a drag creates nothing - a bare click does not drop a shape.
function drawShape(
  shape: ShapeKind,
  x0: number,
  y0: number,
  x1: number,
  y1: number,
) {
  const a = screenToFlowCoordinate({ x: x0, y: y0 });
  const b = screenToFlowCoordinate({ x: x1, y: y1 });
  const width = Math.abs(b.x - a.x);
  const height = Math.abs(b.y - a.y);
  if (width < MIN_DRAW || height < MIN_DRAW) return;
  const position = { x: Math.min(a.x, b.x), y: Math.min(a.y, b.y) };
  const size = { width: Math.round(width), height: Math.round(height) };
  // A line records its drag direction: same-sign dx/dy -> "\" (flip), else "/".
  const id =
    shape === "line"
      ? addShape(shape, position, size, (x1 - x0) * (y1 - y0) > 0)
      : addShape(shape, position, size);
  newNodeOwner.set(id, activeFileName.value);
  rebuildGraph();
  syncTextFromModel();
}

// Place a DB table centered on a client point (drop / click).
function placeTable(clientX: number, clientY: number) {
  const point = screenToFlowCoordinate({ x: clientX, y: clientY });
  const id = addTable({
    x: point.x - TABLE_DROP_SIZE.width / 2,
    y: point.y - TABLE_DROP_SIZE.height / 2,
  });
  newNodeOwner.set(id, activeFileName.value);
  rebuildGraph();
  syncTextFromModel();
}

// Place a container centered on a client point (drop / click).
function placeContainer(clientX: number, clientY: number) {
  const point = screenToFlowCoordinate({ x: clientX, y: clientY });
  const id = addContainer(
    {
      x: point.x - CONTAINER_SIZE.width / 2,
      y: point.y - CONTAINER_SIZE.height / 2,
    },
    CONTAINER_SIZE,
  );
  newNodeOwner.set(id, activeFileName.value);
  rebuildGraph();
  syncTextFromModel();
}

// Persist a node's label after inline (double-click) editing.
function commitNodeLabel(id: string, label: string) {
  commit((draft) => {
    const target = draft.nodes.find((n) => n.id === id);
    if (target) target.label = label;
  });
  rebuildGraph();
  syncTextFromModel();
}

// Persist a shape's fill and/or border color from the selection popover. A patch
// value of undefined clears that field (back to the default look).
function commitNodeColor(
  id: string,
  patch: { fill?: string; stroke?: string },
) {
  commit((draft) => {
    const target = draft.nodes.find((n) => n.id === id);
    if (!target) return;
    if ("fill" in patch) target.fill = patch.fill;
    if ("stroke" in patch) target.stroke = patch.stroke;
  });
  rebuildGraph();
  syncTextFromModel();
}

// Persist a node's governance metadata from the inspector. A field present in
// the patch is set, or cleared when its value is empty (so data.cue stays minimal
// and the field falls back to its optional-absent default). Mirrors
// commitNodeColor's "key in patch" clear-semantics.
function commitNodeGovernance(
  id: string,
  patch: Partial<Pick<DiagramNode, "role" | "owner" | "region" | "zone">>,
) {
  commit((draft) => {
    const target = draft.nodes.find((n) => n.id === id);
    if (!target) return;
    if ("role" in patch) target.role = patch.role || undefined;
    if ("owner" in patch) target.owner = patch.owner || undefined;
    if ("region" in patch) target.region = patch.region || undefined;
    if ("zone" in patch) target.zone = patch.zone || undefined;
  });
  rebuildGraph();
  syncTextFromModel();
}

// Persist an edge's governance metadata from the inspector. Same clear-on-empty
// semantics as commitNodeGovernance; a false `sync` clears the field.
function commitEdgeGovernance(
  id: string,
  patch: Partial<Pick<DiagramEdge, "card" | "call" | "protocol" | "sync">>,
) {
  commit((draft) => {
    const target = draft.edges.find((e) => e.id === id);
    if (!target) return;
    if ("card" in patch) target.card = patch.card || undefined;
    if ("call" in patch) target.call = patch.call || undefined;
    if ("protocol" in patch) target.protocol = patch.protocol || undefined;
    if ("sync" in patch) target.sync = patch.sync || undefined;
  });
  rebuildGraph();
  syncTextFromModel();
}

// Set the governance packs the diagram opts into. An empty list clears the field
// so a bare diagram emits no `policies` key (emit() drops undefined).
function setPolicies(policies: string[]) {
  commit((draft) => {
    draft.policies = policies.length ? policies : undefined;
  });
  rebuildGraph();
  syncTextFromModel();
}

// Persist a node's geometry after a resize handle drag.
function commitNodeResize(
  id: string,
  params: { x: number; y: number; width: number; height: number },
) {
  commit((draft) => {
    const target = draft.nodes.find((n) => n.id === id);
    if (target) {
      target.x = params.x;
      target.y = params.y;
      target.width = Math.round(params.width);
      target.height = Math.round(params.height);
    }
  });
  rebuildGraph();
  syncTextFromModel();
}

// --- free-line endpoints, shape hit-testing, endpoint dragging ----------------

// The two endpoints of a line node from its box + flip. "\" (flip): top-left ->
// bottom-right; "/" : bottom-left -> top-right.
function lineEndpoints(n: {
  x: number;
  y: number;
  width?: number;
  height?: number;
  flip?: boolean;
}): [{ x: number; y: number }, { x: number; y: number }] {
  const w = n.width ?? 1;
  const h = n.height ?? 1;
  return n.flip
    ? [{ x: n.x, y: n.y }, { x: n.x + w, y: n.y + h }]
    : [{ x: n.x, y: n.y + h }, { x: n.x + w, y: n.y }];
}

// Live endpoint drag for a line. beginLineDrag pins the other endpoint; dragLineTo
// updates the view through Vue Flow's typed API and records the geometry in
// `last`; endLineDrag commits `last` once - converting the line to a relation if
// both ends landed on shapes.
let lineDrag:
  | {
      id: string;
      fixed: { x: number; y: number };
      last: { x: number; y: number; width: number; height: number; flip: boolean } | null;
    }
  | null = null;

function beginLineDrag(id: string, whichEnd: number) {
  const n = diagram.value.nodes.find((m) => m.id === id);
  if (!n) return;
  const ends = lineEndpoints(n);
  lineDrag = { id, fixed: ends[whichEnd === 0 ? 1 : 0], last: null };
}

function dragLineTo(clientX: number, clientY: number) {
  if (!lineDrag) return;
  const p = screenToFlowCoordinate({ x: clientX, y: clientY });
  const f = lineDrag.fixed;
  const x = Math.min(f.x, p.x);
  const y = Math.min(f.y, p.y);
  const width = Math.max(1, Math.abs(p.x - f.x));
  const height = Math.max(1, Math.abs(p.y - f.y));
  const flip = (p.x - f.x) * (p.y - f.y) > 0;
  lineDrag.last = { x, y, width, height, flip };
  // Live view via Vue Flow's typed API - keeps the geometry off the deep Node type.
  updateNode(lineDrag.id, {
    position: { x, y },
    style: { width: `${width}px`, height: `${height}px` },
  });
  updateNodeData(lineDrag.id, { flip });
}

function endLineDrag(id: string) {
  const drag = lineDrag;
  lineDrag = null;
  if (!drag || !drag.last) return;
  const x = drag.last.x;
  const y = drag.last.y;
  const width = Math.round(drag.last.width);
  const height = Math.round(drag.last.height);
  const flip = drag.last.flip;
  // Endpoint dragging is pure reshaping - a line stays a decorative line node.
  // Relations are made explicitly via the shape handles (hover / connect mode).
  commit((draft) => {
    const node = draft.nodes.find((m) => m.id === id);
    if (node) {
      node.x = x;
      node.y = y;
      node.width = width;
      node.height = height;
      node.flip = flip;
    }
  });
  rebuildGraph();
  syncTextFromModel();
}

// --- nesting: absolute positions and container hit-testing --------------------

// Absolute (canvas) top-left of a model node, summing x/y up the parent chain.
// A child's stored x/y are relative to its parent (Vue Flow's convention).
function absolutePosition(id: string): { x: number; y: number } {
  const byId = new Map(diagram.value.nodes.map((n) => [n.id, n]));
  let x = 0;
  let y = 0;
  let cur = byId.get(id);
  while (cur) {
    x += cur.x;
    y += cur.y;
    cur = cur.parent ? byId.get(cur.parent) : undefined;
  }
  return { x, y };
}

// True when `candidate` is `ancestorId` or nested anywhere below it. Used to stop
// a node from being dropped into its own descendant (which would cut a cycle).
function isSelfOrDescendant(candidate: string, ancestorId: string): boolean {
  const byId = new Map(diagram.value.nodes.map((n) => [n.id, n]));
  let cur = byId.get(candidate);
  while (cur) {
    if (cur.id === ancestorId) return true;
    cur = cur.parent ? byId.get(cur.parent) : undefined;
  }
  return false;
}

// Innermost container whose absolute box contains `point`, excluding `dragId`
// and anything nested inside it. Innermost = smallest area, so nesting a node in
// a container that itself sits in another lands it in the inner one.
function containerAt(
  point: { x: number; y: number },
  dragId: string,
): string | null {
  let best: string | null = null;
  let bestArea = Infinity;
  for (const n of diagram.value.nodes) {
    if (n.type !== "container") continue;
    if (isSelfOrDescendant(n.id, dragId)) continue;
    const w = n.width ?? 0;
    const h = n.height ?? 0;
    const at = absolutePosition(n.id);
    if (
      point.x >= at.x &&
      point.x <= at.x + w &&
      point.y >= at.y &&
      point.y <= at.y + h
    ) {
      const area = w * h;
      if (area < bestArea) {
        best = n.id;
        bestArea = area;
      }
    }
  }
  return best;
}

function armTool(tool: Tool) {
  activeTool.value = activeTool.value === tool ? null : tool;
  // Reveal the connection handles for connect mode AND the line tool: the line
  // tool draws a connector by dragging between two visible handles (empty-space
  // drags still make a decorative line), so its handles must be discoverable.
  connecting.value = activeTool.value === "connect" || activeTool.value === "line";
}
function disarmTool() {
  activeTool.value = null;
  connecting.value = false;
}

// Drag: commit the final position once, not on every move. On drop, re-parent the
// node into the container it landed in (or out of its parent when dropped clear).
// Vue Flow reports node.position relative to the current parent, so positions are
// converted through absolute space when the parent changes.
onNodeDragStop(({ node }) => {
  const current = diagram.value.nodes.find((n) => n.id === node.id);
  if (!current) return;
  // Absolute top-left at the drop: parent's absolute position + reported (relative)
  // position. A top-level node's reported position is already absolute.
  const parentAbs = current.parent
    ? absolutePosition(current.parent)
    : { x: 0, y: 0 };
  const droppedAbs = {
    x: parentAbs.x + node.position.x,
    y: parentAbs.y + node.position.y,
  };
  const w = current.width ?? node.dimensions?.width ?? 0;
  const h = current.height ?? node.dimensions?.height ?? 0;
  const center = { x: droppedAbs.x + w / 2, y: droppedAbs.y + h / 2 };
  const newParent = containerAt(center, node.id) ?? undefined;

  commit((draft) => {
    const target = draft.nodes.find((n) => n.id === node.id);
    if (!target) return;
    if (newParent === current.parent) {
      // Same parent: node.position is already in the right frame.
      target.x = node.position.x;
      target.y = node.position.y;
      return;
    }
    // Re-parent: store the drop in the new parent's frame (relative), or absolute
    // when dropped to the top level.
    const newParentAbs = newParent ? absolutePosition(newParent) : { x: 0, y: 0 };
    target.parent = newParent;
    target.x = droppedAbs.x - newParentAbs.x;
    target.y = droppedAbs.y - newParentAbs.y;
  });
  // Always rebuild: re-parenting changes the node's parent link, and any move
  // invalidates the previous auto-layout's edge routing.
  rebuildGraph();
  syncTextFromModel();
});

// Connect: push a relation into the model, then rebuild so the edge picks up its
// routing/stroke from the mapping.
onConnect((params) => {
  connectShapes(
    params.source,
    params.sourceHandle ?? undefined,
    params.target,
    params.targetHandle ?? undefined,
  );
});

// Delete: Vue Flow removes elements from the VIEW (v-model) on its own; these two
// hooks mirror that removal into the MODEL so the CUE round-trip drops them too.
// Without this, syncFilesFromModel still sees the node as owned and the next
// rebuildGraph resurrects it from the model. The mutation is deferred so we never
// touch the store while Vue Flow is still inside applyChanges; rebuildGraph then
// assigns nodes/edges directly (no change events), so there is no feedback loop.
onNodesChange((changes) => {
  const removed = changes.filter((c) => c.type === "remove").map((c) => c.id);
  if (!removed.length) return;
  void nextTick(() => {
    // Cascade: deleting a container also deletes every node nested under it.
    const kill = new Set(removed);
    let grew = true;
    while (grew) {
      grew = false;
      for (const node of diagram.value.nodes) {
        if (node.parent && kill.has(node.parent) && !kill.has(node.id)) {
          kill.add(node.id);
          grew = true;
        }
      }
    }
    for (const id of kill) newNodeOwner.delete(id);
    commit((draft) => {
      draft.nodes = draft.nodes.filter((n) => !kill.has(n.id));
      // Drop any edge whose endpoint was removed - a dangling edge fails CUE eval.
      draft.edges = draft.edges.filter(
        (e) => !kill.has(e.source) && !kill.has(e.target),
      );
    });
    rebuildGraph();
    syncTextFromModel();
  });
});

onEdgesChange((changes) => {
  const removed = new Set(
    changes.filter((c) => c.type === "remove").map((c) => c.id),
  );
  if (!removed.size) return;
  void nextTick(() => {
    // Edges torn out as a side effect of a node deletion are already gone from the
    // model; bail if nothing here still exists.
    if (!diagram.value.edges.some((e) => removed.has(e.id))) return;
    commit((draft) => {
      draft.edges = draft.edges.filter((e) => !removed.has(e.id));
    });
    rebuildGraph();
    syncTextFromModel();
  });
});

// --- Phase 3: relation <-> line via edge-endpoint dragging --------------------

// Which end of an edge is being dragged (read from the grabbed updater anchor),
// and whether it reconnected to a valid handle before release.
let edgeDrag: { id: string; end: "source" | "target"; reconnected: boolean } | null = null;

// Midpoint of a shape's t/r/b/l side in absolute coords - where a converted line
// starts. Falls back to measured dimensions for auto-sized nodes (e.g. tables).
function handleAnchor(nodeId: string, handle?: string): { x: number; y: number } {
  const n = diagram.value.nodes.find((m) => m.id === nodeId);
  if (!n) return { x: 0, y: 0 };
  const found = findNode(nodeId);
  const w = n.width ?? found?.dimensions?.width ?? 0;
  const h = n.height ?? found?.dimensions?.height ?? 0;
  const at = absolutePosition(n.id);
  const cx = at.x + w / 2;
  const cy = at.y + h / 2;
  if (handle === "t") return { x: cx, y: at.y };
  if (handle === "b") return { x: cx, y: at.y + h };
  if (handle === "l") return { x: at.x, y: cy };
  if (handle === "r") return { x: at.x + w, y: cy };
  return { x: cx, y: cy };
}

// A node id not yet taken (line, line_2, ...).
function freshId(base: string): string {
  const taken = new Set(diagram.value.nodes.map((n) => n.id));
  if (!taken.has(base)) return base;
  let k = 2;
  while (taken.has(`${base}_${k}`)) k++;
  return `${base}_${k}`;
}

// Push a relation edge between two nodes as one undoable step, then rebuild so the
// mapping gives it its routing/stroke. Used by connect mode (handle -> handle).
function connectShapes(
  source: string,
  sourceHandle: string | undefined,
  target: string,
  targetHandle: string | undefined,
) {
  commit((draft) => {
    draft.edges.push({
      id: crypto.randomUUID(),
      source,
      target,
      sourceHandle,
      targetHandle,
      kind: "relation",
    });
  });
  rebuildGraph();
  syncTextFromModel();
}

onEdgeUpdateStart(({ event, edge }) => {
  const el = event.target instanceof Element ? event.target : null;
  const end = el?.classList.contains("vue-flow__edgeupdater-source") ? "source" : "target";
  edgeDrag = { id: edge.id, end, reconnected: false };
});

// Reconnected to another handle/node: repoint the model edge, keep its other fields.
onEdgeUpdate(({ edge, connection }) => {
  if (edgeDrag) edgeDrag.reconnected = true;
  commit((draft) => {
    const e = draft.edges.find((m) => m.id === edge.id);
    if (!e) return;
    e.source = connection.source;
    e.target = connection.target;
    if (connection.sourceHandle) e.sourceHandle = connection.sourceHandle;
    else delete e.sourceHandle;
    if (connection.targetHandle) e.targetHandle = connection.targetHandle;
    else delete e.targetHandle;
  });
  rebuildGraph();
  syncTextFromModel();
});

// Dropped in empty space (no reconnect): turn the relation back into a floating
// line from the still-attached end to the drop point.
onEdgeUpdateEnd(({ event, edge }) => {
  const drag = edgeDrag;
  edgeDrag = null;
  if (!drag || drag.reconnected) return;
  const e = diagram.value.edges.find((m) => m.id === edge.id);
  if (!e) return;
  const keptNode = drag.end === "source" ? e.target : e.source;
  const keptHandle = drag.end === "source" ? e.targetHandle : e.sourceHandle;
  const anchor = handleAnchor(keptNode, keptHandle);
  const client = "clientX" in event ? { x: event.clientX, y: event.clientY } : { x: 0, y: 0 };
  const drop = screenToFlowCoordinate(client);
  const x = Math.min(anchor.x, drop.x);
  const y = Math.min(anchor.y, drop.y);
  const width = Math.max(1, Math.round(Math.abs(drop.x - anchor.x)));
  const height = Math.max(1, Math.round(Math.abs(drop.y - anchor.y)));
  const flip = (drop.x - anchor.x) * (drop.y - anchor.y) > 0;
  const id = freshId("line");
  newNodeOwner.set(id, activeFileName.value);
  commit((draft) => {
    draft.edges = draft.edges.filter((m) => m.id !== edge.id);
    draft.nodes.push({
      id,
      type: "shape",
      shape: "line",
      x,
      y,
      width,
      height,
      flip,
      label: "",
    });
  });
  rebuildGraph();
  syncTextFromModel();
});

function onUndo() {
  undo();
  rebuildGraph();
  syncTextFromModel();
}

function onRedo() {
  redo();
  rebuildGraph();
  syncTextFromModel();
}

// Called from ShapeNode on resize end. Standalone (not on the returned object) so
// ShapeNode can import it without this module importing ShapeNode (avoids a cycle).
export {
  commitNodeResize,
  commitNodeLabel,
  commitNodeColor,
  beginLineDrag,
  dragLineTo,
  endLineDrag,
  setFocus,
  connecting,
};

export function useDiagramCanvas() {
  return {
    nodes,
    edges,
    gridColor: GRID_COLOR,
    files,
    activeFileName,
    activeText,
    setActiveFile,
    addFile,
    renameFile,
    closeFile,
    evalError,
    diagnostics,
    hints,
    showHints,
    toggleHints,
    onCueEdit,
    save,
    format,
    saveState,
    activeTool,
    armTool,
    disarmTool,
    placeShape,
    placeTable,
    placeContainer,
    drawShape,
    connectShapes,
    onUndo,
    onRedo,
    canUndo,
    canRedo,
    focusedContainer,
    breadcrumb,
    setFocus,
    layout,
    selectedElementId,
    selectedElement,
    diagram,
    commitNodeGovernance,
    commitEdgeGovernance,
    setPolicies,
  };
}
