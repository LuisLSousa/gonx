package metrics

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/LuisLSousa/gonx"
)

func TestRestrictedViewMatchesNetworkx(t *testing.T) {
	for _, name := range []string{"ws_undirected", "er_directed", "er_sparse", "ba_tree"} {
		t.Run(name, func(t *testing.T) {
			g := loadEdges(t, name+".edges")
			raw, err := os.ReadFile(filepath.Join("testdata", name+".restricted.json"))
			if err != nil {
				t.Fatal(err)
			}
			var hidden struct {
				Nodes []int    `json:"nodes"`
				Edges [][2]int `json:"edges"`
			}
			if err := json.Unmarshal(raw, &hidden); err != nil {
				t.Fatal(err)
			}
			view := gonx.RestrictedView(g, hidden.Nodes, hidden.Edges)
			want := loadExpected(t, name+".restricted.json")
			// The same search with only the nodes hidden, to show that the
			// hidden edges change something too.
			nodesOnly := gonx.RestrictedView(g, hidden.Nodes, nil)
			n := g.NumNodes()
			dist := make([]float64, n)
			prev := make([]int32, n)
			byNodes := make([]float64, n)
			fromEdges := 0
			for src, wantDist := range want {
				poison(dist, prev)
				if err := Dijkstra(view, src, dist, prev); err != nil {
					t.Fatalf("source %d: %v", src, err)
				}
				if err := Dijkstra(nodesOnly, src, byNodes, nil); err != nil {
					t.Fatal(err)
				}
				for v := range n {
					switch {
					case wantDist[v] < 0 && !math.IsInf(dist[v], 1):
						t.Errorf("source %d: node %d is unreachable in networkx, got %v", src, v, dist[v])
					case wantDist[v] >= 0 && math.Abs(dist[v]-wantDist[v]) > 1e-9:
						t.Errorf("source %d: dist[%d] = %v, networkx says %v", src, v, dist[v], wantDist[v])
					}
					if (wantDist[v] < 0) != math.IsInf(byNodes[v], 1) || (wantDist[v] >= 0 && math.Abs(byNodes[v]-wantDist[v]) > 1e-9) {
						fromEdges++
					}
				}
				checkTree(t, view, src, dist, prev)
			}
			// A fixture whose hidden edges change no distance would pass
			// against a view that ignores edges, since hiding the nodes
			// changes distances on its own.
			if fromEdges == 0 {
				t.Fatal("the hidden edges change no distance; regenerate the fixture")
			}
		})
	}
}

// withoutEdge rebuilds g without the edge from u to v, and on an undirected
// graph without the edge between them, through the Builder's own RemoveEdge.
func withoutEdge(g gonx.Adjacency, u, v int) gonx.Adjacency {
	switch g := g.(type) {
	case *gonx.Graph:
		b := g.ToBuilder()
		b.RemoveEdge(u, v)
		return b.Build()
	case *gonx.Digraph:
		b := g.ToBuilder()
		b.RemoveEdge(u, v)
		return b.Build()
	}
	panic("unreachable")
}

// TestShortestPathAroundAnEdge is the replacement-path question the view
// exists for: the shortest way from u to v when the edge between them is gone.
// It must match a graph rebuilt without the edge, and never use the edge.
func TestShortestPathAroundAnEdge(t *testing.T) {
	for _, name := range []string{"ws_undirected", "er_directed", "er_sparse", "ba_tree"} {
		g := loadEdges(t, name+".edges")
		for u := 0; u < g.NumNodes(); u += 5 {
			for _, v32 := range g.OutNeighbors(u) {
				v := int(v32)
				path, length, err := ShortestPath(gonx.RestrictedView(g, nil, [][2]int{{u, v}}), u, v)
				if err != nil {
					t.Fatal(err)
				}
				wantPath, wantLength, _ := ShortestPath(withoutEdge(g, u, v), u, v)
				if (path == nil) != (wantPath == nil) || (path != nil && math.Abs(length-wantLength) > 1e-9) {
					t.Fatalf("%s: around %d-%d: %v at %v; rebuilt graph says %v at %v", name, u, v, path, length, wantPath, wantLength)
				}
				if len(path) == 2 {
					t.Fatalf("%s: the way around %d-%d is the edge itself", name, u, v)
				}
			}
		}
	}
}

func TestBreadthFirstOnRestrictedView(t *testing.T) {
	g := weightedScaleFree(t)
	// Hide the hub and one of the edges of the node after it.
	u := 1
	v := int(g.Neighbors(u)[len(g.Neighbors(u))-1])
	view := gonx.RestrictedView(g, []int{0}, [][2]int{{u, v}})
	b := g.ToBuilder()
	for _, x := range g.Neighbors(0) {
		b.RemoveEdge(0, int(x))
	}
	b.RemoveEdge(u, v)
	want := b.Build()
	got, wantHops := make([]int32, g.NumNodes()), make([]int32, g.NumNodes())
	for _, src := range []int{1, 2, 500, 9999} {
		BreadthFirst(view, src, got)
		BreadthFirst(want, src, wantHops)
		if !slices.Equal(got, wantHops) {
			t.Errorf("source %d: BreadthFirst on the view differs from the rebuilt graph", src)
		}
	}
	BreadthFirst(view, 0, got)
	if got[0] != 0 || slices.ContainsFunc(got[1:], func(d int32) bool { return d != -1 }) {
		t.Error("a search from a hidden node reached another node")
	}
}

func TestDijkstraOnRestrictedViewNegativeWeight(t *testing.T) {
	b := gonx.NewDigraphBuilder(3)
	b.AddEdgeW(0, 1, 2)
	b.AddEdgeW(1, 2, -1)
	b.AddEdgeW(0, 2, 5)
	g := b.Build()
	dist := make([]float64, 3)
	if err := Dijkstra(gonx.RestrictedView(g, nil, [][2]int{{0, 2}}), 0, dist, nil); !errors.Is(err, ErrNegativeWeight) {
		t.Errorf("negative edge still visible: err %v, want ErrNegativeWeight", err)
	}
	if err := Dijkstra(gonx.RestrictedView(g, nil, [][2]int{{1, 2}}), 0, dist, nil); err != nil {
		t.Errorf("negative edge hidden: %v", err)
	}
	if want := []float64{0, 2, 5}; !slices.Equal(dist, want) {
		t.Errorf("negative edge hidden: dist %v, want %v", dist, want)
	}
}

// TestRestrictedViewConcurrentSearches runs many searches on one view at once;
// under -race it checks that reading a view writes nothing.
func TestRestrictedViewConcurrentSearches(t *testing.T) {
	g := weightedScaleFree(t)
	view := gonx.RestrictedView(g, []int{3}, [][2]int{{0, int(g.Neighbors(0)[0])}})
	want := make([]float64, 64)
	for i := range want {
		_, want[i], _ = ShortestPath(view, i, 9999-i)
	}
	var wg sync.WaitGroup
	for i := range want {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, got, _ := ShortestPath(view, i, 9999-i); got != want[i] {
				t.Errorf("ShortestPath(%d, %d) = %v concurrently, %v alone", i, 9999-i, got, want[i])
			}
		}()
	}
	wg.Wait()
}

// TestShortestPathRestrictedAllocations pins that a view adds nothing per node
// visited: ShortestPath allocates the same on the view as on the graph.
func TestShortestPathRestrictedAllocations(t *testing.T) {
	g := weightedScaleFree(t)
	view := gonx.RestrictedView(g, nil, [][2]int{{5000, int(g.Neighbors(5000)[0])}})
	count := func(a gonx.Adjacency) float64 {
		return testing.AllocsPerRun(10, func() {
			if _, _, err := ShortestPath(a, 0, 9999); err != nil {
				t.Fatal(err)
			}
		})
	}
	if plain, restricted := count(g), count(view); restricted != plain {
		t.Errorf("ShortestPath allocates %v times on the view, %v on the graph", restricted, plain)
	}
}

// BenchmarkDijkstraRestricted_10000 is BenchmarkDijkstra_10000 on views of the
// same graph: one that hides nothing, which measures the cost of going through
// the view, and one that hides an edge, which adds two lists the view owns.
// Hiding the hub (node 0, the oldest node of the preferential attachment)
// changes hundreds of its neighbours' lists, the case where most lookups need
// more than the filter; "hub-rebuilt" runs the same search on a graph built
// without the hub's edges, the floor for what the view costs.
func BenchmarkDijkstraRestricted_10000(b *testing.B) {
	g := weightedScaleFree(b)
	dist := make([]float64, g.NumNodes())
	prev := make([]int32, g.NumNodes())
	rebuilt := g.ToBuilder()
	for _, v := range g.Neighbors(0) {
		rebuilt.RemoveEdge(0, int(v))
	}
	for _, c := range []struct {
		name string
		g    gonx.Adjacency
		src  int
	}{
		{"none", gonx.RestrictedView(g, nil, nil), 0},
		{"edge", gonx.RestrictedView(g, nil, [][2]int{{5000, int(g.Neighbors(5000)[0])}}), 0},
		{"hub", gonx.RestrictedView(g, []int{0}, nil), 1},
		{"hub-rebuilt", rebuilt.Build(), 1},
	} {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if err := Dijkstra(c.g, c.src, dist, prev); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkShortestPathAround_10000 is one replacement-path query as a caller
// would run it: build a view without an edge, then search between its ends.
func BenchmarkShortestPathAround_10000(b *testing.B) {
	g := weightedScaleFree(b)
	u, v := 5000, int(g.Neighbors(5000)[0])
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, _, err := ShortestPath(gonx.RestrictedView(g, nil, [][2]int{{u, v}}), u, v); err != nil {
			b.Fatal(err)
		}
	}
}
