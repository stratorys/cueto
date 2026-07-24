package graph

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"testing"
)

// hasNode reports whether the subtree contains id.
func hasNode(s Subtree, id NodeID) bool {
	for _, n := range s.Nodes {
		if n == id {
			return true
		}
	}
	return false
}

// hasEdge reports whether the subtree contains the undirected edge (a, b).
func hasEdge(s Subtree, a, b NodeID) bool {
	if a > b {
		a, b = b, a
	}
	for _, e := range s.Edges {
		if e.A == a && e.B == b {
			return true
		}
	}
	return false
}

// TestH3_ZeroPrizeLeafPruned: a leaf terminal whose prize does not cover its edge is
// dropped, while the paying terminal it connected to remains.
func TestH3_ZeroPrizeLeafPruned(t *testing.T) {
	nodes := []Node{{ID: 1, Prize: 0}, {ID: 2, Prize: 10}}
	edges := []Edge{{A: 1, B: 2, Weight: 1}}

	got := Extract(nodes, edges, []NodeID{1, 2}, 1)

	if hasNode(got, 1) {
		t.Fatalf("zero-prize leaf 1 should be pruned, got nodes %v", got.Nodes)
	}
	if !hasNode(got, 2) {
		t.Fatalf("paying terminal 2 should remain, got nodes %v", got.Nodes)
	}
	if len(got.Edges) != 0 {
		t.Fatalf("no edges expected after pruning the leaf, got %v", got.Edges)
	}
}

// TestH3_ZeroPrizeBridgeRetained: a zero-prize node on the only path between two paying
// terminals is internal, never a leaf, and survives pruning.
func TestH3_ZeroPrizeBridgeRetained(t *testing.T) {
	nodes := []Node{{ID: 1, Prize: 10}, {ID: 2, Prize: 0}, {ID: 3, Prize: 10}}
	edges := []Edge{{A: 1, B: 2, Weight: 1}, {A: 2, B: 3, Weight: 1}}

	got := Extract(nodes, edges, []NodeID{1, 3}, 1)

	if !hasNode(got, 2) {
		t.Fatalf("zero-prize bridge 2 should be retained, got nodes %v", got.Nodes)
	}
	if !hasNode(got, 1) || !hasNode(got, 3) {
		t.Fatalf("both terminals should remain, got nodes %v", got.Nodes)
	}
	if !hasEdge(got, 1, 2) || !hasEdge(got, 2, 3) {
		t.Fatalf("both bridge edges should remain, got edges %v", got.Edges)
	}
}

// TestH3_CostMonotonicity: raising cost never grows the subtree. A pendant terminal is
// kept at low cost and pruned at high cost, and the node count never increases as cost
// climbs.
func TestH3_CostMonotonicity(t *testing.T) {
	nodes := []Node{{ID: 1, Prize: 10}, {ID: 2, Prize: 10}, {ID: 3, Prize: 5}}
	edges := []Edge{{A: 3, B: 1, Weight: 1}, {A: 1, B: 2, Weight: 1}}
	terminals := []NodeID{1, 2, 3}

	low := Extract(nodes, edges, terminals, 1)
	high := Extract(nodes, edges, terminals, 10)

	if !hasNode(low, 3) {
		t.Fatalf("pendant terminal 3 should be kept at low cost, got %v", low.Nodes)
	}
	if hasNode(high, 3) {
		t.Fatalf("pendant terminal 3 should be pruned at high cost, got %v", high.Nodes)
	}

	prev := len(nodes) + 1
	for _, cost := range []float64{0, 0.5, 1, 5, 10, 50, 100} {
		n := len(Extract(nodes, edges, terminals, cost).Nodes)
		if n > prev {
			t.Fatalf("node count grew from %d to %d as cost rose to %v", prev, n, cost)
		}
		prev = n
	}
}

// TestH3_HashIdentity100Runs: 100 extractions of the same graph serialize identically.
func TestH3_HashIdentity100Runs(t *testing.T) {
	nodes := []Node{
		{ID: 1, Prize: 8}, {ID: 2, Prize: 0}, {ID: 3, Prize: 6},
		{ID: 4, Prize: 0}, {ID: 5, Prize: 9}, {ID: 6, Prize: 2},
		{ID: 7, Prize: 0}, {ID: 8, Prize: 7},
	}
	edges := []Edge{
		{A: 1, B: 2, Weight: 1}, {A: 2, B: 3, Weight: 2}, {A: 2, B: 4, Weight: 1},
		{A: 4, B: 5, Weight: 3}, {A: 3, B: 6, Weight: 1}, {A: 6, B: 7, Weight: 1},
		{A: 7, B: 8, Weight: 1}, {A: 1, B: 5, Weight: 10},
	}
	terminals := []NodeID{1, 5, 8}

	var want [32]byte
	for i := 0; i < 100; i++ {
		h := sha256.Sum256(serialize(Extract(nodes, edges, terminals, 1)))
		if i == 0 {
			want = h
			continue
		}
		if h != want {
			t.Fatalf("run %d produced a different hash than run 0", i)
		}
	}
}

// TestExtractEmpty: no nodes yields an empty, non-nil result.
func TestExtractEmpty(t *testing.T) {
	got := Extract(nil, nil, nil, 1)
	if got.Nodes == nil {
		t.Fatalf("Nodes should be non-nil")
	}
	if len(got.Nodes) != 0 || len(got.Edges) != 0 {
		t.Fatalf("expected empty subtree, got %v", got)
	}
}

// TestExtractSingleTerminal: an isolated terminal is kept with no edges.
func TestExtractSingleTerminal(t *testing.T) {
	got := Extract([]Node{{ID: 7, Prize: 0}}, nil, []NodeID{7}, 5)
	if !hasNode(got, 7) || len(got.Nodes) != 1 || len(got.Edges) != 0 {
		t.Fatalf("expected sole node 7, got %v", got)
	}
}

// TestExtractDisconnectedTerminals: terminals in two components yield a forest, each
// component connected independently.
func TestExtractDisconnectedTerminals(t *testing.T) {
	nodes := []Node{
		{ID: 1, Prize: 10}, {ID: 2, Prize: 10},
		{ID: 3, Prize: 10}, {ID: 4, Prize: 10},
	}
	edges := []Edge{{A: 1, B: 2, Weight: 1}, {A: 3, B: 4, Weight: 1}}

	got := Extract(nodes, edges, []NodeID{1, 2, 3, 4}, 1)

	if len(got.Nodes) != 4 {
		t.Fatalf("expected all 4 nodes across both components, got %v", got.Nodes)
	}
	if !hasEdge(got, 1, 2) || !hasEdge(got, 3, 4) {
		t.Fatalf("expected one edge per component, got %v", got.Edges)
	}
	if len(got.Edges) != 2 {
		t.Fatalf("expected exactly 2 edges, got %v", got.Edges)
	}
}

// TestExtractUndeclaredTerminalSkipped: a terminal absent from the node set is ignored.
func TestExtractUndeclaredTerminalSkipped(t *testing.T) {
	nodes := []Node{{ID: 1, Prize: 10}, {ID: 2, Prize: 10}}
	edges := []Edge{{A: 1, B: 2, Weight: 1}}

	got := Extract(nodes, edges, []NodeID{1, 2, 99}, 1)

	if hasNode(got, 99) {
		t.Fatalf("undeclared terminal 99 must not appear, got %v", got.Nodes)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("expected the two declared terminals, got %v", got.Nodes)
	}
}

// serialize renders a subtree to a stable byte form for hashing. Inputs are already
// sorted by Extract; this only fixes an encoding.
func serialize(s Subtree) []byte {
	nodes := append([]NodeID(nil), s.Nodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i] < nodes[j] })
	edges := append([]Edge(nil), s.Edges...)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].A != edges[j].A {
			return edges[i].A < edges[j].A
		}
		return edges[i].B < edges[j].B
	})

	out := make([]byte, 0, 64)
	out = append(out, "N:"...)
	for _, n := range nodes {
		out = append(out, fmt.Sprintf("%d,", n)...)
	}
	out = append(out, "E:"...)
	for _, e := range edges {
		out = append(out, fmt.Sprintf("%d-%d@%g,", e.A, e.B, e.Weight)...)
	}
	return out
}
