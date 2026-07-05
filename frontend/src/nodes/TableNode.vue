<script setup lang="ts">
import { Handle, Position } from "@vue-flow/core";
import type { Column } from "../model";

// Custom node for a DB table: header + one row per column.
// Each column exposes a left (target) and right (source) handle so relations
// can attach to a specific column, not just the node.
defineProps<{
  data: { label: string; columns?: Column[] };
}>();
</script>

<template>
  <div class="table-node">
    <div class="header">{{ data.label }}</div>
    <div v-for="col in data.columns" :key="col.name" class="col">
      <Handle
        :id="`${col.name}-target`"
        type="target"
        :position="Position.Left"
      />
      <span class="name">
        {{ col.name }}
        <span v-if="col.pk" class="badge pk">PK</span>
        <span v-if="col.fk" class="badge fk">FK</span>
      </span>
      <span class="dbtype">{{ col.dbType }}</span>
      <Handle
        :id="`${col.name}-source`"
        type="source"
        :position="Position.Right"
      />
    </div>
  </div>
</template>

<style scoped>
.table-node {
  min-width: 180px;
  border: 1px solid #94a3b8;
  border-radius: 6px;
  background: #fff;
  font-size: 13px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.15);
}

.header {
  background: #1e293b;
  color: #fff;
  font-weight: 600;
  padding: 6px 10px;
  text-align: center;
  border-radius: 5px 5px 0 0;
}

.col {
  position: relative;
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 5px 10px;
  border-top: 1px solid #e2e8f0;
}

.name {
  color: #0f172a;
}

.dbtype {
  color: #64748b;
  font-family: ui-monospace, monospace;
}

.badge {
  font-size: 10px;
  padding: 0 4px;
  border-radius: 3px;
  margin-left: 4px;
  vertical-align: middle;
}

.badge.pk {
  background: #fde68a;
  color: #92400e;
}

.badge.fk {
  background: #bfdbfe;
  color: #1e40af;
}

.col :deep(.vue-flow__handle) {
  width: 12px;
  height: 12px;
  background: #64748b;
  border: 2px solid #fff;
  z-index: 2;
}

/* Grow the interactive hit area beyond the visible dot without enlarging it. */
.col :deep(.vue-flow__handle)::after {
  content: "";
  position: absolute;
  inset: -6px;
}
</style>
