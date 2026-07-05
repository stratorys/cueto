// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Diagram store: the JSON model is the source of truth.
// Edits go through commit(), which snapshots the previous state for undo.
// Single-canvas POC, so module-level singleton state is fine.

import { computed, ref, toRaw } from "vue";
import type { Diagram, ShapeKind, TypedNodeType } from "./model";
import { sampleDiagram } from "./model";

// The model is pure JSON, so a JSON round-trip deep-clones it and strips Vue's
// reactive proxy. structuredClone throws (DataCloneError) on a reactive proxy.
function clone(diagram: Diagram): Diagram {
  return JSON.parse(JSON.stringify(toRaw(diagram)));
}

const diagram = ref<Diagram>(clone(sampleDiagram));
const undoStack = ref<Diagram[]>([]);
const redoStack = ref<Diagram[]>([]);

// Apply a mutation as one undoable step. Commit on interaction-end, never mid-drag.
function commit(mutate: (draft: Diagram) => void): void {
  undoStack.value.push(clone(diagram.value));
  redoStack.value = [];
  const next = clone(diagram.value);
  mutate(next);
  diagram.value = next;
}

// A node id doubles as its CUE map key (nodes: [ID=string]: #Node), so it must
// be a legible, valid bare identifier. Slug the label, prefixing when it would
// otherwise start with a digit.
function slugify(label: string): string {
  const slug = label
    .toLowerCase()
    .replace(/[^a-z0-9_]+/g, "_")
    .replace(/^_+|_+$/g, "");
  return /^[a-z_]/.test(slug) ? slug : `n_${slug}`;
}

// Disambiguate a base id against ids already in use: base, base_2, base_3, ...
function uniqueId(base: string, taken: Set<string>): string {
  if (!taken.has(base)) return base;
  let n = 2;
  while (taken.has(`${base}_${n}`)) n++;
  return `${base}_${n}`;
}

// Create a shape at a position with a size, as one undoable step. The id is
// slugged from the geometry (rectangle, rectangle_2, ...). Returns the new id.
function addShape(
  shape: ShapeKind,
  position: { x: number; y: number },
  size: { width: number; height: number },
  flip?: boolean,
): string {
  const taken = new Set(diagram.value.nodes.map((node) => node.id));
  const id = uniqueId(slugify(shape), taken);
  commit((draft) => {
    draft.nodes.push({
      id,
      type: "shape",
      shape,
      x: position.x,
      y: position.y,
      width: size.width,
      height: size.height,
      flip,
      label: "",
    });
  });
  return id;
}

// Create a DB table node at a position, as one undoable step. Size is
// content-derived (no width/height), so TableNode grows with its columns. The
// table starts with one primary-key column; further columns are edited in CUE.
// Returns the new id.
function addTable(position: { x: number; y: number }): string {
  const taken = new Set(diagram.value.nodes.map((node) => node.id));
  const id = uniqueId("table", taken);
  commit((draft) => {
    draft.nodes.push({
      id,
      type: "table",
      x: position.x,
      y: position.y,
      label: id,
      columns: [{ name: "id", dbType: "int", pk: true }],
    });
  });
  return id;
}

// Create a container node at a position, as one undoable step. A container holds
// other nodes (they point at it via `parent`); it starts empty with an explicit
// frame size so it has a visible box. Returns the new id.
function addContainer(
  position: { x: number; y: number },
  size: { width: number; height: number },
): string {
  const taken = new Set(diagram.value.nodes.map((node) => node.id));
  const id = uniqueId("container", taken);
  commit((draft) => {
    draft.nodes.push({
      id,
      type: "container",
      x: position.x,
      y: position.y,
      width: size.width,
      height: size.height,
      label: id,
    });
  });
  return id;
}

// Create a typed domain node (entity/process/decision) at a position with a size,
// as one undoable step. The id is slugged from the type (entity, entity_2, ...).
// Unlike a table it carries no payload - TypedNode draws it from its type alone.
// Returns the new id.
function addTypedNode(
  type: TypedNodeType,
  position: { x: number; y: number },
  size: { width: number; height: number },
): string {
  const taken = new Set(diagram.value.nodes.map((node) => node.id));
  const id = uniqueId(slugify(type), taken);
  commit((draft) => {
    draft.nodes.push({
      id,
      type,
      x: position.x,
      y: position.y,
      width: size.width,
      height: size.height,
      label: "",
    });
  });
  return id;
}

// Replace the whole model as one undoable step (used when CUE text re-evaluates).
function replace(next: Diagram): void {
  undoStack.value.push(clone(diagram.value));
  redoStack.value = [];
  diagram.value = clone(next);
}

// Clear the undo/redo history. Used after the initial persisted-diagram load so the
// first Undo can't revert to the hardcoded sample the store was seeded with.
function resetHistory(): void {
  undoStack.value = [];
  redoStack.value = [];
}

function undo(): void {
  const previous = undoStack.value.pop();
  if (!previous) return;
  redoStack.value.push(clone(diagram.value));
  diagram.value = previous;
}

function redo(): void {
  const next = redoStack.value.pop();
  if (!next) return;
  undoStack.value.push(clone(diagram.value));
  diagram.value = next;
}

export function useDiagram() {
  return {
    diagram,
    commit,
    addShape,
    addTable,
    addContainer,
    addTypedNode,
    replace,
    resetHistory,
    undo,
    redo,
    canUndo: computed(() => undoStack.value.length > 0),
    canRedo: computed(() => redoStack.value.length > 0),
  };
}
