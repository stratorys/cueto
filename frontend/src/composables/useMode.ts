// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// The backend persistence mode, resolved once at app mount from GET /config. In
// playground mode saves become immutable versions in the content-addressed store;
// in workspace mode saves write real files on disk and git is the history. The rest
// of the UI reads `isWorkspace` to pick its data source and messaging. Module-level
// singleton like the other composables.

import { computed, ref } from "vue";
import type { Mode } from "../api";
import { getConfig } from "../api";

export const mode = ref<Mode>("playground");

export const isWorkspace = computed(() => mode.value === "workspace");

// Resolve the mode from the backend. An unreachable backend keeps the default
// (playground), so the first-visit playground experience never depends on config.
export async function initMode(): Promise<Mode> {
  const result = await getConfig();
  mode.value = result.ok ? result.mode : "playground";
  return mode.value;
}

export function useMode() {
  return { mode, isWorkspace, initMode };
}
