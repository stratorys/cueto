// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Resizable split between a pane and the canvas. Returns the current size (width
// for a side pane, height for a top/bottom pane), a collapsed flag with a toggle,
// and a pointerdown handler for the divider. `side` picks which edge the pane is
// anchored to: a left pane grows with the cursor's x, a right pane as the cursor
// moves toward the left; a bottom pane grows as the cursor moves up, a top pane
// with the cursor's y. When `storageKey` is set, the size and collapsed state are
// view-state preferences persisted to localStorage so they survive a refresh.

import { ref } from "vue";

export function usePaneResize(
  initialWidth = 560,
  side: "left" | "right" | "top" | "bottom" = "left",
  storageKey?: string,
) {
  const horizontal = side === "left" || side === "right";
  const paneWidth = ref(loadWidth(storageKey) ?? initialWidth);
  const collapsed = ref(loadCollapsed(storageKey));

  function onResize(event: PointerEvent) {
    if (horizontal) {
      const raw = side === "left" ? event.clientX : window.innerWidth - event.clientX;
      paneWidth.value = Math.min(Math.max(raw, 220), window.innerWidth - 320);
    } else {
      const raw = side === "top" ? event.clientY : window.innerHeight - event.clientY;
      paneWidth.value = Math.min(Math.max(raw, 120), window.innerHeight - 200);
    }
  }

  function stopResize() {
    window.removeEventListener("pointermove", onResize);
    window.removeEventListener("pointerup", stopResize);
    document.body.style.userSelect = "";
    document.body.style.cursor = "";
    persistWidth(storageKey, paneWidth.value);
  }

  function startResize() {
    // A collapsed pane has no width to drag; the divider only toggles it back open.
    if (collapsed.value) return;
    window.addEventListener("pointermove", onResize);
    window.addEventListener("pointerup", stopResize);
    document.body.style.userSelect = "none";
    document.body.style.cursor = horizontal ? "col-resize" : "row-resize";
  }

  function toggleCollapse() {
    collapsed.value = !collapsed.value;
    persistCollapsed(storageKey, collapsed.value);
  }

  return { paneWidth, collapsed, startResize, toggleCollapse };
}

function loadWidth(storageKey?: string): number | null {
  if (!storageKey) return null;
  try {
    const raw = localStorage.getItem(storageKey);
    if (!raw) return null;
    const value = Number(raw);
    return Number.isFinite(value) ? value : null;
  } catch {
    return null;
  }
}

function persistWidth(storageKey: string | undefined, width: number): void {
  if (!storageKey) return;
  try {
    localStorage.setItem(storageKey, String(Math.round(width)));
  } catch {
    // Storage unavailable or full: pane width is non-critical, so fail silently.
  }
}

function loadCollapsed(storageKey?: string): boolean {
  if (!storageKey) return false;
  try {
    return localStorage.getItem(`${storageKey}.collapsed`) === "1";
  } catch {
    return false;
  }
}

function persistCollapsed(storageKey: string | undefined, collapsed: boolean): void {
  if (!storageKey) return;
  try {
    localStorage.setItem(`${storageKey}.collapsed`, collapsed ? "1" : "0");
  } catch {
    // Storage unavailable or full: collapsed state is non-critical, so fail silently.
  }
}
