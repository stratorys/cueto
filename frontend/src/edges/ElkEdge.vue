<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
import { computed } from "vue";
import { BaseEdge, getSmoothStepPath } from "@vue-flow/core";
import type { EdgeProps } from "@vue-flow/core";
import type { DiagramEdge } from "../model";

// Edge that draws the exact orthogonal polyline elkjs computed for it, passed in
// as absolute-coordinate `data.points`. Until a layout runs (or after a manual
// drag clears the stale points) it falls back to Vue Flow's smooth-step path, so
// every edge renders regardless of layout state. `data.kind` picks the visual
// connector: a filled arrowhead, a hollow inheritance triangle, or a dashed line
// (markers are defined once in MarkerDefs, rendered by DiagramCanvas).
const props = defineProps<
  EdgeProps<{ points?: { x: number; y: number }[]; kind?: DiagramEdge["kind"] }>
>();

// Marker follows `kind`; the marker shapes inherit the edge stroke via
// `context-stroke`, so they track the amber selection color too. "relation" and
// "line" carry no marker.
const markerUrl = computed(() => {
  if (props.data?.kind === "arrow") return "url(#cueto-arrow)";
  if (props.data?.kind === "inherit") return "url(#cueto-inherit)";
  return undefined;
});

// The edge stroke is set inline (from the mapping), which beats the default
// theme's `.selected` CSS rule - so selection is drawn here instead: an amber,
// thicker stroke that overrides the base style while the edge is selected. A
// "line" kind renders dashed.
const edgeStyle = computed(() => {
  const dash = props.data?.kind === "line" ? { strokeDasharray: "6 4" } : {};
  return props.selected
    ? { ...(props.style as object), ...dash, stroke: "#f59e0b", strokeWidth: 2.5 }
    : { ...(props.style as object), ...dash };
});

const path = computed(() => {
  const points = props.data?.points;
  if (points && points.length >= 2) {
    return points.map((p, i) => `${i === 0 ? "M" : "L"} ${p.x} ${p.y}`).join(" ");
  }
  const [fallback] = getSmoothStepPath({
    sourceX: props.sourceX,
    sourceY: props.sourceY,
    sourcePosition: props.sourcePosition,
    targetX: props.targetX,
    targetY: props.targetY,
    targetPosition: props.targetPosition,
  });
  return fallback;
});
</script>

<template>
  <BaseEdge :id="id" :path="path" :marker-end="markerUrl" :style="edgeStyle" />
</template>
