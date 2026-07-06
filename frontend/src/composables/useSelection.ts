// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Canvas selection and the property mutators driven by the inspector: label,
// color, node/edge governance, and resize. Every mutator commits one
// undoable step, rebuilds the view, and flushes the model back to CUE text.
// Module-level singleton, shared with the other canvas composables.

import { computed, ref, watch } from "vue";
import type { DiagramEdge, DiagramNode, NodeType } from "../model";
import { useDiagram } from "../useDiagram";
import { store } from "./flowStore";
import { rebuildGraph } from "./useGraphView";
import { syncTextFromModel } from "./useCueSync";

const { diagram, commit } = useDiagram();

// Id of the node or edge currently selected on the canvas, or null. Drives the
// code pane's block tint (canvas -> code focus). Empty selection -> null.
export const selectedElementId = ref<string | null>(null);

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
export const selectedElement = computed<
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

// Id of the edge whose free-form text label is being edited inline (from a
// double-click on the edge), or null. ElkEdge reads this to show its editor.
export const editingEdgeId = ref<string | null>(null);

// Open / close the inline edge-label editor.
export function startEdgeEdit(id: string) {
  editingEdgeId.value = id;
}
export function cancelEdgeEdit() {
  editingEdgeId.value = null;
}

// Persist an edge's free-form text label after inline (double-click) editing. An
// empty label clears the field so a bare edge emits no `label` key.
export function commitEdgeLabel(id: string, label: string) {
  editingEdgeId.value = null;
  commit((draft) => {
    const target = draft.edges.find((e) => e.id === id);
    if (target) target.label = label || undefined;
  });
  rebuildGraph();
  syncTextFromModel();
}

// Persist a node's label after inline (double-click) editing.
export function commitNodeLabel(id: string, label: string) {
  commit((draft) => {
    const target = draft.nodes.find((n) => n.id === id);
    if (target) target.label = label;
  });
  rebuildGraph();
  syncTextFromModel();
}

// Persist a shape's fill and/or border color from the selection popover. A patch
// value of undefined clears that field (back to the default look).
export function commitNodeColor(
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

// Persist an edge's cardinality from the inspector. A field present in the patch
// is set, or cleared when its value is empty (so data.cue stays minimal and the
// field falls back to its optional-absent default). Mirrors commitNodeColor's
// "key in patch" clear-semantics.
function commitEdgeGovernance(
  id: string,
  patch: Partial<Pick<DiagramEdge, "card">>,
) {
  commit((draft) => {
    const target = draft.edges.find((e) => e.id === id);
    if (!target) return;
    if ("card" in patch) target.card = patch.card || undefined;
  });
  rebuildGraph();
  syncTextFromModel();
}

// Switch a node's visual type from the inspector. Payload fields belong to
// specific types, so drop the ones that no longer apply (keeping data.cue clean):
// a `shape` needs a concrete silhouette to render, `columns` only make sense on a
// table. The inspector only offers this for the plain visual types (it excludes
// table/container), so no children are ever orphaned here.
function commitNodeType(id: string, type: NodeType) {
  commit((draft) => {
    const target = draft.nodes.find((n) => n.id === id);
    if (!target) return;
    target.type = type;
    target.shape = type === "shape" ? (target.shape ?? "rectangle") : undefined;
    if (type !== "table") target.columns = undefined;
  });
  rebuildGraph();
  syncTextFromModel();
}

// Switch an edge's visual kind (relation/arrow/inherit/line) from the inspector.
function commitEdgeKind(id: string, kind: DiagramEdge["kind"]) {
  commit((draft) => {
    const target = draft.edges.find((e) => e.id === id);
    if (target) target.kind = kind;
  });
  rebuildGraph();
  syncTextFromModel();
}

// Reverse an edge's direction: swap its endpoints and their handles in one
// undoable step. Kind, label, and governance metadata are preserved.
function commitEdgeReverse(id: string) {
  commit((draft) => {
    const target = draft.edges.find((e) => e.id === id);
    if (!target) return;
    const source = target.source;
    const sourceHandle = target.sourceHandle;
    target.source = target.target;
    target.target = source;
    target.sourceHandle = target.targetHandle;
    target.targetHandle = sourceHandle;
  });
  rebuildGraph();
  syncTextFromModel();
}

// Persist a node's geometry after a resize handle drag.
export function commitNodeResize(
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

export function useSelection() {
  return {
    selectedElementId,
    selectedElement,
    commitNodeType,
    commitEdgeKind,
    commitEdgeReverse,
    commitEdgeGovernance,
  };
}
