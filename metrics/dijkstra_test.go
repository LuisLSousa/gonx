package metrics

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/generators"
)

// loadEdges parses a testdata edge list ("undirected N" or "directed N" on the
// first line, then "u v w" per edge) into the graph kind it names.
func loadEdges(t *testing.T, name string) gonx.Adjacency {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		t.Fatalf("%s: empty", name)
	}
	var kind string
	var n int
	if _, err := fmt.Sscan(sc.Text(), &kind, &n); err != nil {
		t.Fatalf("%s: bad header %q", name, sc.Text())
	}
	var ub *gonx.Builder
	var db *gonx.DigraphBuilder
	switch kind {
	case "undirected":
		ub = gonx.NewBuilder(n)
	case "directed":
		db = gonx.NewDigraphBuilder(n)
	default:
		t.Fatalf("%s: unknown kind %q", name, kind)
	}
	for sc.Scan() {
		var u, v int
		var w float64
		if _, err := fmt.Sscan(sc.Text(), &u, &v, &w); err != nil {
			t.Fatalf("%s: bad line %q", name, sc.Text())
		}
		ok := false
		if ub != nil {
			ok = ub.AddEdgeW(u, v, w)
		} else {
			ok = db.AddEdgeW(u, v, w)
		}
		if !ok {
			t.Fatalf("%s: edge %d %d rejected", name, u, v)
		}
	}
	if ub != nil {
		return ub.Build()
	}
	return db.Build()
}

// loadExpected reads the networkx answers written by testdata/gen_expected.py:
// per source, a dense distance list with -1 for unreachable nodes.
func loadExpected(t *testing.T, name string) map[int][]float64 {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Sources map[string][]float64 `json:"sources"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	out := make(map[int][]float64, len(file.Sources))
	for k, v := range file.Sources {
		src, err := strconv.Atoi(k)
		if err != nil {
			t.Fatal(err)
		}
		out[src] = v
	}
	return out
}

// edgeWeight returns the weight of the forward edge u->v in g, or false when
// there is none. Unweighted graphs report 1.
func edgeWeight(g gonx.Adjacency, u, v int) (float64, bool) {
	ws := g.OutWeights(u)
	for i, x := range g.OutNeighbors(u) {
		if int(x) == v {
			if ws == nil {
				return 1, true
			}
			return ws[i], true
		}
	}
	return 0, false
}

// poison fills dist and prev with garbage, so a test can tell an implementation
// that writes every entry from one that relies on the caller's zero values.
func poison(dist []float64, prev []int32) {
	for i := range dist {
		dist[i] = math.NaN()
	}
	for i := range prev {
		prev[i] = 99
	}
}

// checkTree verifies the contract that binds dist and prev together: src has
// distance 0 and no predecessor, every unreachable node is +Inf with no
// predecessor, every other node's predecessor is a real edge whose weight
// closes the distance exactly, and following prev from any reachable node
// arrives at src within n steps (so zero-weight cycles cannot hide in it).
func checkTree(t *testing.T, g gonx.Adjacency, src int, dist []float64, prev []int32) {
	t.Helper()
	n := len(dist)
	if dist[src] != 0 || prev[src] != -1 {
		t.Errorf("source: dist %v prev %d, want 0 and -1", dist[src], prev[src])
	}
	for v := range n {
		if v == src {
			continue
		}
		if math.IsInf(dist[v], 1) {
			if prev[v] != -1 {
				t.Errorf("unreachable %d has prev %d", v, prev[v])
			}
			continue
		}
		p := int(prev[v])
		if p < 0 || p >= n {
			t.Errorf("node %d: prev %d out of range", v, p)
			continue
		}
		w, ok := edgeWeight(g, p, v)
		if !ok {
			t.Errorf("node %d: prev %d is not a predecessor", v, p)
			continue
		}
		if math.Abs(dist[p]+w-dist[v]) > 1e-9 {
			t.Errorf("node %d: dist %v, but prev %d has dist %v and edge weight %v", v, dist[v], p, dist[p], w)
		}
		steps := 0
		for x := v; x != src; x = int(prev[x]) {
			if steps++; steps > n {
				t.Errorf("node %d: prev chain does not reach the source within %d steps", v, n)
				break
			}
		}
	}
}

func TestDijkstraMatchesNetworkx(t *testing.T) {
	for _, name := range []string{"ws_undirected", "er_directed"} {
		t.Run(name, func(t *testing.T) {
			g := loadEdges(t, name+".edges")
			want := loadExpected(t, name+".dijkstra.json")
			n := g.NumNodes()
			dist := make([]float64, n)
			prev := make([]int32, n)
			for src, wantDist := range want {
				poison(dist, prev)
				if err := Dijkstra(g, src, dist, prev); err != nil {
					t.Fatalf("source %d: %v", src, err)
				}
				for v := range n {
					switch {
					case wantDist[v] < 0 && !math.IsInf(dist[v], 1):
						t.Errorf("source %d: node %d is unreachable in networkx, got %v", src, v, dist[v])
					case wantDist[v] >= 0 && math.Abs(dist[v]-wantDist[v]) > 1e-9:
						t.Errorf("source %d: dist[%d] = %v, networkx says %v", src, v, dist[v], wantDist[v])
					}
				}
				checkTree(t, g, src, dist, prev)
			}
		})
	}
}

func TestDijkstraHandWorked(t *testing.T) {
	// A path 0-1-2-3 of unit weights and a direct 0-3 edge of weight 3.5: the
	// three-hop path wins, and the direct edge's tentative distance for node 3
	// has to be improved after it was first seen. Node 4 is isolated.
	b := gonx.NewBuilder(5)
	b.AddEdgeW(0, 1, 1)
	b.AddEdgeW(1, 2, 1)
	b.AddEdgeW(2, 3, 1)
	b.AddEdgeW(0, 3, 3.5)
	g := b.Build()
	dist := make([]float64, 5)
	prev := make([]int32, 5)
	poison(dist, prev)
	if err := Dijkstra(g, 0, dist, prev); err != nil {
		t.Fatal(err)
	}
	want := []float64{0, 1, 2, 3, math.Inf(1)}
	for v, w := range want {
		if dist[v] != w {
			t.Errorf("dist[%d] = %v, want %v", v, dist[v], w)
		}
	}
	if prev[3] != 2 || prev[4] != -1 || prev[0] != -1 {
		t.Errorf("prev = %v, want prev[3] = 2, prev[4] = -1, prev[0] = -1", prev)
	}
	checkTree(t, g, 0, dist, prev)

	// Zero-weight edges: 0-1 and 1-2 weigh 0, 0-2 weighs 0.5. Node 2 is at
	// distance 0 through 1, and the tree must still lead back to 0 rather than
	// loop between the two zero-distance nodes.
	z := gonx.NewBuilder(3)
	z.AddEdgeW(0, 1, 0)
	z.AddEdgeW(1, 2, 0)
	z.AddEdgeW(0, 2, 0.5)
	zg := z.Build()
	dist, prev = dist[:3], prev[:3]
	poison(dist, prev)
	if err := Dijkstra(zg, 0, dist, prev); err != nil {
		t.Fatal(err)
	}
	if dist[1] != 0 || dist[2] != 0 {
		t.Errorf("zero-weight path: dist = %v, want [0 0 0]", dist)
	}
	checkTree(t, zg, 0, dist, prev)

	// Directed: a cycle 0->1->2->0 with weights 1, 1, 10. From 2, node 0 costs
	// 10 and node 1 costs 11; from 1, node 0 costs 11 via 2.
	d := gonx.NewDigraphBuilder(3)
	d.AddEdgeW(0, 1, 1)
	d.AddEdgeW(1, 2, 1)
	d.AddEdgeW(2, 0, 10)
	dg := d.Build()
	if err := Dijkstra(dg, 2, dist, nil); err != nil {
		t.Fatal(err)
	}
	if dist[0] != 10 || dist[1] != 11 || dist[2] != 0 {
		t.Errorf("from 2: dist = %v, want [10 11 0]", dist)
	}
	if err := Dijkstra(dg, 1, dist, nil); err != nil {
		t.Fatal(err)
	}
	if dist[0] != 11 || dist[1] != 0 || dist[2] != 1 {
		t.Errorf("from 1: dist = %v, want [11 0 1]", dist)
	}
}

func TestDijkstraUnweightedEqualsBreadthFirst(t *testing.T) {
	ug, err := generators.WattsStrogatz(200, 6, 0.2, gonx.NewRand(4))
	if err != nil {
		t.Fatal(err)
	}
	r := gonx.NewRand(5)
	db := gonx.NewDigraphBuilder(150)
	for db.NumEdges() < 400 {
		db.AddEdge(r.IntN(150), r.IntN(150))
	}
	for name, g := range map[string]gonx.Adjacency{"undirected": ug, "directed": db.Build()} {
		n := g.NumNodes()
		dist := make([]float64, n)
		hops := make([]int32, n)
		for _, src := range []int{0, 3, n - 1} {
			poison(dist, nil)
			if err := Dijkstra(g, src, dist, nil); err != nil {
				t.Fatal(err)
			}
			BreadthFirst(g, src, hops)
			for v := range n {
				switch {
				case hops[v] == -1 && !math.IsInf(dist[v], 1):
					t.Errorf("%s source %d: BreadthFirst cannot reach %d, Dijkstra says %v", name, src, v, dist[v])
				case hops[v] >= 0 && dist[v] != float64(hops[v]):
					t.Errorf("%s source %d: dist[%d] = %v, BreadthFirst says %d", name, src, v, dist[v], hops[v])
				}
			}
		}
	}
}

func TestDijkstraNegativeWeight(t *testing.T) {
	// The rule is about the graph, not the search: every source fails,
	// including 2, which reaches nothing, and 0, from which the negative edge
	// is unreachable. dist and prev must come back untouched.
	b := gonx.NewDigraphBuilder(4)
	b.AddEdgeW(0, 1, 1)
	b.AddEdgeW(2, 0, -3)
	b.AddEdgeW(3, 0, 1)
	g := b.Build()
	dist := make([]float64, 4)
	prev := make([]int32, 4)
	for src := range 4 {
		poison(dist, prev)
		if err := Dijkstra(g, src, dist, prev); !errors.Is(err, ErrNegativeWeight) {
			t.Errorf("from %d: err = %v, want ErrNegativeWeight", src, err)
		}
		for v := range dist {
			if !math.IsNaN(dist[v]) || prev[v] != 99 {
				t.Fatalf("from %d: dist %v prev %v were written to", src, dist, prev)
			}
		}
	}
	u := gonx.NewBuilder(3)
	u.AddEdgeW(0, 1, 2)
	u.AddEdgeW(1, 2, -1)
	if err := Dijkstra(u.Build(), 0, dist[:3], nil); !errors.Is(err, ErrNegativeWeight) {
		t.Errorf("undirected negative edge: err = %v, want ErrNegativeWeight", err)
	}
}

func TestDijkstraOverflowReadsAsUnreachable(t *testing.T) {
	// Two edges of the largest finite weight: node 1 is reached at MaxFloat64,
	// and the path to node 2 overflows to +Inf, which the contract reports as
	// unreachable rather than as a node with a predecessor and no distance.
	b := gonx.NewDigraphBuilder(3)
	b.AddEdgeW(0, 1, math.MaxFloat64)
	b.AddEdgeW(1, 2, math.MaxFloat64)
	g := b.Build()
	dist := make([]float64, 3)
	prev := make([]int32, 3)
	poison(dist, prev)
	if err := Dijkstra(g, 0, dist, prev); err != nil {
		t.Fatal(err)
	}
	if dist[1] != math.MaxFloat64 || prev[1] != 0 {
		t.Errorf("node 1: dist %v prev %d, want MaxFloat64 and 0", dist[1], prev[1])
	}
	if !math.IsInf(dist[2], 1) || prev[2] != -1 {
		t.Errorf("node 2: dist %v prev %d, want +Inf and -1", dist[2], prev[2])
	}
}

// bellmanFord is the oracle: slower, simpler, and correct for any weights.
func bellmanFord(g gonx.Adjacency, src int) []float64 {
	n := g.NumNodes()
	dist := make([]float64, n)
	for i := range dist {
		dist[i] = math.Inf(1)
	}
	dist[src] = 0
	for range n {
		changed := false
		for u := range n {
			if math.IsInf(dist[u], 1) {
				continue
			}
			ws := g.OutWeights(u)
			for i, v := range g.OutNeighbors(u) {
				w := 1.0
				if ws != nil {
					w = ws[i]
				}
				if d := dist[u] + w; d < dist[v] {
					dist[v] = d
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}
	return dist
}

func TestDijkstraAgainstBellmanFord(t *testing.T) {
	r := gonx.NewRand(8)
	// Weights from a coarse grid so that ties and zero-weight edges occur.
	weight := func() float64 { return float64(r.IntN(8)) / 2 }

	topo, err := generators.BarabasiAlbert(300, 2, r)
	if err != nil {
		t.Fatal(err)
	}
	ub := gonx.NewBuilder(topo.NumNodes())
	for u, v := range topo.Edges() {
		ub.AddEdgeW(u, v, weight())
	}
	db := gonx.NewDigraphBuilder(250)
	for db.NumEdges() < 900 {
		db.AddEdgeW(r.IntN(250), r.IntN(250), weight())
	}

	for name, g := range map[string]gonx.Adjacency{"undirected": ub.Build(), "directed": db.Build()} {
		n := g.NumNodes()
		dist := make([]float64, n)
		prev := make([]int32, n)
		for _, src := range []int{0, 17, n - 1} {
			poison(dist, prev)
			if err := Dijkstra(g, src, dist, prev); err != nil {
				t.Fatalf("%s source %d: %v", name, src, err)
			}
			want := bellmanFord(g, src)
			for v := range n {
				if dist[v] != want[v] && math.Abs(dist[v]-want[v]) > 1e-9 {
					t.Errorf("%s source %d: dist[%d] = %v, Bellman-Ford says %v", name, src, v, dist[v], want[v])
				}
			}
			checkTree(t, g, src, dist, prev)
		}
	}
}

func TestDijkstraPanics(t *testing.T) {
	g := gonx.NewBuilder(3).Build()
	cases := map[string]func(){
		"source too large":     func() { _ = Dijkstra(g, 3, make([]float64, 3), nil) },
		"negative source":      func() { _ = Dijkstra(g, -1, make([]float64, 3), nil) },
		"short dist":           func() { _ = Dijkstra(g, 0, make([]float64, 2), nil) },
		"short prev":           func() { _ = Dijkstra(g, 0, make([]float64, 3), make([]int32, 2)) },
		"ShortestPath: source": func() { _, _, _ = ShortestPath(g, 3, 0) },
		"BreadthFirst: source": func() { BreadthFirst(g, 3, make([]int32, 3)) },
		"BreadthFirst: dist":   func() { BreadthFirst(g, 0, make([]int32, 4)) },
	}
	for name, fn := range cases {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Errorf("%s: no panic", name)
					return
				}
				// Cases named after a function must blame that function.
				if caller, _, ok := strings.Cut(name, ":"); ok && !strings.Contains(fmt.Sprint(r), caller+":") {
					t.Errorf("%s: panic %q does not name %s", name, r, caller)
				}
			}()
			fn()
		}()
	}
}

// weightedScaleFree is the shared input of the allocation test and benchmark:
// 10k nodes, 30k edges, weights in [1, 10).
func weightedScaleFree(tb testing.TB) *gonx.Graph {
	tb.Helper()
	r := gonx.NewRand(1)
	topo, err := generators.BarabasiAlbert(10_000, 3, r)
	if err != nil {
		tb.Fatal(err)
	}
	wb := gonx.NewBuilder(topo.NumNodes())
	for u, v := range topo.Edges() {
		wb.AddEdgeW(u, v, 1+r.Float64()*9)
	}
	return wb.Build()
}

// TestDijkstraAllocations pins the "O(n) scratch per call" clause: a heap and
// its bookkeeping are a handful of allocations, so anything that allocates per
// node visited or per edge relaxed fails here.
func TestDijkstraAllocations(t *testing.T) {
	g := weightedScaleFree(t)
	dist := make([]float64, g.NumNodes())
	prev := make([]int32, g.NumNodes())
	allocs := testing.AllocsPerRun(10, func() {
		if err := Dijkstra(g, 0, dist, prev); err != nil {
			t.Fatal(err)
		}
	})
	if allocs > 4 {
		t.Errorf("Dijkstra allocates %v times per call, want at most 4", allocs)
	}
}

func BenchmarkDijkstra_10000(b *testing.B) {
	g := weightedScaleFree(b)
	dist := make([]float64, g.NumNodes())
	prev := make([]int32, g.NumNodes())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Dijkstra(g, 0, dist, prev); err != nil {
			b.Fatal(err)
		}
	}
}

func TestShortestPath(t *testing.T) {
	// 0-1-2-3 with unit weights and a heavier direct 0-3; 4-5 is a separate
	// component, so 4 is unreachable from 0.
	b := gonx.NewBuilder(6)
	b.AddEdgeW(0, 1, 1)
	b.AddEdgeW(1, 2, 1)
	b.AddEdgeW(2, 3, 1)
	b.AddEdgeW(0, 3, 3.5)
	b.AddEdgeW(4, 5, 2)
	g := b.Build()

	cases := []struct {
		src, dst int
		path     []int
		length   float64
	}{
		{0, 3, []int{0, 1, 2, 3}, 3},
		{3, 0, []int{3, 2, 1, 0}, 3},
		{0, 0, []int{0}, 0},
		{4, 5, []int{4, 5}, 2},
		{0, 4, nil, math.Inf(1)},
	}
	for _, c := range cases {
		path, length, err := ShortestPath(g, c.src, c.dst)
		if err != nil {
			t.Fatalf("ShortestPath(%d, %d): %v", c.src, c.dst, err)
		}
		if !slices.Equal(path, c.path) || length != c.length {
			t.Errorf("ShortestPath(%d, %d) = %v, %v; want %v, %v", c.src, c.dst, path, length, c.path, c.length)
		}
	}

	// A search that stops early never expands node 2, so the negative edge
	// 2->1 would go unseen and [0 1] at length 1 would come back, while the
	// true shortest path is [0 2 1] at -5. The up-front check refuses instead.
	d := gonx.NewDigraphBuilder(3)
	d.AddEdgeW(0, 1, 1)
	d.AddEdgeW(0, 2, 5)
	d.AddEdgeW(2, 1, -10)
	if path, _, err := ShortestPath(d.Build(), 0, 1); !errors.Is(err, ErrNegativeWeight) {
		t.Errorf("negative edge off the explored region: path %v, err %v; want ErrNegativeWeight", path, err)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("no panic for an out-of-range target")
			}
		}()
		_, _, _ = ShortestPath(g, 0, 6)
	}()
}

// TestShortestPathMatchesDijkstra checks the early exit against a full run on
// the fixtures: same length, a path that starts and ends where it should, and
// edges whose weights add up to that length.
func TestShortestPathMatchesDijkstra(t *testing.T) {
	for _, name := range []string{"ws_undirected", "er_directed"} {
		g := loadEdges(t, name+".edges")
		n := g.NumNodes()
		dist := make([]float64, n)
		if err := Dijkstra(g, 0, dist, nil); err != nil {
			t.Fatal(err)
		}
		for dst := 0; dst < n; dst += 3 {
			path, length, err := ShortestPath(g, 0, dst)
			if err != nil {
				t.Fatalf("%s: ShortestPath(0, %d): %v", name, dst, err)
			}
			if math.IsInf(dist[dst], 1) {
				if path != nil || !math.IsInf(length, 1) {
					t.Errorf("%s: unreachable %d: path %v, length %v", name, dst, path, length)
				}
				continue
			}
			if math.Abs(length-dist[dst]) > 1e-9 {
				t.Errorf("%s: ShortestPath(0, %d) length %v, Dijkstra says %v", name, dst, length, dist[dst])
			}
			if path[0] != 0 || path[len(path)-1] != dst {
				t.Errorf("%s: path to %d runs %d..%d", name, dst, path[0], path[len(path)-1])
			}
			var sum float64
			for i := 1; i < len(path); i++ {
				w, ok := edgeWeight(g, path[i-1], path[i])
				if !ok {
					t.Fatalf("%s: path to %d uses the missing edge %d->%d", name, dst, path[i-1], path[i])
				}
				sum += w
			}
			if math.Abs(sum-length) > 1e-9 {
				t.Errorf("%s: path to %d sums to %v, reported length %v", name, dst, sum, length)
			}
		}
	}
}
