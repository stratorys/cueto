// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// A small promise-based dialog service that replaces the browser's blocking
// window.prompt / window.confirm. A single <AppModal /> mounted at the app root
// renders the shared state; any component calls promptDialog()/confirmDialog() and
// awaits the result. Only one dialog is shown at a time - opening a second resolves
// the first as cancelled.

import { reactive } from "vue";

interface ModalState {
  open: boolean;
  kind: "prompt" | "confirm";
  title: string;
  message: string;
  // Present only for prompts.
  value: string;
  placeholder: string;
  confirmLabel: string;
  cancelLabel: string;
  // Tints the confirm button red for destructive actions.
  danger: boolean;
}

const state = reactive<ModalState>({
  open: false,
  kind: "prompt",
  title: "",
  message: "",
  value: "",
  placeholder: "",
  confirmLabel: "OK",
  cancelLabel: "Cancel",
  danger: false,
});

// Resolver for the currently open dialog. `null` when nothing is open.
let resolve: ((result: string | boolean | null) => void) | null = null;

function settle(result: string | boolean | null) {
  const done = resolve;
  resolve = null;
  state.open = false;
  if (done) done(result);
}

export interface PromptOptions {
  title: string;
  message?: string;
  defaultValue?: string;
  placeholder?: string;
  confirmLabel?: string;
  cancelLabel?: string;
}

// Ask for a line of text. Resolves to the trimmed value, or null if cancelled.
export function promptDialog(options: PromptOptions): Promise<string | null> {
  settle(null);
  state.kind = "prompt";
  state.title = options.title;
  state.message = options.message ?? "";
  state.value = options.defaultValue ?? "";
  state.placeholder = options.placeholder ?? "";
  state.confirmLabel = options.confirmLabel ?? "OK";
  state.cancelLabel = options.cancelLabel ?? "Cancel";
  state.danger = false;
  state.open = true;
  return new Promise((r) => {
    resolve = (result) => r(typeof result === "string" ? result : null);
  });
}

export interface ConfirmOptions {
  title: string;
  message?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
}

// Ask a yes/no question. Resolves to true when confirmed, false otherwise.
export function confirmDialog(options: ConfirmOptions): Promise<boolean> {
  settle(null);
  state.kind = "confirm";
  state.title = options.title;
  state.message = options.message ?? "";
  state.value = "";
  state.placeholder = "";
  state.confirmLabel = options.confirmLabel ?? "OK";
  state.cancelLabel = options.cancelLabel ?? "Cancel";
  state.danger = options.danger ?? false;
  state.open = true;
  return new Promise((r) => {
    resolve = (result) => r(result === true);
  });
}

// Consumed by <AppModal />.
export function useModal() {
  function accept() {
    settle(state.kind === "prompt" ? state.value.trim() : true);
  }
  function cancel() {
    settle(state.kind === "prompt" ? null : false);
  }
  return { state, accept, cancel };
}
