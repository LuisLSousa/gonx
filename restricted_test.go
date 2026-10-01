package gonx

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
)

// withoutHidden builds, the slow and obvious way, the graph a view of g should
// look like: every edge of g that is not hidden and touches no hidden node,
// with its weight, over the same node IDs.
func withoutHidden(g Adjacency, nodes []int, edges [][2]int) Adjacency {
	gone := map[int]bool{}
	for _, x := range nodes {
		gone[x] = true
	}
	_, directed := g.(*Digraph)
	cut := map[[2]int]bool{}
	for _, e := range edges {
		cut[e] = true
		if !directed {
			cut[[2]int{e[1], e[0]}] = true
		}
	}
	n := g.NumNodes()
	ub, db := NewBuilder(n), NewDigraphBuilder(n)
	if g.Weighted() {
		ub, db = NewWeightedBuilder(n), NewWeightedDigraphBuilder(n)
	}
	for u := range n {
		ws := g.OutWeights(u)
		for i, v32 := range g.OutNeighbors(u) {
			v := int(v32)
			if gone[u] || gone[v] || cut[[2]int{u, v}] || (!directed && v < u) {
				continue
			}
			switch {
			case directed && ws != nil:
				db.AddEdgeW(u, v, ws[i])
			case directed:
				db.AddEdge(u, v)
			case ws != nil:
				ub.AddEdgeW(u, v, ws[i])
			default:
				ub.AddEdge(u, v)
			}
		}
	}
	if directed {
		return db.Build()
	}
	return ub.Build()
}

// checkView compares view, made from g with nodes and edges hidden, against
// the graph withoutHidden builds: the same lists and weights at every node, the
// same flags, no spare capacity, and g's own slices wherever nothing changed.
func checkView(t *testing.T, g Adjacency, view *Restricted, nodes []int, edges [][2]int) {
	t.Helper()
	want := withoutHidden(g, nodes, edges)
	if view.NumNodes() != want.NumNodes() || view.Weighted() != want.Weighted() {
		t.Fatalf("NumNodes, Weighted = %d, %v; want %d, %v",
			view.NumNodes(), view.Weighted(), want.NumNodes(), want.Weighted())
	}
	if got := view.HasNegativeWeight(); got != want.HasNegativeWeight() {
		t.Fatalf("HasNegativeWeight = %v, want %v (hiding nodes %v, edges %v)", got, !got, nodes, edges)
	}
	for u := range g.NumNodes() {
		nbrs, ws := view.OutNeighbors(u), view.OutWeights(u)
		if !slices.Equal(nbrs, want.OutNeighbors(u)) {
			t.Fatalf("OutNeighbors(%d) = %v, want %v (hiding nodes %v, edges %v)",
				u, nbrs, want.OutNeighbors(u), nodes, edges)
		}
		if (ws == nil) == view.Weighted() {
			t.Fatalf("OutWeights(%d) = %v on a view with Weighted() = %v", u, ws, view.Weighted())
		}
		if !slices.Equal(ws, want.OutWeights(u)) {
			t.Fatalf("OutWeights(%d) = %v, want %v", u, ws, want.OutWeights(u))
		}
		if cap(nbrs) != len(nbrs) || cap(ws) != len(ws) {
			t.Fatalf("node %d: lists have spare capacity", u)
		}
		// A list the view leaves alone must be g's own, not a copy.
		orig := g.OutNeighbors(u)
		if len(orig) > 0 && len(nbrs) == len(orig) {
			if &nbrs[0] != &orig[0] || (ws != nil && &ws[0] != &g.OutWeights(u)[0]) {
				t.Fatalf("node %d is untouched but its lists are copies", u)
			}
		}
	}
}

// randomGraph returns a graph of either kind on n nodes with about m edges,
// weighted unless weighted is false, with some weights negative.
func randomGraph(r *rand.Rand, directed, weighted bool, n, m int) Adjacency {
	ub, db := NewBuilder(n), NewDigraphBuilder(n)
	for range m {
		u, v := r.IntN(n), r.IntN(n)
		w := float64(r.IntN(40)) - 4
		switch {
		case directed && weighted:
			db.AddEdgeW(u, v, w)
		case directed:
			db.AddEdge(u, v)
		case weighted:
			ub.AddEdgeW(u, v, w)
		default:
			ub.AddEdge(u, v)
		}
	}
	if directed {
		return db.Build()
	}
	return ub.Build()
}

// randomHidden picks a few nodes and edges of g to hide: mostly edges g has,
// spelled either way round, and some pairs it does not have.
func randomHidden(r *rand.Rand, g Adjacency) (nodes []int, edges [][2]int) {
	n := g.NumNodes()
	for range r.IntN(3) {
		nodes = append(nodes, r.IntN(n))
	}
	for range r.IntN(6) {
		u := r.IntN(n)
		out := g.OutNeighbors(u)
		if len(out) == 0 || r.IntN(4) == 0 {
			edges = append(edges, [2]int{u, r.IntN(n)})
			continue
		}
		v := int(out[r.IntN(len(out))])
		if r.IntN(2) == 0 {
			u, v = v, u
		}
		edges = append(edges, [2]int{u, v})
	}
	return nodes, edges
}

func TestRestrictedViewMatchesRebuiltGraph(t *testing.T) {
	r := NewRand(3)
	large := 0
	for trial := range 600 {
		directed, weighted := trial%2 == 0, trial%3 != 0
		n := 2 + r.IntN(30)
		g := randomGraph(r, directed, weighted, n, r.IntN(4*n))
		nodes, edges := randomHidden(r, g)
		v := RestrictedView(g, nodes, edges)
		checkView(t, g, v, nodes, edges)
		if v.large != nil {
			large++
		}
	}
	// Both filters, the single word and the one sized to many touched nodes,
	// must have been through the check.
	if large == 0 || large == 600 {
		t.Errorf("%d of 600 views used the multi-word filter; the trials cover only one kind", large)
	}
}

func TestRestrictedViewConventions(t *testing.T) {
	// A triangle 0-1-2 with a tail 2-3.
	b := NewWeightedBuilder(4)
	b.AddEdgeW(0, 1, 1)
	b.AddEdgeW(1, 2, 2)
	b.AddEdgeW(0, 2, 3)
	b.AddEdgeW(2, 3, 4)
	g := b.Build()

	// An undirected edge goes as a whole, whichever way round it is named.
	edges := [][2]int{{1, 0}}
	v := RestrictedView(g, nil, edges)
	if got := v.OutNeighbors(0); !slices.Equal(got, []int32{2}) {
		t.Errorf("hiding {1, 0}: OutNeighbors(0) = %v, want [2]", got)
	}
	if got := v.OutNeighbors(1); !slices.Equal(got, []int32{2}) {
		t.Errorf("hiding {1, 0}: OutNeighbors(1) = %v, want [2]", got)
	}
	if got := v.OutWeights(1); !slices.Equal(got, []float64{2}) {
		t.Errorf("hiding {1, 0}: OutWeights(1) = %v, want [2]", got)
	}
	// The view keeps no reference to the caller's slice.
	edges[0] = [2]int{2, 3}
	if got := v.OutNeighbors(3); !slices.Equal(got, []int32{2}) {
		t.Errorf("changing the argument after the call changed the view: OutNeighbors(3) = %v", got)
	}

	// A hidden node keeps its ID and loses every edge.
	v = RestrictedView(g, []int{2}, nil)
	want := [][]int32{{1}, {0}, {}, {}}
	if v.NumNodes() != 4 {
		t.Errorf("hiding node 2: NumNodes = %d, want 4", v.NumNodes())
	}
	for u, w := range want {
		if got := v.OutNeighbors(u); !slices.Equal(got, w) {
			t.Errorf("hiding node 2: OutNeighbors(%d) = %v, want %v", u, got, w)
		}
	}
	if ws := v.OutWeights(2); ws == nil || len(ws) != 0 {
		t.Errorf("hiding node 2: OutWeights(2) = %#v, want empty and non-nil on a weighted graph", ws)
	}

	// On a Digraph only the named direction goes.
	d := NewDigraphBuilder(3)
	d.AddEdge(0, 1)
	d.AddEdge(1, 0)
	d.AddEdge(1, 2)
	dv := RestrictedView(d.Build(), nil, [][2]int{{0, 1}})
	if got := dv.OutNeighbors(0); len(got) != 0 {
		t.Errorf("hiding 0->1: OutNeighbors(0) = %v, want []", got)
	}
	if got := dv.OutNeighbors(1); !slices.Equal(got, []int32{0, 2}) {
		t.Errorf("hiding 0->1: OutNeighbors(1) = %v, want [0 2]", got)
	}
	if dv.OutWeights(1) != nil {
		t.Error("OutWeights is not nil on a view of an unweighted Digraph")
	}
}

func TestRestrictedViewTouchesOnlyChangedLists(t *testing.T) {
	g := randomGraph(NewRand(8), false, true, 200, 800)
	u := 17
	v := int(g.OutNeighbors(u)[0])
	if got := RestrictedView(g, nil, [][2]int{{u, v}}).touched; !slices.Equal(got, []int32{int32(min(u, v)), int32(max(u, v))}) {
		t.Errorf("hiding one edge touched %v, want its two ends", got)
	}
	// Pairs g does not have, and a node with no edges, change no list.
	var absent [][2]int
	for w := range g.NumNodes() {
		if w != u && !slices.Contains(g.OutNeighbors(u), int32(w)) {
			absent = append(absent, [2]int{u, w})
		}
	}
	lonely := NewBuilder(3)
	lonely.AddEdge(0, 1)
	if got := RestrictedView(g, nil, absent).touched; len(got) != 0 {
		t.Errorf("hiding absent edges touched %v", got)
	}
	if got := RestrictedView(lonely.Build(), []int{2}, nil).touched; len(got) != 0 {
		t.Errorf("hiding an isolated node touched %v", got)
	}
}

func TestRestrictedViewNegativeWeight(t *testing.T) {
	// 0-1 at -1 is the only negative edge; 1-2 at 5 is not.
	b := NewBuilder(3)
	b.AddEdgeW(0, 1, -1)
	b.AddEdgeW(1, 2, 5)
	g := b.Build()
	cases := []struct {
		nodes []int
		edges [][2]int
		want  bool
	}{
		{nil, nil, true},
		{nil, [][2]int{{1, 2}}, true},
		{nil, [][2]int{{1, 0}}, false},
		{[]int{0}, nil, false},
		{[]int{2}, nil, true},
	}
	for _, c := range cases {
		if got := RestrictedView(g, c.nodes, c.edges).HasNegativeWeight(); got != c.want {
			t.Errorf("hiding nodes %v, edges %v: HasNegativeWeight = %v, want %v", c.nodes, c.edges, got, c.want)
		}
	}

	// On a Digraph the reverse of a negative edge is a different edge.
	d := NewDigraphBuilder(2)
	d.AddEdgeW(0, 1, -1)
	d.AddEdgeW(1, 0, 2)
	dg := d.Build()
	if !RestrictedView(dg, nil, [][2]int{{1, 0}}).HasNegativeWeight() {
		t.Error("hiding 1->0 hid the negative 0->1")
	}
	if RestrictedView(dg, nil, [][2]int{{0, 1}}).HasNegativeWeight() {
		t.Error("hiding the negative 0->1 left a negative weight")
	}
}

func TestRestrictedViewOfView(t *testing.T) {
	r := NewRand(5)
	for trial := range 200 {
		g := randomGraph(r, trial%2 == 0, true, 2+r.IntN(20), 60)
		n1, e1 := randomHidden(r, g)
		n2, e2 := randomHidden(r, g)
		nested := RestrictedView(RestrictedView(g, n1, e1), n2, e2)
		if nested.g != g {
			t.Fatal("a view of a view is not built over the original graph")
		}
		checkView(t, g, nested, append(n1, n2...), append(e1, e2...))
	}
}

// lying embeds a Graph and overrides every Adjacency method with wrong
// answers, which a view of it must ignore throughout.
type lying struct{ *Graph }

func (lying) NumNodes() int            { return 0 }
func (lying) OutNeighbors(int) []int32 { return nil }
func (lying) OutWeights(int) []float64 { return nil }
func (lying) Weighted() bool           { return false }
func (lying) HasNegativeWeight() bool  { return false }

func TestRestrictedViewOfEmbeddedGraph(t *testing.T) {
	b := NewBuilder(4)
	b.AddEdgeW(0, 1, -1)
	b.AddEdgeW(1, 2, 2)
	b.AddEdgeW(2, 3, 3)
	g := b.Build()
	// Hiding {2, 1} changes the lists of 1 and 2 and leaves 0 and 3 alone, so
	// both kinds of node are covered.
	edges := [][2]int{{2, 1}}
	got := RestrictedView(lying{g}, nil, edges)
	if got.g != g {
		t.Fatal("the view is not of the embedded graph")
	}
	checkView(t, g, got, nil, edges)
}

// decorator embeds the interface rather than a concrete type, so it inherits
// none of the unexported methods a view reads storage through; a view of it
// must still find the graph or view inside.
type decorator struct{ Adjacency }

func TestRestrictedViewOfInterfaceWrapper(t *testing.T) {
	b := NewBuilder(4)
	b.AddEdgeW(0, 1, -1)
	b.AddEdgeW(1, 2, 2)
	b.AddEdgeW(2, 3, 3)
	g := b.Build()
	edges := [][2]int{{2, 1}}
	for name, w := range map[string]Adjacency{
		"graph":                decorator{g},
		"embedding type":       decorator{lying{g}},
		"wrapper of a wrapper": decorator{decorator{g}},
	} {
		got := RestrictedView(w, nil, edges)
		if got.g != g {
			t.Fatalf("%s: the view is not of the wrapped graph", name)
		}
		checkView(t, g, got, nil, edges)
	}
	// A wrapped view gives the union over the same graph, as a view does.
	got := RestrictedView(decorator{RestrictedView(g, []int{3}, nil)}, nil, edges)
	if got.g != g {
		t.Fatal("wrapped view: the result is not over the original graph")
	}
	checkView(t, g, got, []int{3}, edges)
}

// byValue embeds a Graph by value, so only a pointer to it is an Adjacency,
// and a nil one fails inside the promoted method.
type byValue struct{ Graph }

// TestRestrictedViewNilPanics pins that a missing graph, whether a nil
// interface, a nil pointer, to a graph or to a wrapper, or a wrapper around
// any of those, panics with a message naming RestrictedView and the types,
// rather than with a nil dereference.
func TestRestrictedViewNilPanics(t *testing.T) {
	for _, c := range []struct {
		g    Adjacency
		want string
	}{
		{nil, "gonx: RestrictedView: nil Adjacency"},
		{decorator{}, "gonx: RestrictedView: gonx.decorator wraps a nil Adjacency or a nil pointer"},
		{&decorator{}, "gonx: RestrictedView: *gonx.decorator wraps a nil Adjacency or a nil pointer"},
		{decorator{decorator{}}, "gonx: RestrictedView: gonx.decorator wraps a nil Adjacency or a nil pointer"},
		{(*decorator)(nil), "gonx: RestrictedView: nil *gonx.decorator"},
		{(*byValue)(nil), "gonx: RestrictedView: nil *gonx.byValue"},
		{decorator{(*byValue)(nil)}, "gonx: RestrictedView: gonx.decorator wraps a nil Adjacency or a nil pointer"},
		{(*Graph)(nil), "gonx: RestrictedView: nil *gonx.Graph"},
		{(*Digraph)(nil), "gonx: RestrictedView: nil *gonx.Digraph"},
		{(*Restricted)(nil), "gonx: RestrictedView: nil *gonx.Restricted"},
		{decorator{(*Graph)(nil)}, "gonx: RestrictedView: gonx.decorator wraps a nil *gonx.Graph"},
		{struct{ *Graph }{nil}, "gonx: RestrictedView: struct { *gonx.Graph } wraps a nil *gonx.Graph"},
		{decorator{(*Restricted)(nil)}, "gonx: RestrictedView: gonx.decorator wraps a nil *gonx.Restricted"},
	} {
		func() {
			defer func() {
				if got := fmt.Sprint(recover()); got != c.want {
					t.Errorf("RestrictedView(%#v) panics with %q, want %q", c.g, got, c.want)
				}
			}()
			RestrictedView(c.g, nil, nil)
		}()
	}
}

func TestRestrictedViewPanics(t *testing.T) {
	g := NewBuilder(3).Build()
	for name, f := range map[string]func(){
		"hidden node -1":      func() { RestrictedView(g, []int{-1}, nil) },
		"hidden node 3":       func() { RestrictedView(g, []int{3}, nil) },
		"edge end 3":          func() { RestrictedView(g, nil, [][2]int{{0, 3}}) },
		"edge end -1":         func() { RestrictedView(g, nil, [][2]int{{-1, 0}}) },
		"edge end 1<<32":      func() { RestrictedView(g, nil, [][2]int{{0, 1 << 32}}) },
		"OutNeighbors(3)":     func() { RestrictedView(g, nil, nil).OutNeighbors(3) },
		"OutWeights(-1)":      func() { RestrictedView(g, nil, nil).OutWeights(-1) },
		"OutNeighbors(1<<32)": func() { RestrictedView(g, []int{0}, nil).OutNeighbors(1 << 32) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			f()
		}()
	}
}

// TestRestrictedViewAllocations pins that reading a view allocates nothing,
// whether the list is g's own or the view's.
func TestRestrictedViewAllocations(t *testing.T) {
	g := randomGraph(NewRand(2), false, true, 100, 400)
	v := RestrictedView(g, []int{5}, [][2]int{{7, int(g.OutNeighbors(7)[0])}})
	allocs := testing.AllocsPerRun(100, func() {
		for u := range v.NumNodes() {
			_ = v.OutNeighbors(u)
			_ = v.OutWeights(u)
		}
	})
	if allocs != 0 {
		t.Errorf("reading every list allocates %v times, want 0", allocs)
	}
}

// FuzzRestrictedView reads a graph and what to hide from the fuzzer's bytes:
// the first byte picks the kind and whether it is weighted, then each triple
// is an edge (u, v, weight), a hidden edge, or a hidden node, by the top bits
// of its first byte.
func FuzzRestrictedView(f *testing.F) {
	f.Add([]byte{0, 1, 2, 30, 2, 3, 40, 0x41, 2, 0, 0x80, 1, 0})
	f.Add([]byte{3, 0, 1, 0, 1, 0, 0, 0x40, 0, 1, 0x40, 1, 0, 0xc0, 3, 3})
	f.Fuzz(func(t *testing.T, script []byte) {
		if len(script) == 0 || len(script) > 3000 {
			return
		}
		const n = 12
		directed, weighted := script[0]&1 == 1, script[0]&2 == 0
		ub, db := NewBuilder(n), NewDigraphBuilder(n)
		var nodes []int
		var edges [][2]int
		for i := 1; i+2 < len(script); i += 3 {
			op, u, v := script[i]>>6, int(script[i]&0x3f)%n, int(script[i+1])%n
			w := float64(int8(script[i+2])) / 8
			switch {
			case op == 0 || op == 3:
				switch {
				case directed && weighted:
					db.AddEdgeW(u, v, w)
				case directed:
					db.AddEdge(u, v)
				case weighted:
					ub.AddEdgeW(u, v, w)
				default:
					ub.AddEdge(u, v)
				}
			case op == 1:
				edges = append(edges, [2]int{u, v})
			default:
				nodes = append(nodes, u)
			}
		}
		var g Adjacency = ub.Build()
		if directed {
			g = db.Build()
		}
		checkView(t, g, RestrictedView(g, nodes, edges), nodes, edges)
	})
}

// BenchmarkRestrictedViewNegative builds a view that hides one edge of a
// 100k-node path, with and without a negative weight between its last nodes,
// so that a cost which grows with the whole graph rather than with the lists
// the view changes, such as a scan for a surviving negative weight, shows up
// as a gap between the two.
func BenchmarkRestrictedViewNegative(b *testing.B) {
	for _, negative := range []bool{false, true} {
		const n = 100_000
		g := NewWeightedBuilder(n)
		for u := range n - 1 {
			g.AddEdgeW(u, u+1, 1)
		}
		if negative {
			g.AddEdgeW(n-3, n-1, -1)
		}
		graph := g.Build()
		b.Run(map[bool]string{false: "nonnegative", true: "negative"}[negative], func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				_ = RestrictedView(graph, nil, [][2]int{{10, 11}})
			}
		})
	}
}
