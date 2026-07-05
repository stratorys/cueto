// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// All pointer-driven canvas interactions: palette tools, shape/table/container
// placement and drawing, free-line endpoint editing, container nesting on drag,
// deletion mirrored into the model, and relation <-> line via edge-endpoint
// dragging. The Vue Flow store event handlers are registered here. Module-level
// singleton, shared with the other canvas composables.

import { nextTick, ref } from "vue";
import type { ShapeKind, Tool } from "../model";
import { useDiagram } from "../useDiagram";
import {
  findNode,
  onConnect,
  onEdgesChange,
  onEdgeUpdate,
  onEdgeUpdateEnd,
  onEdgeUpdateStart,
  onNodeDragStop,
  onNodesChange,
  screenToFlowCoordinate,
  updateNode,
  updateNodeData,
} from "./flowStore";
import { activeFileName, newNodeOwner } from "./useEditorFiles";
import { rebuildGraph } from "./useGraphView";
import { syncTextFromModel } from "./useCueSync";

const { diagram, commit, addShape, addTable, addContainer } = useDiagram();

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

// The armed palette tool (a shape to draw, or "connect" mode); null when nothing
// is armed.
export const activeTool = ref<Tool | null>(null);

// True while "connect" mode is armed. Node components read it to force their
// connection handles visible so a handle-to-handle drag is discoverable.
export const connecting = ref(false);

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

export function beginLineDrag(id: string, whichEnd: number) {
  const n = diagram.value.nodes.find((m) => m.id === id);
  if (!n) return;
  const ends = lineEndpoints(n);
  lineDrag = { id, fixed: ends[whichEnd === 0 ? 1 : 0], last: null };
}

export function dragLineTo(clientX: number, clientY: number) {
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

export function endLineDrag(id: string) {
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

export function useDrawTools() {
  return {
    activeTool,
    armTool,
    disarmTool,
    placeShape,
    placeTable,
    placeContainer,
    drawShape,
    connectShapes,
  };
}
