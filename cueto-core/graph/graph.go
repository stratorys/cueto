// Package graph implements the prize-collecting subtree heuristic that connects a
// set of terminal nodes through a weighted graph while pruning branches that do not
// pay for themselves. It imports nothing, not even other core packages: callers adapt
// their own identifiers to NodeID at the boundary.
//
// The method is the Kou-Markowsky-Berman (KMB) Steiner-tree construction generalized
// with prizes: shortest paths between terminals, a minimum spanning tree over that
// metric closure, expansion of each spanning edge back to its underlying path, and a
// spanning tree over the induced subgraph to remove any cycles the expansion created.
// A prize-collecting prune then drops every terminal-free branch whose collected prize
// does not cover its edge cost. Every connected component is evaluated independently,
// so disconnected terminals yield a forest.
//
// Determinism is the whole contract. Every tie-break resolves by NodeID order, and no
// map iteration reaches an output without a sort at the emit point, so repeated calls
// on identical input produce byte-identical results.
package graph

import (
	"container/heap"
	"math"
	"sort"
)

// NodeID identifies a node. Callers map their own addresses onto this space.
type NodeID int32

// Node is a candidate node carrying the prize collected when it is included.
type Node struct {
	ID    NodeID
	Prize float64
}

// Edge is an undirected connection whose Weight is the distance paid to traverse it.
type Edge struct {
	A, B   NodeID
	Weight float64
}

// Subtree is the extracted result. Nodes is sorted ascending; Edges is sorted by
// (A, B) with A < B. Across disconnected components the result is a forest.
type Subtree struct {
	Nodes []NodeID
	Edges []Edge
}

// Extract connects the terminals through the graph, pruning terminal-free branches
// whose prize does not cover cost times their edge weight, and returns the resulting
// subtree (a forest when terminals span multiple components). Nodes and edges not
// declared in nodes are ignored; terminals absent from nodes or unreachable are
// skipped. The result is empty when no declared terminal exists.
func Extract(nodes []Node, edges []Edge, terminals []NodeID, cost float64) Subtree {
	g := build(nodes, edges)

	termSet := make(map[NodeID]bool, len(terminals))
	for _, t := range terminals {
		if _, ok := g.prize[t]; ok {
			termSet[t] = true
		}
	}

	outNodes := make(map[NodeID]bool)
	outEdges := make(map[edgeKey]float64)

	for _, comp := range g.components() {
		compTerms := make([]NodeID, 0, len(comp))
		for _, id := range comp {
			if termSet[id] {
				compTerms = append(compTerms, id)
			}
		}
		if len(compTerms) == 0 {
			continue
		}
		sort.Slice(compTerms, func(i, j int) bool { return compTerms[i] < compTerms[j] })

		treeNodes, treeEdges := g.steinerTree(compTerms)
		keepNodes, keepEdges := prune(treeNodes, treeEdges, g.prize, cost)

		for id := range keepNodes {
			outNodes[id] = true
		}
		for k, w := range keepEdges {
			outEdges[k] = w
		}
	}

	return assemble(outNodes, outEdges)
}

// edgeKey is a normalized undirected edge with Lo < Hi, used as a map key so that
// duplicate and reversed edges collapse to one entry.
type edgeKey struct{ Lo, Hi NodeID }

func makeKey(a, b NodeID) edgeKey {
	if a > b {
		a, b = b, a
	}
	return edgeKey{Lo: a, Hi: b}
}

// graphData is the working graph: declared prizes plus a symmetric adjacency map that
// keeps the minimum weight seen for each pair.
type graphData struct {
	prize map[NodeID]float64
	adj   map[NodeID]map[NodeID]float64
}

func build(nodes []Node, edges []Edge) *graphData {
	g := &graphData{
		prize: make(map[NodeID]float64, len(nodes)),
		adj:   make(map[NodeID]map[NodeID]float64, len(nodes)),
	}
	for _, n := range nodes {
		g.prize[n.ID] = n.Prize
		if g.adj[n.ID] == nil {
			g.adj[n.ID] = make(map[NodeID]float64)
		}
	}
	for _, e := range edges {
		if e.A == e.B {
			continue
		}
		if _, ok := g.prize[e.A]; !ok {
			continue
		}
		if _, ok := g.prize[e.B]; !ok {
			continue
		}
		g.link(e.A, e.B, e.Weight)
	}
	return g
}

func (g *graphData) link(a, b NodeID, w float64) {
	if cur, ok := g.adj[a][b]; !ok || w < cur {
		g.adj[a][b] = w
		g.adj[b][a] = w
	}
}

// sortedNodes returns every declared node ID in ascending order, the deterministic
// spine that all component and iteration logic is driven from.
func (g *graphData) sortedNodes() []NodeID {
	ids := make([]NodeID, 0, len(g.prize))
	for id := range g.prize {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// neighbors returns a node's adjacent IDs in ascending order.
func (g *graphData) neighbors(id NodeID) []NodeID {
	ns := make([]NodeID, 0, len(g.adj[id]))
	for n := range g.adj[id] {
		ns = append(ns, n)
	}
	sort.Slice(ns, func(i, j int) bool { return ns[i] < ns[j] })
	return ns
}

// components partitions declared nodes into connected components via union-find,
// returning each component's members sorted ascending, ordered by their smallest ID.
func (g *graphData) components() [][]NodeID {
	parent := make(map[NodeID]NodeID, len(g.prize))
	for _, id := range g.sortedNodes() {
		parent[id] = id
	}
	var find func(NodeID) NodeID
	find = func(x NodeID) NodeID {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b NodeID) {
		ra, rb := find(a), find(b)
		if ra == rb {
			return
		}
		if ra < rb {
			parent[rb] = ra
		} else {
			parent[ra] = rb
		}
	}
	for _, a := range g.sortedNodes() {
		for _, b := range g.neighbors(a) {
			union(a, b)
		}
	}

	groups := make(map[NodeID][]NodeID)
	for _, id := range g.sortedNodes() {
		r := find(id)
		groups[r] = append(groups[r], id)
	}
	roots := make([]NodeID, 0, len(groups))
	for r := range groups {
		roots = append(roots, r)
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i] < roots[j] })

	out := make([][]NodeID, 0, len(roots))
	for _, r := range roots {
		out = append(out, groups[r])
	}
	return out
}

// steinerTree builds the KMB tree spanning terms within a single component and returns
// its node set and normalized edge set. A single terminal yields just that node.
func (g *graphData) steinerTree(terms []NodeID) (map[NodeID]bool, map[edgeKey]float64) {
	if len(terms) == 1 {
		return map[NodeID]bool{terms[0]: true}, map[edgeKey]float64{}
	}

	dist := make(map[NodeID]map[NodeID]float64, len(terms))
	pred := make(map[NodeID]map[NodeID]NodeID, len(terms))
	for _, t := range terms {
		dist[t], pred[t] = g.dijkstra(t)
	}

	// Minimum spanning tree over the terminal metric closure (Kruskal, tie-break by
	// weight then endpoint IDs), then expand each chosen pair back to its path.
	type closureEdge struct {
		u, v NodeID
		w    float64
	}
	closure := make([]closureEdge, 0, len(terms)*(len(terms)-1)/2)
	for i := 0; i < len(terms); i++ {
		for j := i + 1; j < len(terms); j++ {
			u, v := terms[i], terms[j]
			w, ok := dist[u][v]
			if !ok || math.IsInf(w, 1) {
				continue
			}
			closure = append(closure, closureEdge{u: u, v: v, w: w})
		}
	}
	sort.Slice(closure, func(i, j int) bool {
		if closure[i].w != closure[j].w {
			return closure[i].w < closure[j].w
		}
		if closure[i].u != closure[j].u {
			return closure[i].u < closure[j].u
		}
		return closure[i].v < closure[j].v
	})

	uf := newUnionFind(terms)
	nodes := make(map[NodeID]bool)
	induced := make(map[edgeKey]float64)
	for _, ce := range closure {
		if !uf.union(ce.u, ce.v) {
			continue
		}
		g.expandPath(ce.v, ce.u, pred, nodes, induced)
	}

	// Spanning tree over the induced subgraph removes cycles the path union created.
	return spanningTree(nodes, induced)
}

// dijkstra returns shortest-path distances and predecessors from src. Ties on distance
// break toward the smaller predecessor ID so reconstructed paths are deterministic.
func (g *graphData) dijkstra(src NodeID) (map[NodeID]float64, map[NodeID]NodeID) {
	dist := map[NodeID]float64{src: 0}
	pred := map[NodeID]NodeID{}
	pq := &nodeHeap{{id: src, dist: 0}}
	heap.Init(pq)

	for pq.Len() > 0 {
		cur := heap.Pop(pq).(nodeDist)
		if d, ok := dist[cur.id]; ok && cur.dist > d {
			continue
		}
		for _, nb := range g.neighbors(cur.id) {
			nd := cur.dist + g.adj[cur.id][nb]
			old, seen := dist[nb]
			if !seen || nd < old || (nd == old && cur.id < pred[nb]) {
				dist[nb] = nd
				pred[nb] = cur.id
				heap.Push(pq, nodeDist{id: nb, dist: nd})
			}
		}
	}
	return dist, pred
}

// expandPath walks predecessors from target back to src, recording each hop's node and
// original edge weight into the accumulating node and edge sets.
func (g *graphData) expandPath(target, src NodeID, pred map[NodeID]map[NodeID]NodeID, nodes map[NodeID]bool, edges map[edgeKey]float64) {
	p := pred[src]
	cur := target
	nodes[cur] = true
	for cur != src {
		prev, ok := p[cur]
		if !ok {
			return
		}
		nodes[prev] = true
		w := g.adj[prev][cur]
		key := makeKey(prev, cur)
		if ex, seen := edges[key]; !seen || w < ex {
			edges[key] = w
		}
		cur = prev
	}
}

// prune iteratively removes every leaf whose prize does not cover cost times its
// connecting edge weight, cascading until a round removes nothing. A node stays a
// bridge as long as it has two or more neighbors, so a zero-prize node between two
// retained terminals is never a leaf and survives. Degree-zero nodes (an isolated
// terminal) are kept, since they pay no edge. The removal set each round is defined by
// degree and prize alone, so the outcome is independent of iteration order.
func prune(treeNodes map[NodeID]bool, treeEdges map[edgeKey]float64, prize map[NodeID]float64, cost float64) (map[NodeID]bool, map[edgeKey]float64) {
	nodes := make(map[NodeID]bool, len(treeNodes))
	for id := range treeNodes {
		nodes[id] = true
	}
	edges := make(map[edgeKey]float64, len(treeEdges))
	for k, w := range treeEdges {
		edges[k] = w
	}

	adj := make(map[NodeID]map[NodeID]float64, len(nodes))
	for id := range nodes {
		adj[id] = make(map[NodeID]float64)
	}
	for k, w := range edges {
		adj[k.Lo][k.Hi] = w
		adj[k.Hi][k.Lo] = w
	}

	for {
		remove := make([]NodeID, 0)
		for id := range nodes {
			if len(adj[id]) != 1 {
				continue
			}
			var w float64
			for _, wt := range adj[id] {
				w = wt
			}
			if prize[id] < cost*w {
				remove = append(remove, id)
			}
		}
		if len(remove) == 0 {
			break
		}
		for _, v := range remove {
			for u := range adj[v] {
				delete(adj[u], v)
				delete(edges, makeKey(v, u))
			}
			delete(adj, v)
			delete(nodes, v)
		}
	}
	return nodes, edges
}

// spanningTree returns a minimum spanning tree of the given node and edge sets via
// Kruskal, tie-broken by weight then endpoint IDs.
func spanningTree(nodes map[NodeID]bool, edges map[edgeKey]float64) (map[NodeID]bool, map[edgeKey]float64) {
	ids := make([]NodeID, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	type we struct {
		k edgeKey
		w float64
	}
	list := make([]we, 0, len(edges))
	for k, w := range edges {
		list = append(list, we{k: k, w: w})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].w != list[j].w {
			return list[i].w < list[j].w
		}
		if list[i].k.Lo != list[j].k.Lo {
			return list[i].k.Lo < list[j].k.Lo
		}
		return list[i].k.Hi < list[j].k.Hi
	})

	uf := newUnionFind(ids)
	tree := make(map[edgeKey]float64, len(nodes))
	for _, e := range list {
		if uf.union(e.k.Lo, e.k.Hi) {
			tree[e.k] = e.w
		}
	}
	kept := make(map[NodeID]bool, len(nodes))
	for id := range nodes {
		kept[id] = true
	}
	return kept, tree
}

// assemble turns the accumulated node and edge sets into a sorted Subtree.
func assemble(nodes map[NodeID]bool, edges map[edgeKey]float64) Subtree {
	ns := make([]NodeID, 0, len(nodes))
	for id := range nodes {
		ns = append(ns, id)
	}
	sort.Slice(ns, func(i, j int) bool { return ns[i] < ns[j] })

	es := make([]Edge, 0, len(edges))
	for k, w := range edges {
		es = append(es, Edge{A: k.Lo, B: k.Hi, Weight: w})
	}
	sort.Slice(es, func(i, j int) bool {
		if es[i].A != es[j].A {
			return es[i].A < es[j].A
		}
		return es[i].B < es[j].B
	})
	return Subtree{Nodes: ns, Edges: es}
}

// unionFind is a disjoint-set structure used by the Kruskal passes.
type unionFind struct{ parent map[NodeID]NodeID }

func newUnionFind(ids []NodeID) *unionFind {
	uf := &unionFind{parent: make(map[NodeID]NodeID, len(ids))}
	for _, id := range ids {
		uf.parent[id] = id
	}
	return uf
}

func (uf *unionFind) find(x NodeID) NodeID {
	for uf.parent[x] != x {
		uf.parent[x] = uf.parent[uf.parent[x]]
		x = uf.parent[x]
	}
	return x
}

// union joins two sets and reports whether they were previously disjoint. The smaller
// root wins so the structure is deterministic.
func (uf *unionFind) union(a, b NodeID) bool {
	ra, rb := uf.find(a), uf.find(b)
	if ra == rb {
		return false
	}
	if ra < rb {
		uf.parent[rb] = ra
	} else {
		uf.parent[ra] = rb
	}
	return true
}

// nodeDist and nodeHeap form the Dijkstra priority queue, ordered by distance then ID.
type nodeDist struct {
	id   NodeID
	dist float64
}

type nodeHeap []nodeDist

func (h nodeHeap) Len() int { return len(h) }
func (h nodeHeap) Less(i, j int) bool {
	if h[i].dist != h[j].dist {
		return h[i].dist < h[j].dist
	}
	return h[i].id < h[j].id
}
func (h nodeHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *nodeHeap) Push(x any)   { *h = append(*h, x.(nodeDist)) }
func (h *nodeHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}
