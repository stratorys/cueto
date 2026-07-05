// Resizable split between a side pane and the canvas. Returns the current width
// and a pointerdown handler for the divider. `side` picks which edge the pane is
// anchored to: a left pane grows with the cursor's x, a right pane grows as the
// cursor moves toward the left.

import { ref } from "vue";

export function usePaneResize(initialWidth = 560, side: "left" | "right" = "left") {
  const paneWidth = ref(initialWidth);

  function onResize(event: PointerEvent) {
    const raw = side === "left" ? event.clientX : window.innerWidth - event.clientX;
    paneWidth.value = Math.min(Math.max(raw, 220), window.innerWidth - 320);
  }

  function stopResize() {
    window.removeEventListener("pointermove", onResize);
    window.removeEventListener("pointerup", stopResize);
    document.body.style.userSelect = "";
    document.body.style.cursor = "";
  }

  function startResize() {
    window.addEventListener("pointermove", onResize);
    window.addEventListener("pointerup", stopResize);
    document.body.style.userSelect = "none";
    document.body.style.cursor = "col-resize";
  }

  return { paneWidth, startResize };
}
