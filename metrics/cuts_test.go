package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/generators"
)

func buildGraph(n int, edges [][2]int) *gonx.Graph {
	b := gonx.NewBuilder(n)
	for _, e := range edges {
		b.AddEdge(e[0], e[1])
	}
	return b.Build()
}

// bruteCuts is the oracle: it deletes each edge, and then each node's edges,
// and counts components. An edge is a bridge when deleting it adds a
// component; its V is whichever endpoint then no longer shares a component
// with the smallest node of the original one. A node is a cut node when
// isolating it leaves more than one extra component: one is the node itself.
func bruteCuts(g *gonx.Graph) ([]Bridge, []int) {
	n := g.NumNodes()
	before := len(ConnectedComponents(g))
	compOf := func(h *gonx.Graph) ([]int, [][]int) {
		comps := ConnectedComponents(h)
		id := make([]int, n)
		for i, c := range comps {
			for _, v := range c {
				id[v] = i
			}
		}
		return id, comps
	}
	origID, origComps := compOf(g)

	var bridges []Bridge
	for u, v := range g.Edges() {
		b := g.ToBuilder()
		b.RemoveEdge(u, v)
		h := b.Build()
		id, comps := compOf(h)
		if len(comps) == before {
			continue
		}
		smallest := slices.Min(origComps[origID[u]])
		if id[v] == id[smallest] {
			u, v = v, u
		}
		bridges = append(bridges, Bridge{U: u, V: v, Side: len(comps[id[v]])})
	}
	slices.SortFunc(bridges, func(a, b Bridge) int { return a.V - b.V })

	var points []int
	for x := range n {
		b := g.ToBuilder()
		for _, y := range g.Neighbors(x) {
			b.RemoveEdge(x, int(y))
		}
		if len(ConnectedComponents(b.Build()))-1 > before {
			points = append(points, x)
		}
	}
	return bridges, points
}

// sameBridges compares U, V and Side, and expects SideWeight to equal Side,
// which is what Bridges reports when it is given no node weights.
func sameBridges(t *testing.T, label string, got, want []Bridge) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d bridges, want %d\ngot  %v\nwant %v", label, len(got), len(want), got, want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.U != w.U || g.V != w.V || g.Side != w.Side || g.SideWeight != float64(w.Side) {
			t.Errorf("%s: bridge %d = %+v, want {U:%d V:%d Side:%d SideWeight:%d}", label, i, g, w.U, w.V, w.Side, w.Side)
		}
	}
}

func TestCutsHandWorked(t *testing.T) {
	cases := []struct {
		name    string
		n       int
		edges   [][2]int
		bridges []Bridge // SideWeight left zero; sameBridges expects Side
		points  []int
	}{
		{name: "empty", n: 0},
		{name: "isolated nodes", n: 3},
		{name: "one edge", n: 2, edges: [][2]int{{0, 1}}, bridges: []Bridge{{U: 0, V: 1, Side: 1}}},
		{
			// 0-1-2-3: every edge is a bridge and the inner nodes are cut
			// nodes; each V is the far end from node 0.
			name: "path", n: 4, edges: [][2]int{{0, 1}, {1, 2}, {2, 3}},
			bridges: []Bridge{{U: 0, V: 1, Side: 3}, {U: 1, V: 2, Side: 2}, {U: 2, V: 3, Side: 1}},
			points:  []int{1, 2},
		},
		{name: "cycle", n: 4, edges: [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 0}}},
		{
			// The root of the search is a cut node here, and only through the
			// rule for roots: it has three tree children.
			name: "star", n: 4, edges: [][2]int{{0, 1}, {0, 2}, {0, 3}},
			bridges: []Bridge{{U: 0, V: 1, Side: 1}, {U: 0, V: 2, Side: 1}, {U: 0, V: 3, Side: 1}},
			points:  []int{0},
		},
		{
			// Triangles 0-1-2 and 3-4-5 joined by 2-3, a pendant 6 on 5, and
			// a separate edge 7-8 whose smallest node is 7.
			name: "barbell", n: 9,
			edges:   [][2]int{{0, 1}, {1, 2}, {0, 2}, {2, 3}, {3, 4}, {4, 5}, {3, 5}, {5, 6}, {7, 8}},
			bridges: []Bridge{{U: 2, V: 3, Side: 4}, {U: 5, V: 6, Side: 1}, {U: 7, V: 8, Side: 1}},
			points:  []int{2, 3, 5},
		},
		{
			// The side that detaches is the big one: node 0 hangs off 4, but
			// it is the component's smallest node, so V is 4 and the side is
			// the cycle 1-2-3-4, not the pendant.
			name: "low pendant", n: 5, edges: [][2]int{{0, 4}, {1, 2}, {2, 3}, {3, 4}, {4, 1}},
			bridges: []Bridge{{U: 0, V: 4, Side: 4}},
			points:  []int{4},
		},
		{
			// A bridge whose V is smaller than its U: in 0-3-1, cutting 3-1
			// detaches 1, which is below 3.
			name: "V below U", n: 4, edges: [][2]int{{0, 3}, {3, 1}},
			bridges: []Bridge{{U: 3, V: 1, Side: 1}, {U: 0, V: 3, Side: 2}},
			points:  []int{3},
		},
	}
	for _, c := range cases {
		g := buildGraph(c.n, c.edges)
		sameBridges(t, c.name, Bridges(g, nil), c.bridges)
		if got := ArticulationPoints(g); !slices.Equal(got, c.points) {
			t.Errorf("%s: ArticulationPoints = %v, want %v", c.name, got, c.points)
		}
	}
}

func TestCutsMatchNetworkx(t *testing.T) {
	for _, name := range []string{"ws_undirected", "er_sparse"} {
		g := loadEdges(t, name+".edges").(*gonx.Graph)
		raw, err := os.ReadFile(filepath.Join("testdata", name+".cuts.json"))
		if err != nil {
			t.Fatal(err)
		}
		var want struct {
			Points  []int    `json:"articulation_points"`
			Bridges [][3]int `json:"bridges"`
		}
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatal(err)
		}
		wb := make([]Bridge, len(want.Bridges))
		for i, b := range want.Bridges {
			wb[i] = Bridge{U: b[0], V: b[1], Side: b[2]}
		}
		sameBridges(t, name, Bridges(g, nil), wb)
		if got := ArticulationPoints(g); !slices.Equal(got, want.Points) {
			t.Errorf("%s: ArticulationPoints = %v, want %v", name, got, want.Points)
		}
	}
}

func TestCutsAgainstBruteForce(t *testing.T) {
	// Average degrees from about 1 to 4 cover forests, the threshold where a
	// giant component appears, and graphs where bridges are rare.
	r := gonx.NewRand(21)
	for _, n := range []int{1, 2, 5, 12, 40, 120, 300} {
		for _, deg := range []float64{1, 1.8, 2.5, 4} {
			p := min(1, deg/float64(max(n-1, 1)))
			g, err := generators.ErdosRenyi(n, p, r)
			if err != nil {
				t.Fatal(err)
			}
			wantB, wantP := bruteCuts(g)
			sameBridges(t, "random graph", Bridges(g, nil), wantB)
			if got := ArticulationPoints(g); !slices.Equal(got, wantP) {
				t.Errorf("n=%d deg=%v: ArticulationPoints = %v, want %v", n, deg, got, wantP)
			}
		}
	}
}

func TestBridgesNodeWeight(t *testing.T) {
	// The barbell from the hand-worked cases, with node weights standing for
	// populations: cutting 2-3 strands 3, 4, 5 and 6, cutting 5-6 strands 6.
	g := buildGraph(7, [][2]int{{0, 1}, {1, 2}, {0, 2}, {2, 3}, {3, 4}, {4, 5}, {3, 5}, {5, 6}})
	w := []float64{10, 20, 30, 1, 2, 4, 0.5}
	got := Bridges(g, w)
	want := []Bridge{{U: 2, V: 3, Side: 4, SideWeight: 7.5}, {U: 5, V: 6, Side: 1, SideWeight: 0.5}}
	if !slices.Equal(got, want) {
		t.Errorf("Bridges with weights = %+v, want %+v", got, want)
	}
	defer func() {
		if recover() == nil {
			t.Error("no panic for a nodeWeight of the wrong length")
		}
	}()
	Bridges(g, w[:6])
}

func TestCutsDeepPath(t *testing.T) {
	// A path is the worst case for the search depth. A recursive version
	// would need a frame per node here; the explicit stack needs 4 bytes per node.
	const n = 300_000
	b := gonx.NewBuilder(n)
	for i := 1; i < n; i++ {
		b.AddEdgeUnchecked(i-1, i)
	}
	g := b.Build()
	bridges := Bridges(g, nil)
	if len(bridges) != n-1 {
		t.Fatalf("%d bridges, want %d", len(bridges), n-1)
	}
	for i, br := range bridges {
		if br.U != i || br.V != i+1 || br.Side != n-1-i {
			t.Fatalf("bridge %d = %+v, want {U:%d V:%d Side:%d}", i, br, i, i+1, n-1-i)
		}
	}
	if pts := ArticulationPoints(g); len(pts) != n-2 || pts[0] != 1 || pts[len(pts)-1] != n-2 {
		t.Fatalf("ArticulationPoints: %d nodes from %v to %v, want %d from 1 to %d", len(pts), pts[0], pts[len(pts)-1], n-2, n-2)
	}
}

// FuzzCuts decodes a byte string into a graph on at most 12 nodes, one edge
// per byte pair, and checks both functions against the brute-force oracle.
func FuzzCuts(f *testing.F) {
	f.Add([]byte{7, 0, 1, 1, 2, 2, 0, 2, 3})
	f.Add([]byte{12, 0, 1, 1, 2, 2, 3, 3, 4, 5, 6, 6, 7, 7, 5, 4, 5, 9, 10})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}
		n := int(data[0])%12 + 1
		b := gonx.NewBuilder(n)
		for i := 1; i+1 < len(data) && i < 80; i += 2 {
			b.AddEdge(int(data[i])%n, int(data[i+1])%n)
		}
		g := b.Build()
		wantB, wantP := bruteCuts(g)
		sameBridges(t, "fuzzed graph", Bridges(g, nil), wantB)
		if got := ArticulationPoints(g); !slices.Equal(got, wantP) {
			t.Errorf("ArticulationPoints = %v, want %v on input %v", got, wantP, data)
		}
	})
}

func BenchmarkBridges_100000(b *testing.B) {
	g, err := generators.RandomAvgDegree(100_000, 2.5, gonx.NewRand(1))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = Bridges(g, nil)
	}
}
