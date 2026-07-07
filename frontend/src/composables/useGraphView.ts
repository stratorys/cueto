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
import type { Diagram, EdgeWaypoint } from "../model";
import type { EdgePoints, EdgeWaypoints, NodePositions } from "../mapping";
import { toFlowEdges, toFlowNodes } from "../mapping";
import { layoutDiagram } from "../useLayout";
import { useDiagram } from "../useDiagram";
import { useHighlight } from "./useHighlight";
import { fitView, findNode as findNodeRaw } from "./flowStore";
import { syncTextFromModel } from "./useCueSync";

const { diagram, commit, undo, redo } = useDiagram();

// Vue Flow's node type is deep enough to trip TS's instantiation limit in this file;
// expose findNode through a shallow signature (only the measured size is ever read),
// so no call site instantiates the recursive type. Same idiom used elsewhere here.
const findNode = findNodeRaw as unknown as (
  id: string,
) => { dimensions?: { width: number; height: number } } | undefined;

export const GRID_COLOR = "#e2e8f0";

// The measured on-screen size of a rendered node, or undefined if it has none yet.
function measuredSize(id: string): { width: number; height: number } | undefined {
  return findNode(id)?.dimensions;
}

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

// Positions a user dragged a derived node to. Ephemeral like autoPositions (never
// committed to the model or written to CUE), but sticky: a re-eval re-runs ELK for the
// rest of the graph while these pinned nodes keep where the user put them, so manual
// readability tweaks survive edits. Pruned to nodes that still exist.
const pinnedPositions = ref<NodePositions>({});

// Cosmetic routing a user dragged onto a derived edge. The edge analog of
// pinnedPositions: ephemeral view state (never written to the coordinate-free CUE),
// sticky across re-evals so readability tweaks survive edits, and pruned to edges
// that still exist. A hand-drawn edge stores its waypoints on the model instead.
const pinnedWaypoints = ref<EdgeWaypoints>({});

// pinEdgeWaypoints records a derived edge's dragged route and re-renders. Mirrors
// pinAutoPosition: view state only, so the route never reaches the file. An empty
// list drops the pin so the edge falls back to its ELK route.
export function pinEdgeWaypoints(id: string, waypoints: EdgeWaypoint[]) {
  const next = { ...pinnedWaypoints.value };
  if (waypoints.length) next[id] = waypoints;
  else delete next[id];
  pinnedWaypoints.value = next;
  rebuildGraph(true);
}

// An ELK edge polyline is anchored to where ELK placed its endpoints. Once a node
// moves outside ELK - a manual drag, or a pin overriding ELK's placement on re-layout -
// the routes of edges touching it end at the wrong spot. Return the route map with
// those dropped so the edge falls back to its live smooth-step path; routes between
// untouched nodes keep their orthogonal routing.
function routesClearedAround(routes: EdgePoints, movedIds: Set<string>): EdgePoints {
  const kept: EdgePoints = { ...routes };
  for (const edge of diagram.value.edges) {
    if (movedIds.has(edge.source) || movedIds.has(edge.target)) delete kept[edge.id];
  }
  return kept;
}

// pinAutoPosition records a derived node's dragged position. It updates the rendered
// autoPositions immediately and remembers the pin so the next layoutAuto preserves it.
// This is how a derived node moves without its coordinates ever reaching the file.
export function pinAutoPosition(id: string, position: { x: number; y: number }) {
  pinnedPositions.value = { ...pinnedPositions.value, [id]: position };
  autoPositions.value = { ...autoPositions.value, [id]: position };
  edgePoints.value = routesClearedAround(edgePoints.value, new Set([id]));
  rebuildGraph(true);
}

// A diagram is "auto-layout mode" when any node has no coordinates - it was
// derived from data rather than drawn. Such a diagram is laid out into
// autoPositions; derived nodes render draggable but their drags stay ephemeral
// (pinned in autoPositions, never written back to CUE).
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
  edges.value = toFlowEdges(
    diagram.value,
    focusedContainer.value,
    edgePoints.value,
    pinnedWaypoints.value,
  );
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
    const size = measuredSize(node.id);
    return {
      width: size?.width || 160,
      height: size?.height || 80,
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
  // Lay out only once the cards have real measured sizes; feeding ELK the fallback
  // size packs the tall table nodes together (see whenMeasured).
  await whenMeasured();
  const result = await layoutDiagram(subgraph, (node) => {
    if (node.width && node.height) return { width: node.width, height: node.height };
    const size = measuredSize(node.id);
    return {
      width: size?.width || 160,
      height: size?.height || 80,
    };
  });
  const positions: NodePositions = {};
  for (const [id, geo] of Object.entries(result.nodes)) {
    positions[id] = { x: Math.round(geo.x), y: Math.round(geo.y) };
  }
  // A node the user dragged keeps its pinned spot; ELK places the rest. Pins for nodes
  // that no longer exist (data changed) are dropped so they cannot resurrect.
  const pins: NodePositions = {};
  for (const id of derivedIds) {
    if (pinnedPositions.value[id]) {
      pins[id] = pinnedPositions.value[id];
      positions[id] = pinnedPositions.value[id];
    }
  }
  pinnedPositions.value = pins;
  autoPositions.value = positions;
  // Drop pinned edge routes whose edge no longer exists (the data changed), the edge
  // analog of pruning node pins, so a removed relation cannot resurrect a stale route.
  const liveEdgeIds = new Set(diagram.value.edges.map((e) => e.id));
  const keptWaypoints: EdgeWaypoints = {};
  for (const [id, route] of Object.entries(pinnedWaypoints.value)) {
    if (liveEdgeIds.has(id)) keptWaypoints[id] = route;
  }
  pinnedWaypoints.value = keptWaypoints;
  // A pinned node was moved back to its pin after ELK routed its edges, so those routes
  // are stale; drop them (kept routes are all between ELK-placed nodes).
  edgePoints.value = routesClearedAround(result.edges, new Set(Object.keys(pins)));
  rebuildGraph(true);
  // Sizes were measured before layout, so the geometry is final: fit on the next frame,
  // once Vue Flow has applied the new positions.
  requestAnimationFrame(() => void fitView({ padding: 0.2 }));
}

// whenMeasured resolves once every rendered node has a real measured size, or after a
// bounded number of frames so a node that never reports one cannot hang the layout. Vue
// Flow measures freshly rendered cards asynchronously on a cold load or refresh, so the
// auto-layout waits before feeding ELK the sizes - otherwise the tall table nodes lay
// out at their fallback height and pack together (the bug that "clicking auto-layout"
// worked around, since by then the cards had measured). A re-eval keeps prior sizes, so
// this resolves on the first frame.
const measureAttemptsMax = 30;
function whenMeasured(): Promise<void> {
  return new Promise((resolve) => {
    const tick = (attempt: number) => {
      if (derivedNodesMeasured() || attempt >= measureAttemptsMax) return resolve();
      requestAnimationFrame(() => tick(attempt + 1));
    };
    tick(0);
  });
}

// derivedNodesMeasured reports whether every rendered node has a non-zero measured size
// in the Vue Flow store, the precondition for laying out and fitting the derived diagram.
function derivedNodesMeasured(): boolean {
  // Iterate through a shallow view of the ids; Vue Flow's Node type is deep enough
  // to trip TS's instantiation limit in this file.
  const rendered = nodes.value as unknown as { id: string }[];
  return rendered.every((n) => {
    const size = measuredSize(n.id);
    return !!size && size.width > 0 && size.height > 0;
  });
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
