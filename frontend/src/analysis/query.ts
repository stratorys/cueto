// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Query the diagram model as a filtered subgraph. A query is a space-separated
// list of tokens, ANDed together. Pure functions over the typed model - no CUE
// round-trip, since the model is small typed JSON already in the client.
//
// Token grammar:
//   field:value    exact match on a node property, e.g. type:table, shape:ellipse
//   field:~text    case-insensitive substring match, e.g. label:~payment
//   orphan         nodes with no incident edge
//   n-n            nodes at either end of an n-n relation, plus those edges
//
// A field token reads the property generically off the node object, so any node
// property is queryable without changes here. Result edges are the induced
// subgraph (both endpoints matched), except n-n which contributes its edges
// explicitly.

import type { Diagram, DiagramNode } from "../model";
import { orphans } from "./graph";

export interface QueryResult {
  nodeIds: Set<string>;
  edgeIds: Set<string>;
}

// One token's contribution: the nodes it matches, and any edges it implicates
// directly (only n-n does; other tokens leave edges to the induced-subgraph pass).
interface TokenMatch {
  nodes: Set<string>;
  edges: Set<string>;
}

export function runQuery(diagram: Diagram, text: string): QueryResult {
  const tokens = text.trim().split(/\s+/).filter(Boolean);
  if (tokens.length === 0) return { nodeIds: new Set(), edgeIds: new Set() };

  const matches = tokens.map((token) => matchToken(diagram, token));

  // AND across tokens: intersect matched nodes.
  let nodeIds = matches[0].nodes;
  for (const match of matches.slice(1)) {
    nodeIds = intersect(nodeIds, match.nodes);
  }

  const explicitEdges = union(matches.map((m) => m.edges));
  const edgeIds = new Set<string>();
  if (explicitEdges.size > 0) {
    // Keep only explicit (e.g. n-n) edges whose endpoints survived the AND.
    for (const edge of diagram.edges) {
      if (explicitEdges.has(edge.id) && nodeIds.has(edge.source) && nodeIds.has(edge.target)) {
        edgeIds.add(edge.id);
      }
    }
  } else {
    // Induced subgraph: edges with both endpoints in the matched node set.
    for (const edge of diagram.edges) {
      if (nodeIds.has(edge.source) && nodeIds.has(edge.target)) edgeIds.add(edge.id);
    }
  }

  return { nodeIds, edgeIds };
}

function matchToken(diagram: Diagram, token: string): TokenMatch {
  const empty = (): TokenMatch => ({ nodes: new Set(), edges: new Set() });

  if (token === "orphan") {
    return { nodes: new Set(orphans(diagram)), edges: new Set() };
  }
  if (token === "n-n") {
    const nodes = new Set<string>();
    const edges = new Set<string>();
    for (const edge of diagram.edges) {
      if (edge.card === "n-n") {
        nodes.add(edge.source);
        nodes.add(edge.target);
        edges.add(edge.id);
      }
    }
    return { nodes, edges };
  }

  const colon = token.indexOf(":");
  if (colon === -1) return empty(); // unknown bare keyword matches nothing

  const field = token.slice(0, colon);
  const raw = token.slice(colon + 1);
  const substring = raw.startsWith("~");
  const value = substring ? raw.slice(1).toLowerCase() : raw;

  const nodes = new Set<string>();
  for (const node of diagram.nodes) {
    // A field token may name any node property; a non-key string simply misses
    // (yields undefined). Casting the key keeps the value fully typed - no unknown.
    const property = node[field as keyof DiagramNode];
    if (property === undefined) continue;
    const text = String(property);
    const hit = substring ? text.toLowerCase().includes(value) : text === value;
    if (hit) nodes.add(node.id);
  }
  return { nodes, edges: new Set() };
}

function intersect(a: Set<string>, b: Set<string>): Set<string> {
  const out = new Set<string>();
  for (const id of a) if (b.has(id)) out.add(id);
  return out;
}

function union(sets: Set<string>[]): Set<string> {
  const out = new Set<string>();
  for (const set of sets) for (const id of set) out.add(id);
  return out;
}
