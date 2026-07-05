// Diagram store: the JSON model is the source of truth.
// Edits go through commit(), which snapshots the previous state for undo.
// Single-canvas POC, so module-level singleton state is fine.

import { computed, ref, toRaw } from "vue";
import type { Diagram } from "./model";
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

// Replace the whole model as one undoable step (used when CUE text re-evaluates).
function replace(next: Diagram): void {
  undoStack.value.push(clone(diagram.value));
  redoStack.value = [];
  diagram.value = clone(next);
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
    replace,
    undo,
    redo,
    canUndo: computed(() => undoStack.value.length > 0),
    canRedo: computed(() => redoStack.value.length > 0),
  };
}
