// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// The one Vue Flow store the canvas composables share. Explicit id so this and
// <VueFlow id="diagram"> bind to the SAME store. screenToFlowCoordinate maps
// client (screen) coords to graph coords, accounting for BOTH the pane offset
// (the CUE pane shifts the canvas right) and pan/zoom.

import { useVueFlow } from "@vue-flow/core";

export const store = useVueFlow("diagram");

export const {
  onNodeDragStop,
  onNodesChange,
  onEdgesChange,
  onConnect,
  onEdgeUpdateStart,
  onEdgeUpdate,
  onEdgeUpdateEnd,
  screenToFlowCoordinate,
  updateNode,
  updateNodeData,
  fitView,
  findNode,
} = store;
