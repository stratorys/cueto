<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// The single dialog surface for the app, driven by the useModal() service. Mounted
// once at the root; promptDialog()/confirmDialog() calls anywhere toggle it. Enter
// confirms, Escape cancels, and a prompt's input is focused and selected on open.
import { nextTick, onBeforeUnmount, ref, watch } from "vue";
import { useModal } from "../composables/useModal";

const { state, accept, cancel } = useModal();

const input = ref<HTMLInputElement | null>(null);

// Global key handling so Escape/Enter work regardless of what holds focus - a
// confirm dialog has no input to catch them.
function onKeydown(event: KeyboardEvent) {
  if (event.key === "Escape") {
    event.preventDefault();
    cancel();
  } else if (event.key === "Enter" && state.kind === "confirm") {
    event.preventDefault();
    accept();
  }
}

watch(
  () => state.open,
  (open) => {
    if (open) {
      window.addEventListener("keydown", onKeydown);
      if (state.kind === "prompt") {
        void nextTick(() => {
          input.value?.focus();
          input.value?.select();
        });
      }
    } else {
      window.removeEventListener("keydown", onKeydown);
    }
  },
);

onBeforeUnmount(() => window.removeEventListener("keydown", onKeydown));

const button =
  "rounded border px-3 py-1 font-mono text-xs cursor-pointer disabled:cursor-default disabled:opacity-40";
</script>

<template>
  <Teleport to="body">
    <div
      v-if="state.open"
      class="fixed inset-0 z-[100] flex items-center justify-center bg-black/50 p-4"
      @click.self="cancel"
    >
      <div
        role="dialog"
        aria-modal="true"
        class="w-full max-w-sm rounded-lg border border-slate-700 bg-slate-900 p-4 shadow-xl"
      >
        <h2 class="font-mono text-sm text-slate-100">{{ state.title }}</h2>
        <p v-if="state.message" class="mt-1.5 text-xs leading-snug text-slate-400">{{ state.message }}</p>

        <input
          v-if="state.kind === 'prompt'"
          ref="input"
          v-model="state.value"
          :placeholder="state.placeholder"
          class="mt-3 w-full rounded border border-slate-700 bg-slate-950 px-2 py-1.5 font-mono text-xs text-slate-100 outline-none focus:border-amber-500"
          @keydown.enter.prevent="accept"
        />

        <div class="mt-4 flex justify-end gap-2">
          <button :class="[button, 'border-slate-700 text-slate-300 hover:border-slate-500']" @click="cancel">
            {{ state.cancelLabel }}
          </button>
          <button
            :class="[
              button,
              state.danger
                ? 'border-red-500 bg-red-500/15 text-red-300 hover:bg-red-500/25'
                : 'border-amber-500 bg-amber-500/15 text-amber-300 hover:bg-amber-500/25',
            ]"
            @click="accept"
          >
            {{ state.confirmLabel }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
