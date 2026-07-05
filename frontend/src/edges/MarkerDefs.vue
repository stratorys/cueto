<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// Shared SVG marker defs for the edge kinds ElkEdge draws by `url(#id)`:
//  - cueto-arrow: a filled arrowhead ("arrow" kind).
//  - cueto-inherit: a hollow UML generalization triangle ("inherit" kind).
// `context-stroke` makes each marker inherit its edge's stroke, so both track the
// amber selection color. Rendered once by DiagramCanvas as a zero-size SVG; its
// ids are document-global, so Vue Flow's own edge paths can reference them.
</script>

<template>
  <svg class="pointer-events-none absolute h-0 w-0" aria-hidden="true">
    <defs>
      <marker
        id="cueto-arrow"
        viewBox="0 0 10 10"
        refX="9"
        refY="5"
        markerWidth="9"
        markerHeight="9"
        markerUnits="userSpaceOnUse"
        orient="auto-start-reverse"
      >
        <path d="M0,0 L10,5 L0,10 z" fill="context-stroke" />
      </marker>
      <marker
        id="cueto-inherit"
        viewBox="0 0 12 12"
        refX="11"
        refY="6"
        markerWidth="14"
        markerHeight="14"
        markerUnits="userSpaceOnUse"
        orient="auto-start-reverse"
      >
        <path d="M0,0 L12,6 L0,12 z" fill="white" stroke="context-stroke" stroke-width="1.5" />
      </marker>
    </defs>
  </svg>
</template>
