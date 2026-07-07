// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Pure derivations over the inference trace, shared by the sync state (element lookup
// for the "why is this here" inspector) and the legend (node kinds). Kept free of Vue
// so they are unit-testable without a component harness.

import type { TraceEntry } from "./api";

// indexTrace keys the trace by element id (node id or edge id) for O(1) inspector
// lookup. Element ids are unique, so a later entry never collides with an earlier one.
export function indexTrace(trace: TraceEntry[]): Map<string, TraceEntry> {
  const map = new Map<string, TraceEntry>();
  for (const entry of trace) map.set(entry.element, entry);
  return map;
}

// legendKinds returns the distinct registry names (node kinds) present in the trace,
// sorted so the legend and its swatch assignment are stable across evals. A declared
// (non-inferred) view has no registry entries and yields an empty legend.
export function legendKinds(trace: TraceEntry[]): string[] {
  const seen = new Set<string>();
  for (const entry of trace) {
    if (entry.kind === "node" && entry.rule === "registry") seen.add(entry.detail);
  }
  return [...seen].sort();
}
