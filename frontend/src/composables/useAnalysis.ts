// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Graph-analysis state for the inspector's Analysis tab. Module-level singleton
// (matches useDiagram / useHighlight): derives SPOF / cycle / orphan results
// from the live model and drives the canvas highlight for blast-radius and
// what-if simulation.

import { computed, ref, watch } from "vue";
import { useDiagram } from "../useDiagram";
import { useHighlight } from "./useHighlight";
import type { Direction } from "../analysis/graph";
import {
  blastRadius,
  findCycles,
  orphans,
  simulateDown,
  singlePointsOfFailure,
} from "../analysis/graph";

const { diagram } = useDiagram();
const { setHighlight, clearHighlight } = useHighlight();

// Which way "impact" flows; see the direction convention in analysis/graph.ts.
const direction = ref<Direction>("dependents");

// What-if: the set of nodes toggled "down".
const downNodes = ref<Set<string>>(new Set());

const spofs = computed(() => singlePointsOfFailure(diagram.value));
const cycles = computed(() => findCycles(diagram.value));
const orphanNodes = computed(() => orphans(diagram.value));

// Everything the currently-downed nodes take with them.
const impacted = computed(() => simulateDown(diagram.value, downNodes.value, direction.value));

function toggleDown(id: string): void {
  const next = new Set(downNodes.value);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  downNodes.value = next;
}

function clearDown(): void {
  downNodes.value = new Set();
}

// Reflect the what-if selection on the canvas: the downed nodes plus their
// impact set stay lit, everything else dims. Empty selection clears.
watch([impacted, downNodes, direction], () => {
  if (downNodes.value.size === 0) {
    clearHighlight();
    return;
  }
  setHighlight([...downNodes.value, ...impacted.value], [], "focus");
});

// One-off: light up a single node's blast radius (row click on a SPOF/orphan).
function focusNode(id: string): void {
  const radius = blastRadius(diagram.value, id, direction.value);
  setHighlight([id, ...radius], [], "focus");
}

// Light up the members of one cycle.
function focusCycle(members: string[]): void {
  setHighlight(members, [], "focus");
}

export function useAnalysis() {
  return {
    diagram,
    direction,
    downNodes,
    spofs,
    cycles,
    orphanNodes,
    impacted,
    toggleDown,
    clearDown,
    focusNode,
    focusCycle,
    clearHighlight,
  };
}
