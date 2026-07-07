// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Graph analysis over the typed diagram model. Pure functions only (no Vue, no
// DOM) so they are trivially unit-testable and reusable.
//
// Direction convention: an edge `source -> target` means "source depends on /
// uses / calls target". Therefore:
//   - "dependents"  traverses edges BACKWARD (pred): who transitively relies on
//     this node. This answers "if this DB dies, what breaks?".
//   - "dependsOn"   traverses edges FORWARD (succ): what this node transitively
//     relies on.
// Because users may draw a relation either way, callers expose a direction
// toggle rather than hard-coding one.
//
// Containers (node.type === "container") are structural groupings, not part of
// the dependency graph, so they are excluded from orphan detection. Reachability
// and SPOF/cycle analysis operate purely on `edges`; node nesting is ignored.

import type { Diagram } from "../model";

export interface Adjacency {
  // successors: source -> set of targets it points at.
  succ: Map<string, Set<string>>;
  // predecessors: target -> set of sources pointing at it.
  pred: Map<string, Set<string>>;
}

export type Direction = "dependents" | "dependsOn";

// Build both adjacency maps in one pass. Edges whose endpoints are not real
// nodes (dangling references) are skipped so traversal never invents ids.
export function buildAdjacency(d: Diagram): Adjacency {
  const succ = new Map<string, Set<string>>();
  const pred = new Map<string, Set<string>>();
  for (const node of d.nodes) {
    succ.set(node.id, new Set());
    pred.set(node.id, new Set());
  }
  for (const edge of d.edges) {
    if (!succ.has(edge.source) || !succ.has(edge.target)) continue;
    succ.get(edge.source)!.add(edge.target);
    pred.get(edge.target)!.add(edge.source);
  }
  return { succ, pred };
}

// Transitive impact set of a single node, excluding the seed itself.
export function blastRadius(d: Diagram, seed: string, dir: Direction = "dependents"): Set<string> {
  const seen = reach(buildAdjacency(d), [seed], dir);
  seen.delete(seed);
  return seen;
}

// Combined impact of taking a set of nodes "down": every node that transitively
// depends on any downed node, excluding the downed nodes themselves.
export function simulateDown(
  d: Diagram,
  down: Iterable<string>,
  dir: Direction = "dependents",
): Set<string> {
  const downSet = new Set(down);
  const seen = reach(buildAdjacency(d), downSet, dir);
  for (const id of downSet) seen.delete(id);
  return seen;
}

// Breadth-first reachability from a set of seeds along the chosen direction.
function reach(adj: Adjacency, seeds: Iterable<string>, dir: Direction): Set<string> {
  const step = dir === "dependents" ? adj.pred : adj.succ;
  const seen = new Set<string>();
  const stack = [...seeds];
  while (stack.length) {
    const id = stack.pop()!;
    for (const next of step.get(id) ?? []) {
      if (!seen.has(next)) {
        seen.add(next);
        stack.push(next);
      }
    }
  }
  // Seeds are not part of their own impact set unless a cycle re-adds them; the
  // callers that care (blastRadius/simulateDown) strip the seeds explicitly.
  return seen;
}

// Nodes with no incident edge. Containers are excluded (they group nodes rather
// than participate in the dependency graph).
export function orphans(d: Diagram): string[] {
  const degree = new Map<string, number>();
  for (const node of d.nodes) {
    if (node.type !== "container") degree.set(node.id, 0);
  }
  for (const edge of d.edges) {
    if (degree.has(edge.source)) degree.set(edge.source, degree.get(edge.source)! + 1);
    if (edge.source !== edge.target && degree.has(edge.target)) {
      degree.set(edge.target, degree.get(edge.target)! + 1);
    }
  }
  return [...degree.entries()].filter(([, count]) => count === 0).map(([id]) => id);
}

// Dependency cycles: strongly connected components of size > 1 (Tarjan), plus
// any single node with a self-loop. Each returned array lists the members of one
// cycle.
export function findCycles(d: Diagram): string[][] {
  const { succ } = buildAdjacency(d);
  let counter = 0;
  const index = new Map<string, number>();
  const low = new Map<string, number>();
  const stack: string[] = [];
  const onStack = new Set<string>();
  const components: string[][] = [];

  function connect(v: string) {
    index.set(v, counter);
    low.set(v, counter);
    counter++;
    stack.push(v);
    onStack.add(v);
    for (const w of succ.get(v) ?? []) {
      if (!index.has(w)) {
        connect(w);
        low.set(v, Math.min(low.get(v)!, low.get(w)!));
      } else if (onStack.has(w)) {
        low.set(v, Math.min(low.get(v)!, index.get(w)!));
      }
    }
    if (low.get(v) === index.get(v)) {
      const component: string[] = [];
      let w: string;
      do {
        w = stack.pop()!;
        onStack.delete(w);
        component.push(w);
      } while (w !== v);
      components.push(component);
    }
  }

  for (const node of d.nodes) {
    if (!index.has(node.id)) connect(node.id);
  }

  const selfLoops = new Set(
    d.edges.filter((edge) => edge.source === edge.target).map((edge) => edge.source),
  );
  return components.filter(
    (component) => component.length > 1 || (component.length === 1 && selfLoops.has(component[0])),
  );
}

// Single points of failure: articulation points of the UNDIRECTED projection of
// the graph. A node is an articulation point when removing it disconnects some
// part of the graph from another - i.e. traffic between two regions has no
// alternate path around it. Standard DFS lowlink over each connected component.
export function singlePointsOfFailure(d: Diagram): string[] {
  const adj = new Map<string, Set<string>>();
  for (const node of d.nodes) adj.set(node.id, new Set());
  for (const edge of d.edges) {
    if (edge.source === edge.target) continue;
    if (!adj.has(edge.source) || !adj.has(edge.target)) continue;
    adj.get(edge.source)!.add(edge.target);
    adj.get(edge.target)!.add(edge.source);
  }

  const visited = new Set<string>();
  const disc = new Map<string, number>();
  const low = new Map<string, number>();
  const articulation = new Set<string>();
  let timer = 0;

  function dfs(u: string, parent: string | null) {
    visited.add(u);
    disc.set(u, timer);
    low.set(u, timer);
    timer++;
    let children = 0;
    for (const v of adj.get(u) ?? []) {
      if (v === parent) continue;
      if (visited.has(v)) {
        low.set(u, Math.min(low.get(u)!, disc.get(v)!));
      } else {
        children++;
        dfs(v, u);
        low.set(u, Math.min(low.get(u)!, low.get(v)!));
        if (parent !== null && low.get(v)! >= disc.get(u)!) articulation.add(u);
      }
    }
    // A DFS-tree root is an articulation point only with 2+ children.
    if (parent === null && children > 1) articulation.add(u);
  }

  for (const node of d.nodes) {
    if (!visited.has(node.id)) dfs(node.id, null);
  }
  return [...articulation];
}
