// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Pure derivations over the inference trace, used by the sync state for element lookup
// (the "why is this here" inspector). Kept free of Vue so it is unit-testable without a
// component harness. The legend itself is backend-authoritative (the /eval legend),
// not derived here.

import type { TraceEntry } from "./api";

// indexTrace keys the trace by element id (node id or edge id) for O(1) inspector
// lookup. Element ids are unique, so a later entry never collides with an earlier one.
export function indexTrace(trace: TraceEntry[]): Map<string, TraceEntry> {
  const map = new Map<string, TraceEntry>();
  for (const entry of trace) map.set(entry.element, entry);
  return map;
}
