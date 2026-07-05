// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Typed narrowings for DOM event targets. `event.target` is `EventTarget | null`,
// so reading `.value` / `.checked` / `.files` needs a narrowing. These guard once,
// in one place, so components read from events without scattered casts.

// The <input> element behind an event, or null when the target is something else.
export function inputEl(event: Event): HTMLInputElement | null {
  return event.target instanceof HTMLInputElement ? event.target : null;
}

// Trimmed <input>/<select> value, or undefined when blank (clears a field).
export function fieldValue(event: Event): string | undefined {
  const target = event.target;
  const value =
    target instanceof HTMLInputElement || target instanceof HTMLSelectElement
      ? target.value
      : "";
  return value || undefined;
}

// Checkbox state from a change event.
export function isChecked(event: Event): boolean {
  return event.target instanceof HTMLInputElement && event.target.checked;
}

// The Element behind an event (for .closest()/.tagName/.setPointerCapture), or
// null when the target is not an Element.
export function elementTarget(event: Event): Element | null {
  return event.target instanceof Element ? event.target : null;
}
