// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Canvas orchestration: one place that couples the JSON model, the Vue Flow
// view, and the CUE text. Module-level singleton so App (CodePane) and
// DiagramCanvas share one instance. The pieces live in focused composables
// (useEditorFiles, useCueSync, useGraphView, useSelection, useDrawTools) sharing
// the same singleton state; this module aggregates their public surface.
//
// Sync ordering is deliberate and NOT driven by a watcher:
//   graph edit  -> mutate model -> rebuildGraph() + syncTextFromModel()
//   text typed  -> debounce -> runEval() -> replace(model) -> rebuildGraph()
// A text-originated eval must never clobber what the user is typing, hence the
// explicit calls.

import { useDiagram } from "../useDiagram";
import { useEditorFiles } from "./useEditorFiles";
import { useCueSync } from "./useCueSync";
import { useGraphView } from "./useGraphView";
import { useSelection } from "./useSelection";
import { useDrawTools } from "./useDrawTools";

// Standalone exports the node components import directly (so this module never
// imports the node components, which would cut a cycle).
export { commitNodeResize, commitNodeLabel, commitNodeColor } from "./useSelection";
export { editingEdgeId, startEdgeEdit, cancelEdgeEdit, commitEdgeLabel } from "./useSelection";
export { beginLineDrag, dragLineTo, endLineDrag, connecting, hoveredNodeId } from "./useDrawTools";
export { setFocus } from "./useGraphView";
export type { SaveState } from "./useCueSync";

export function useDiagramCanvas() {
  const { diagram, canUndo, canRedo } = useDiagram();
  return {
    ...useEditorFiles(),
    ...useCueSync(),
    ...useGraphView(),
    ...useSelection(),
    ...useDrawTools(),
    canUndo,
    canRedo,
    diagram,
  };
}
