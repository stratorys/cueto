// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Editor-pane theme (light/dark) as a view-state preference, following the
// localStorage convention used by the pane-resize and file-tree state. The choice
// is a `.dark` class on <html> that Tailwind's `dark:` variant keys off (see
// style.css). Only the CUE editor pane carries `dark:` variants, so the class flips
// that pane alone; the rest of the app is always light. Dark is the default, which
// preserves the pane's original look until the user opts into light.
import { ref, watch } from "vue";

export type Theme = "light" | "dark";

const STORAGE_KEY = "cueto.theme";

// A saved choice wins; otherwise default to dark (the pane's original appearance).
function initialTheme(): Theme {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved === "light" || saved === "dark") return saved;
  } catch {
    // Storage unavailable: fall through to the default.
  }
  return "dark";
}

function apply(theme: Theme) {
  document.documentElement.classList.toggle("dark", theme === "dark");
}

// Module-level singleton: applied at first import (before the app mounts) so the
// class is set before the first paint, avoiding a light flash.
const theme = ref<Theme>(initialTheme());
apply(theme.value);
watch(theme, (value) => {
  apply(value);
  try {
    localStorage.setItem(STORAGE_KEY, value);
  } catch {
    // Storage unavailable or full: theme is non-critical, fail silently.
  }
});

export function useTheme() {
  function toggle() {
    theme.value = theme.value === "dark" ? "light" : "dark";
  }
  return { theme, toggle };
}
