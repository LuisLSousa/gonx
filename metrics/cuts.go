package metrics

import (
	"fmt"

	"github.com/LuisLSousa/gonx"
)

// Bridge is an edge whose removal splits its connected component in two. V is
// the endpoint on the side that does not contain the component's smallest node,
// and Side is the number of nodes on that side, V included; the other side
// holds the rest of the component. SideWeight is the sum of the caller's node
// weights over the same nodes, or Side itself when no weights were given.
type Bridge struct {
	U, V       int
	Side       int
	SideWeight float64
}

// Bridges returns every bridge of g, in ascending order of V. A node is the V
// of at most one bridge, so V alone identifies each entry. Edge weights play no
// part: whether an edge is a bridge depends only on the graph's shape.
//
// nodeWeight, when non-nil, gives each node a weight such as a population or a
// number of servers, and every bridge then reports the total weight it cuts
// off in SideWeight. It must have length g.NumNodes(). Computing the sums here
// costs one addition per node, while computing them afterwards would take a
// traversal per bridge. They are plain float64 sums, so a NaN weight makes
// every SideWeight that includes it NaN, as do infinities of both signs.
//
// The result matches networkx.bridges up to the orientation of each pair,
// which networkx does not specify. It runs in O(n + m) time on an iterative
// depth-first search, so path-like graphs with millions of nodes need no deep
// call stack. It panics if nodeWeight is non-nil with the wrong length.
func Bridges(g *gonx.Graph, nodeWeight []float64) []Bridge {
	n := g.NumNodes()
	if nodeWeight != nil && len(nodeWeight) != n {
		panic(fmt.Sprintf("gonx/metrics: Bridges: nodeWeight has length %d, want %d", len(nodeWeight), n))
	}
	t := lowLink(g, nodeWeight)
	isBridge := func(v int) bool {
		p := t.parent[v]
		return p >= 0 && t.low[v] > t.disc[p]
	}
	// Counting first sizes the result exactly. Most edges of a tree-like graph
	// are bridges, and growing the slice by append would allocate several
	// times its final size on the way.
	k := 0
	for v := range n {
		if isBridge(v) {
			k++
		}
	}
	if k == 0 {
		return nil
	}
	out := make([]Bridge, 0, k)
	for v := range n {
		if !isBridge(v) {
			continue
		}
		p := t.parent[v]
		b := Bridge{U: int(p), V: v, Side: int(t.size[v]), SideWeight: float64(t.size[v])}
		if t.weight != nil {
			b.SideWeight = t.weight[v]
		}
		out = append(out, b)
	}
	return out
}

// ArticulationPoints returns, in ascending order, the nodes whose removal,
// together with their edges, splits their connected component into more than
// one piece. It matches networkx.articulation_points and runs in O(n + m) time
// on the same search as [Bridges].
func ArticulationPoints(g *gonx.Graph) []int {
	n := g.NumNodes()
	t := lowLink(g, nil)
	// cut[u] counts u's tree children while u is a root, saturating at 2, and
	// is set to 2 outright when a child's subtree cannot reach above u.
	cut := make([]uint8, n)
	for v := range n {
		p := t.parent[v]
		switch {
		case p < 0:
		case t.parent[p] < 0:
			// A root is a cut node when it has two or more tree children:
			// they can only reach each other through it.
			if cut[p] < 2 {
				cut[p]++
			}
		case t.low[v] >= t.disc[p]:
			cut[p] = 2
		}
	}
	k := 0
	for _, c := range cut {
		if c == 2 {
			k++
		}
	}
	if k == 0 {
		return nil
	}
	out := make([]int, 0, k) // sized exactly, as in Bridges
	for u, c := range cut {
		if c == 2 {
			out = append(out, u)
		}
	}
	return out
}

// dfsForest is what one depth-first pass over every component records. disc[u]
// is u's discovery time, low[u] the smallest discovery time reachable from u's
// subtree through at most one non-tree edge, parent[u] u's tree parent or -1
// for a root, and size[u] the number of nodes in u's subtree. weight[u] is the
// sum of the node weights over that subtree, and nil when none were given.
type dfsForest struct {
	disc, low, parent, size []int32
	weight                  []float64
}

// lowLink runs Tarjan's low-link search over g with an explicit stack. Roots
// are taken in ascending node order, so each tree is rooted at the smallest
// node of its component, and neighbors are scanned in their stored, ascending
// order, which makes every result derived from the forest deterministic.
func lowLink(g *gonx.Graph, nodeWeight []float64) dfsForest {
	n := g.NumNodes()
	t := dfsForest{
		disc:   make([]int32, n),
		low:    make([]int32, n),
		parent: make([]int32, n),
		size:   make([]int32, n),
	}
	if nodeWeight != nil {
		t.weight = make([]float64, n)
	}
	for i := range t.disc {
		t.disc[i] = -1
	}
	// next[u] is how far u's neighbor list has been scanned, which is the
	// whole of a recursive frame that the explicit stack needs to remember.
	next := make([]int32, n)
	stack := make([]int32, 0, 64)
	var clock int32
	visit := func(v, p int32) {
		t.disc[v], t.low[v], t.parent[v], t.size[v] = clock, clock, p, 1
		clock++
		if t.weight != nil {
			t.weight[v] = nodeWeight[v]
		}
		stack = append(stack, v)
	}
	for root := range n {
		if t.disc[root] >= 0 {
			continue
		}
		visit(int32(root), -1)
		for len(stack) > 0 {
			u := stack[len(stack)-1]
			nbrs := g.Neighbors(int(u))
			if i := next[u]; int(i) < len(nbrs) {
				next[u]++
				v := nbrs[i]
				if t.disc[v] < 0 {
					visit(v, u)
				} else if v != t.parent[u] {
					// A back edge. Skipping the parent is enough to ignore the
					// tree edge itself, because a Graph has no parallel edges.
					t.low[u] = min(t.low[u], t.disc[v])
				}
				continue
			}
			stack = stack[:len(stack)-1]
			if p := t.parent[u]; p >= 0 {
				t.low[p] = min(t.low[p], t.low[u])
				t.size[p] += t.size[u]
				if t.weight != nil {
					t.weight[p] += t.weight[u]
				}
			}
		}
	}
	return t
}
