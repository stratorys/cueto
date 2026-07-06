// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// The Vue Flow view built from the model: the controlled node/edge arrays, the
// rebuild that regenerates them, cross-panel highlight patching, drill-down
// focus, auto-layout, and undo/redo. Module-level singleton, shared with the
// other canvas composables.

import { computed, nextTick, ref, watch } from "vue";
import type { Diagram } from "../model";
import type { EdgePoints, NodePositions } from "../mapping";
import { toFlowEdges, toFlowNodes } from "../mapping";
import { layoutDiagram } from "../useLayout";
import { useDiagram } from "../useDiagram";
import { useHighlight } from "./useHighlight";
import { fitView, findNode } from "./flowStore";
import { syncTextFromModel } from "./useCueSync";

const { diagram, commit, undo, redo } = useDiagram();

export const GRID_COLOR = "#e2e8f0";

// Controlled view state: the arrays ARE the view; Vue Flow keeps its store in
// sync both ways.
export const nodes = ref(toFlowNodes(diagram.value));
export const edges = ref(toFlowEdges(diagram.value));

// Absolute-coordinate edge bend points from the last auto-layout. Ephemeral view
// state (never persisted to CUE); cleared whenever a node moves manually, so
// stale routing falls back to a smooth-step path.
export const edgePoints = ref<EdgePoints>({});

// Absolute node positions from the last auto-layout of a coordinate-free
// (data-derived) diagram. Ephemeral view state, the node analog of edgePoints:
// never committed to the model or written to CUE, so the derived file stays
// coordinate-free. Empty for a normal hand-drawn diagram.
export const autoPositions = ref<NodePositions>({});

// A diagram is "auto-layout mode" when any node has no coordinates - it was
// derived from data rather than drawn. Such a diagram is laid out into
// autoPositions and rendered read-only (no drag, no model->text write-back).
export const isAutoLayout = computed(() =>
  diagram.value.nodes.some((n) => n.x === undefined || n.y === undefined),
);

// Cross-panel highlight (blast-radius, diff, query). Purely visual: it patches
// Vue Flow node/edge `class` in place, never the model.
const { highlightedNodeIds, highlightedEdgeIds, mode: highlightMode } = useHighlight();

// Drill-down: id of the container the canvas is focused into (only its subtree is
// shown), or null at the top level.
export const focusedContainer = ref<string | null>(null);

// Path from the top level down to the focused container, for the breadcrumb bar.
// Empty at the top level.
export const breadcrumb = computed<{ id: string; label: string }[]>(() => {
  const byId = new Map(diagram.value.nodes.map((n) => [n.id, n]));
  const trail: { id: string; label: string }[] = [];
  let cur = focusedContainer.value ? byId.get(focusedContainer.value) : undefined;
  while (cur) {
    trail.unshift({ id: cur.id, label: cur.label || cur.id });
    cur = cur.parent ? byId.get(cur.parent) : undefined;
  }
  return trail;
});

// Rebuild the Vue Flow view from the model. Edge bend points are dropped unless
// `keepEdgePoints` is set (only the auto-layout keeps the points it just made);
// any other rebuild follows a manual edit that invalidates the old routing.
export function rebuildGraph(keepEdgePoints = false) {
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
  nodes.value = toFlowNodes(diagram.value, focusedContainer.value, autoPositions.value);
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
  // A data-derived diagram has no model coordinates to commit; re-layout goes
  // through the ephemeral path so the manual button never pollutes the CUE.
  if (isAutoLayout.value) return layoutAuto();
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

// Auto-layout for a coordinate-free (data-derived) diagram. Same elkjs pass as
// layout(), but the geometry goes into the ephemeral autoPositions ref instead of
// the model via commit(), and the text is never regenerated - the CUE stays the
// source of truth and coordinate-free. Called after each eval when isAutoLayout.
export async function layoutAuto() {
  // Lay out only the derived (coordinate-free) nodes and the edges between them;
  // hand-drawn nodes keep their own coordinates and must not be moved.
  const derivedIds = new Set(
    diagram.value.nodes.filter((n) => n.x === undefined || n.y === undefined).map((n) => n.id),
  );
  const subgraph: Diagram = {
    nodes: diagram.value.nodes.filter((n) => derivedIds.has(n.id)),
    edges: diagram.value.edges.filter(
      (e) => derivedIds.has(e.source) && derivedIds.has(e.target),
    ),
  };
  const result = await layoutDiagram(subgraph, (node) => {
    if (node.width && node.height) return { width: node.width, height: node.height };
    const found = findNode(node.id);
    return {
      width: found?.dimensions?.width || 160,
      height: found?.dimensions?.height || 80,
    };
  });
  const positions: NodePositions = {};
  for (const [id, geo] of Object.entries(result.nodes)) {
    positions[id] = { x: Math.round(geo.x), y: Math.round(geo.y) };
  }
  autoPositions.value = positions;
  edgePoints.value = result.edges;
  rebuildGraph(true);
  // Fit with padding so the outer nodes are not flush against (or clipped by) the
  // viewport edge. A second frame lets Vue Flow apply the new node dimensions
  // (taller with a data card) before the fit measures the graph bounds.
  nextTick(() => requestAnimationFrame(() => fitView({ padding: 0.2 })));
}

// Drill into a container (or back to the top level with null), rebuild the view,
// then fit the shown subtree.
export function setFocus(id: string | null) {
  focusedContainer.value = id;
  rebuildGraph();
  nextTick(() => fitView());
}

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

export function useGraphView() {
  return {
    nodes,
    edges,
    gridColor: GRID_COLOR,
    onUndo,
    onRedo,
    focusedContainer,
    breadcrumb,
    setFocus,
    layout,
    isAutoLayout,
  };
}
