// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Cross-panel highlight: one shared "selection" over the canvas, driven by the
// inspector panels (analysis / history / query). Last write wins; every panel
// clears it on tab-switch / unmount so highlights never linger.
//
// Module-level singleton, mirroring useDiagram.ts. The canvas (useDiagramCanvas)
// watches this state and patches Vue Flow node/edge classes in place - it never
// rebuilds the model to show a highlight.

import { ref } from "vue";

// "focus" dims everything not highlighted; "none" clears all classes.
export type HighlightMode = "none" | "focus";

const highlightedNodeIds = ref<Set<string>>(new Set());
const highlightedEdgeIds = ref<Set<string>>(new Set());
const mode = ref<HighlightMode>("none");

// Replace the current highlight set. Passing an empty node/edge set with mode
// "focus" dims the whole graph (a query with no matches), which is intentional.
function setHighlight(
  nodeIds: Iterable<string>,
  edgeIds: Iterable<string> = [],
  m: HighlightMode = "focus",
): void {
  highlightedNodeIds.value = new Set(nodeIds);
  highlightedEdgeIds.value = new Set(edgeIds);
  mode.value = m;
}

function clearHighlight(): void {
  highlightedNodeIds.value = new Set();
  highlightedEdgeIds.value = new Set();
  mode.value = "none";
}

export function useHighlight() {
  return { highlightedNodeIds, highlightedEdgeIds, mode, setHighlight, clearHighlight };
}
