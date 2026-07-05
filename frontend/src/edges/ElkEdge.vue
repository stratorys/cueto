<script setup lang="ts">
import { computed } from "vue";
import { BaseEdge, getSmoothStepPath } from "@vue-flow/core";
import type { EdgeProps } from "@vue-flow/core";

// Edge that draws the exact orthogonal polyline elkjs computed for it, passed in
// as absolute-coordinate `data.points`. Until a layout runs (or after a manual
// drag clears the stale points) it falls back to Vue Flow's smooth-step path, so
// every edge renders regardless of layout state.
const props = defineProps<EdgeProps<{ points?: { x: number; y: number }[] }>>();

// The edge stroke is set inline (from the mapping), which beats the default
// theme's `.selected` CSS rule - so selection is drawn here instead: an amber,
// thicker stroke that overrides the base style while the edge is selected.
const edgeStyle = computed(() =>
  props.selected
    ? { ...(props.style as object), stroke: "#f59e0b", strokeWidth: 2.5 }
    : props.style,
);

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
  <BaseEdge :id="id" :path="path" :marker-end="markerEnd" :style="edgeStyle" />
</template>
