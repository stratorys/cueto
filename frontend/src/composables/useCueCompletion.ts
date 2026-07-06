// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Shared CUE completion data for the code editor and the REPL: the static builtin /
// package reference (fetched once) and the live diagram field paths (refreshed when
// the editor files change). Module-level singleton so both surfaces read one set
// and the diagram is evaluated once per edit burst, not once per surface.

import { ref, watch } from "vue";
import { fetchCueMeta, fetchReplKeys, type CueMeta } from "../api";
import { files } from "./useEditorFiles";
import { type ReplCompletionData } from "../replCompletions";

const meta = ref<CueMeta | null>(null);
const keys = ref<string[]>([]);

let keysTimer: ReturnType<typeof setTimeout> | undefined;
let started = false;

// Read lazily by the completion sources so they always see the latest keys/meta.
// keys.value is replaced wholesale on refresh, so a source's identity-keyed cache
// invalidates correctly.
function completionData(): ReplCompletionData {
  return { keys: keys.value, meta: meta.value };
}

// refreshKeys asks the backend for the dotted field paths of every top-level data
// field (people, diagram, ...), computed from the parsed CUE value. A currently
// invalid/incomplete diagram comes back as an error, leaving the last good key set
// in place.
async function refreshKeys() {
  const result = await fetchReplKeys(files.value);
  if (result.ok) keys.value = [...result.keys].sort();
}

// Debounce a key refresh so a burst of keystrokes triggers one eval.
function scheduleKeys() {
  clearTimeout(keysTimer);
  keysTimer = setTimeout(refreshKeys, 600);
}

// start fetches the reference once and begins tracking the diagram keys. Idempotent:
// the first surface to mount starts it; later mounts reuse the shared refs.
async function start() {
  if (started) return;
  started = true;
  watch(files, scheduleKeys, { deep: true });
  const m = await fetchCueMeta();
  if (m.ok) meta.value = { builtins: m.builtins, packages: m.packages };
  await refreshKeys();
}

export function useCueCompletion() {
  return { meta, keys, completionData, refreshKeys, start };
}
