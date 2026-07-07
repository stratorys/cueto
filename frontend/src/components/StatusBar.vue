<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
import { CircleAlert, Check, Lightbulb, Moon, Sun } from "lucide-vue-next";
import type { SaveState } from "../composables/useDiagramCanvas";
import { useTheme } from "../composables/useTheme";

// The code pane's bottom status bar (VSCode idiom): save state and problem count on
// the left, cursor position, the theme toggle, and the hints toggle on the right.
// Absence of problems needs no announcement, so there is no "valid" badge - a clean
// bar is the signal.
defineProps<{
  saveState: SaveState;
  problemCount: number;
  cursor: { line: number; col: number };
  showHints: boolean;
}>();
defineEmits<{ toggleHints: []; problems: [] }>();

const { theme, toggle: toggleTheme } = useTheme();

const item = "flex items-center gap-1 px-2 h-full";
</script>

<template>
  <div class="flex h-6 flex-none items-center justify-between border-t border-slate-200 bg-white font-mono text-[11px] text-slate-500 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-400">
    <div class="flex h-full items-center">
      <button
        v-if="problemCount"
        :class="item"
        class="text-red-600 hover:bg-slate-100 dark:text-red-400 dark:hover:bg-slate-800"
        title="Jump to the first problem"
        @click="$emit('problems')"
      >
        <CircleAlert class="h-3 w-3" />
        {{ problemCount }} {{ problemCount === 1 ? "problem" : "problems" }}
      </button>
      <span v-else :class="item" class="text-slate-400 dark:text-slate-500">
        <Check class="h-3 w-3" />
        no problems
      </span>

      <span v-if="saveState.status === 'saving'" :class="item">Saving…</span>
      <span
        v-else-if="saveState.status === 'saved'"
        :class="item"
        class="text-emerald-600 dark:text-emerald-400"
        :title="saveState.version"
      >Written to file</span>
      <span v-else-if="saveState.status === 'error'" :class="item" class="text-red-600 dark:text-red-400">Save failed</span>
    </div>

    <div class="flex h-full items-center">
      <button
        :class="item"
        class="hover:bg-slate-100 dark:hover:bg-slate-800"
        :title="theme === 'dark' ? 'Switch to light editor theme' : 'Switch to dark editor theme'"
        @click="toggleTheme"
      >
        <Sun v-if="theme === 'dark'" class="h-3 w-3" />
        <Moon v-else class="h-3 w-3" />
      </button>
      <button
        :class="item"
        class="hover:bg-slate-100 dark:hover:bg-slate-800"
        :title="showHints ? 'Hide type hints' : 'Show type hints'"
        @click="$emit('toggleHints')"
      >
        <Lightbulb class="h-3 w-3" :class="showHints ? 'text-amber-600 dark:text-amber-400' : 'text-slate-400 dark:text-slate-500'" />
        Hints
      </button>
      <span :class="item" class="tabular-nums">Ln {{ cursor.line }}, Col {{ cursor.col }}</span>
    </div>
  </div>
</template>
