<!--
cueto

Copyright: 2026, Lucas Jahier - Stratorys
License: Mozilla Public License v2.0 (MPL v2.0)
SPDX-License-Identifier: MPL-2.0
-->

<script setup lang="ts">
// The project file tree: the open project's .cue files (from loadProject's tree
// read) rendered as a collapsible dir/file explorer. It is a pure view over the
// flat slash paths in `files`; clicking a file activates it (the same setActiveFile
// the tabs use), so tree and tab bar stay in lockstep. Subdirectories in a path
// (sub/extra.cue) become nested folders.
import { computed, ref } from "vue";
import {
  ChevronDown,
  ChevronRight,
  FileCode2,
  FolderPlus,
  PanelLeftClose,
  Trash2,
} from "lucide-vue-next";
import type { EditorFile } from "../model";
import { isDirty } from "../composables/useEditorFiles";

const props = defineProps<{
  files: EditorFile[];
  activeFile: string;
}>();
const emit = defineEmits<{
  select: [name: string];
  addFile: [];
  collapse: [];
  delete: [name: string];
}>();

// A node in the built tree: a directory (with children) or a leaf file. `path` is
// the full slash path from the module root, which for a file is its editor name.
interface TreeNode {
  name: string;
  path: string;
  dir: boolean;
  children: TreeNode[];
}

// Build the nested tree from the flat slash paths, splitting each on "/". Each
// intermediate segment is a directory node; the last is the file leaf.
function buildTree(paths: string[]): TreeNode[] {
  const root: TreeNode = { name: "", path: "", dir: true, children: [] };
  for (const full of paths) {
    const parts = full.split("/");
    let node = root;
    let acc = "";
    parts.forEach((part, i) => {
      acc = acc ? `${acc}/${part}` : part;
      const isFile = i === parts.length - 1;
      let child = node.children.find((c) => c.name === part && c.dir === !isFile);
      if (!child) {
        child = { name: part, path: acc, dir: !isFile, children: [] };
        node.children.push(child);
      }
      node = child;
    });
  }
  const sortRec = (n: TreeNode) => {
    // Directories first, then files, each alphabetical - the conventional explorer order.
    n.children.sort((a, b) => (a.dir === b.dir ? a.name.localeCompare(b.name) : a.dir ? -1 : 1));
    n.children.forEach(sortRec);
  };
  sortRec(root);
  return root.children;
}

// The built tree, recomputed when the file set changes.
const tree = computed(() => buildTree(props.files.map((f) => f.name)));

// Collapsed directory paths. A fresh Set is assigned on every change so it is
// tracked; an empty set means every folder is expanded.
const collapsed = ref(new Set<string>());
function toggleDir(path: string) {
  const next = new Set(collapsed.value);
  if (next.has(path)) next.delete(path);
  else next.add(path);
  collapsed.value = next;
}

// One visible row: a tree node plus its indentation depth. Descendants of a
// collapsed directory are omitted.
interface Row {
  node: TreeNode;
  depth: number;
}

const rows = computed<Row[]>(() => {
  const out: Row[] = [];
  const walk = (nodes: TreeNode[], depth: number) => {
    for (const node of nodes) {
      out.push({ node, depth });
      if (node.dir && !collapsed.value.has(node.path)) walk(node.children, depth + 1);
    }
  };
  walk(tree.value, 0);
  return out;
});
</script>

<template>
  <div class="flex h-full flex-col overflow-hidden bg-slate-900 text-slate-300">
    <div class="flex items-center justify-between px-3 py-2 text-xs font-medium tracking-wide text-slate-500 uppercase">
      <span>Files</span>
      <div class="flex items-center gap-0.5">
        <button
          type="button"
          class="flex h-5 w-5 items-center justify-center rounded text-slate-400 hover:bg-slate-800 hover:text-slate-200"
          title="New file"
          @click="emit('addFile')"
        >
          <FolderPlus class="h-3.5 w-3.5" />
        </button>
        <button
          type="button"
          class="flex h-5 w-5 items-center justify-center rounded text-slate-400 hover:bg-slate-800 hover:text-slate-200"
          title="Hide files"
          @click="emit('collapse')"
        >
          <PanelLeftClose class="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
    <div class="min-h-0 flex-1 overflow-y-auto pb-2">
      <button
        v-for="row in rows"
        :key="row.node.path"
        type="button"
        class="group flex w-full items-center gap-1 py-1 pr-2 text-left font-mono text-xs hover:bg-slate-800"
        :class="!row.node.dir && row.node.path === activeFile ? 'bg-slate-800 text-amber-400' : 'text-slate-300'"
        :style="{ paddingLeft: 8 + row.depth * 12 + 'px' }"
        @click="row.node.dir ? toggleDir(row.node.path) : emit('select', row.node.path)"
      >
        <template v-if="row.node.dir">
          <ChevronDown v-if="!collapsed.has(row.node.path)" class="h-3.5 w-3.5 shrink-0 text-slate-500" />
          <ChevronRight v-else class="h-3.5 w-3.5 shrink-0 text-slate-500" />
          <span class="truncate text-slate-400">{{ row.node.name }}</span>
        </template>
        <template v-else>
          <FileCode2 class="ml-3.5 h-3.5 w-3.5 shrink-0 text-slate-500" />
          <span class="truncate">{{ row.node.name }}</span>
          <span class="relative ml-auto inline-flex h-3.5 w-3.5 shrink-0 items-center justify-center">
            <span
              v-if="isDirty(row.node.path)"
              class="h-1.5 w-1.5 rounded-full bg-slate-400 group-hover:hidden"
              title="Unsaved changes"
            />
            <span
              class="hidden text-slate-500 hover:text-red-400 group-hover:inline-flex"
              role="button"
              title="Delete file"
              @click.stop="emit('delete', row.node.path)"
            >
              <Trash2 class="h-3.5 w-3.5" />
            </span>
          </span>
        </template>
      </button>
    </div>
  </div>
</template>
