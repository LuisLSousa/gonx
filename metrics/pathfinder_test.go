package metrics

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/LuisLSousa/gonx"
)

// checkClean verifies the invariant a PathFinder relies on between queries:
// the last query reset everything it touched, so the touched list and the heap
// are empty and every entry, up to the full capacity, is in its initial state.
func checkClean(t *testing.T, p *PathFinder) {
	t.Helper()
	s := p.s
	if s == nil {
		return
	}
	if len(s.touched) != 0 || len(s.heap.nodes) != 0 {
		t.Fatalf("%d touched nodes and %d heap entries left after a query", len(s.touched), len(s.heap.nodes))
	}
	dist, prev, pos := s.dist[:cap(s.dist)], s.prev[:cap(s.prev)], s.heap.pos[:cap(s.heap.pos)]
	for v := range dist {
		if !math.IsInf(dist[v], 1) || prev[v] != -1 || pos[v] != -1 {
			t.Fatalf("node %d holds dist %v, prev %d, pos %d after a query", v, dist[v], prev[v], pos[v])
		}
	}
}

// randomWeightedGraph returns a graph of either kind on n nodes with about m
// edges, weighted unless weighted is false, with weights on a coarse grid so
// that ties and zero-weight edges occur. negative adds one edge weighing -1.
func randomWeightedGraph(r *rand.Rand, directed, weighted, negative bool, n, m int) gonx.Adjacency {
	ub, db := gonx.NewBuilder(n), gonx.NewDigraphBuilder(n)
	add := func(u, v int, w float64) {
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
	for range m {
		add(r.IntN(n), r.IntN(n), float64(r.IntN(8))/2)
	}
	if negative && weighted {
		add(0, n-1, -1)
	}
	if directed {
		return db.Build()
	}
	return ub.Build()
}

// TestPathFinderMatchesShortestPath runs long sequences of queries through one
// PathFinder, which is where state left over from an earlier query would show:
// graphs of both kinds and of growing and shrinking sizes, views of them,
// unreachable targets, src == dst, and graphs refused for a negative weight,
// all interleaved. Every answer must be exactly ShortestPath's, whose search
// starts from fresh scratch, and the scratch must be clean after each one.
func TestPathFinderMatchesShortestPath(t *testing.T) {
	r := gonx.NewRand(12)
	var p PathFinder
	for trial := range 300 {
		n := 1 + r.IntN(60)
		g := randomWeightedGraph(r, trial%2 == 0, trial%5 != 0, trial%7 == 0, n, r.IntN(3*n))
		if trial%3 == 0 {
			var hideNodes []int
			var hideEdges [][2]int
			for range r.IntN(3) {
				hideNodes = append(hideNodes, r.IntN(n))
			}
			for range r.IntN(8) {
				u := r.IntN(n)
				if out := g.OutNeighbors(u); len(out) > 0 {
					hideEdges = append(hideEdges, [2]int{u, int(out[r.IntN(len(out))])})
				}
			}
			g = gonx.RestrictedView(g, hideNodes, hideEdges)
		}
		for range 20 {
			src, dst := r.IntN(n), r.IntN(n)
			if r.IntN(10) == 0 {
				dst = src
			}
			path, length, err := p.ShortestPath(g, src, dst)
			wantPath, wantLength, wantErr := ShortestPath(g, src, dst)
			if !errors.Is(err, wantErr) {
				t.Fatalf("trial %d: ShortestPath(%d, %d): err %v, want %v", trial, src, dst, err, wantErr)
			}
			if !slices.Equal(path, wantPath) || length != wantLength {
				t.Fatalf("trial %d: ShortestPath(%d, %d) = %v, %v; a fresh search gives %v, %v",
					trial, src, dst, path, length, wantPath, wantLength)
			}
			checkClean(t, &p)
		}
	}
}

// TestPathFinderAgainstBellmanFord checks every target from a few sources on
// one PathFinder against the naive oracle, so that the comparison above does
// not rest on ShortestPath alone.
func TestPathFinderAgainstBellmanFord(t *testing.T) {
	r := gonx.NewRand(4)
	var p PathFinder
	for trial := range 40 {
		n := 2 + r.IntN(80)
		g := randomWeightedGraph(r, trial%2 == 0, true, false, n, 3*n)
		for _, src := range []int{0, n / 2, n - 1} {
			want := bellmanFord(g, src)
			for dst := range n {
				path, length, err := p.ShortestPath(g, src, dst)
				if err != nil {
					t.Fatal(err)
				}
				if length != want[dst] && math.Abs(length-want[dst]) > 1e-9 {
					t.Fatalf("trial %d: length %d->%d = %v, Bellman-Ford says %v", trial, src, dst, length, want[dst])
				}
				checkPath(t, g, src, dst, path, length)
			}
		}
	}
}

// checkPath verifies that path runs from src to dst over edges g has, and
// that their weights add up to length; a nil path must mean +Inf.
func checkPath(t *testing.T, g gonx.Adjacency, src, dst int, path []int, length float64) {
	t.Helper()
	if path == nil {
		if !math.IsInf(length, 1) {
			t.Fatalf("no path %d->%d, but length %v", src, dst, length)
		}
		return
	}
	if path[0] != src || path[len(path)-1] != dst {
		t.Fatalf("path %d->%d runs %d..%d", src, dst, path[0], path[len(path)-1])
	}
	var sum float64
	for i := 1; i < len(path); i++ {
		w, ok := edgeWeight(g, path[i-1], path[i])
		if !ok {
			t.Fatalf("path %d->%d uses the missing edge %d->%d", src, dst, path[i-1], path[i])
		}
		sum += w
	}
	if math.Abs(sum-length) > 1e-9 {
		t.Fatalf("path %d->%d sums to %v, reported length %v", src, dst, sum, length)
	}
}

func TestPathFinderMatchesNetworkx(t *testing.T) {
	var p PathFinder
	// The fixtures differ in size, so one PathFinder also moves between them.
	for _, name := range []string{"ws_undirected", "er_directed", "er_sparse", "ba_tree", "ws_undirected"} {
		g := loadEdges(t, name+".edges")
		for src, want := range loadExpected(t, name+".dijkstra.json") {
			for dst, d := range want {
				path, length, err := p.ShortestPath(g, src, dst)
				if err != nil {
					t.Fatal(err)
				}
				switch {
				case d < 0 && !math.IsInf(length, 1):
					t.Errorf("%s: %d is unreachable from %d in networkx, got %v", name, dst, src, length)
				case d >= 0 && math.Abs(length-d) > 1e-9:
					t.Errorf("%s: length %d->%d = %v, networkx says %v", name, src, dst, length, d)
				}
				checkPath(t, g, src, dst, path, length)
			}
		}
	}
}

func TestPathFinderNegativeWeight(t *testing.T) {
	d := gonx.NewDigraphBuilder(3)
	d.AddEdgeW(0, 1, 1)
	d.AddEdgeW(0, 2, 5)
	d.AddEdgeW(2, 1, -10)
	g := d.Build()
	var p PathFinder
	if _, _, err := p.ShortestPath(g, 0, 1); !errors.Is(err, ErrNegativeWeight) {
		t.Errorf("err %v, want ErrNegativeWeight", err)
	}
	// Hiding the negative edge makes the same graph searchable, on the same
	// PathFinder.
	path, length, err := p.ShortestPath(gonx.RestrictedView(g, nil, [][2]int{{2, 1}}), 0, 1)
	if err != nil || !slices.Equal(path, []int{0, 1}) || length != 1 {
		t.Errorf("negative edge hidden: %v, %v, %v; want [0 1], 1, nil", path, length, err)
	}
}

func TestPathFinderPanics(t *testing.T) {
	g := gonx.NewBuilder(3).Build()
	for name, fn := range map[string]func(p *PathFinder){
		"source 3":  func(p *PathFinder) { p.ShortestPath(g, 3, 0) },
		"source -1": func(p *PathFinder) { p.ShortestPath(g, -1, 0) },
		"target 3":  func(p *PathFinder) { p.ShortestPath(g, 0, 3) },
		"target -1": func(p *PathFinder) { p.ShortestPath(g, 0, -1) },
	} {
		var p PathFinder
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			fn(&p)
		}()
		// A refused query leaves the PathFinder usable.
		if _, length, err := p.ShortestPath(g, 0, 0); err != nil || length != 0 {
			t.Errorf("%s: the next query gives %v, %v", name, length, err)
		}
	}
}

// TestPathFinderPanicOrder pins that both entry points check the target
// first, so a query with both endpoints out of range reports the same one.
func TestPathFinderPanicOrder(t *testing.T) {
	g := gonx.NewBuilder(3).Build()
	message := func(fn func()) (msg string) {
		defer func() { msg = fmt.Sprint(recover()) }()
		fn()
		return ""
	}
	var p PathFinder
	got := message(func() { p.ShortestPath(g, 5, 5) })
	want := message(func() { ShortestPath(g, 5, 5) })
	if !strings.Contains(got, "target 5") || !strings.Contains(want, "target 5") {
		t.Errorf("PathFinder panics with %q and ShortestPath with %q; want both to name target 5", got, want)
	}
}

// TestPathFinderCopy copies a PathFinder after it has run, which shares its
// arrays but not its record of what the last query touched. Each copy must
// answer like a fresh search, and so must the original afterwards, whether the
// copies run one after another or, under -race, at once.
func TestPathFinderCopy(t *testing.T) {
	b := gonx.NewBuilder(6)
	for i := range 5 {
		b.AddEdge(i, i+1)
	}
	path6 := b.Build()
	var p PathFinder
	p.ShortestPath(path6, 0, 0)
	q := p
	q.ShortestPath(path6, 0, 5)
	if path, length, _ := p.ShortestPath(path6, 5, 0); !slices.Equal(path, []int{5, 4, 3, 2, 1, 0}) || length != 5 {
		t.Errorf("original after its copy ran: %v, %v; want [5 4 3 2 1 0], 5", path, length)
	}
	checkClean(t, &p)
	checkClean(t, &q)

	g := weightedScaleFree(t)
	var template PathFinder
	template.ShortestPath(g, 0, 9999)
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := template
			for k := range 20 {
				src, dst := (i*1000+k*37)%10_000, (i*7+k*911)%10_000
				path, length, _ := p.ShortestPath(g, src, dst)
				wantPath, wantLength, _ := ShortestPath(g, src, dst)
				if !slices.Equal(path, wantPath) || length != wantLength {
					t.Errorf("copy %d: ShortestPath(%d, %d) differs from a fresh search", i, src, dst)
					return
				}
			}
		}()
	}
	wg.Wait()
	path, length, _ := template.ShortestPath(g, 9999, 0)
	wantPath, wantLength, _ := ShortestPath(g, 9999, 0)
	if !slices.Equal(path, wantPath) || length != wantLength {
		t.Errorf("template after its copies ran: %v, %v; want %v, %v", path, length, wantPath, wantLength)
	}
}

// TestPathFinderSavedCopy assigns a copy taken before a query back over the
// original after it, so that the copy's view of the scratch is out of date
// unless the scratch keeps its own record of what is dirty.
func TestPathFinderSavedCopy(t *testing.T) {
	b := gonx.NewBuilder(6)
	for i := range 5 {
		b.AddEdge(i, i+1)
	}
	g := b.Build()
	var p PathFinder
	p.ShortestPath(g, 0, 0)
	saved := p
	p.ShortestPath(g, 5, 0)
	p = saved
	if path, length, _ := p.ShortestPath(g, 0, 5); !slices.Equal(path, []int{0, 1, 2, 3, 4, 5}) || length != 5 {
		t.Errorf("restored copy, 0 to 5: %v, %v; want [0 1 2 3 4 5], 5", path, length)
	}
	if path, length, _ := p.ShortestPath(g, 1, 3); !slices.Equal(path, []int{1, 2, 3}) || length != 2 {
		t.Errorf("restored copy, 1 to 3: %v, %v; want [1 2 3], 2", path, length)
	}
	checkClean(t, &p)
}

// faulty panics when a search expands node bad, standing in for an Adjacency
// wrapper that breaks part way through a query.
type faulty struct {
	gonx.Adjacency
	bad int
}

func (f faulty) OutNeighbors(u int) []int32 {
	if u == f.bad {
		panic("faulty: node expanded")
	}
	return f.Adjacency.OutNeighbors(u)
}

// TestPathFinderAfterPanic cuts a query short with a panic mid-search, which
// leaves the scratch dirty, and checks that the next query, on the PathFinder
// itself or on a copy saved before the panic and assigned back, still answers
// like a fresh search.
func TestPathFinderAfterPanic(t *testing.T) {
	g := weightedScaleFree(t)
	for _, restore := range []bool{false, true} {
		var p PathFinder
		p.ShortestPath(g, 0, 1)
		saved := p
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("the faulty graph did not panic")
				}
			}()
			p.ShortestPath(faulty{g, int(g.Neighbors(0)[0])}, 0, 9999)
		}()
		if restore {
			p = saved
		}
		path, length, _ := p.ShortestPath(g, 0, 9999)
		wantPath, wantLength, _ := ShortestPath(g, 0, 9999)
		if !slices.Equal(path, wantPath) || length != wantLength {
			t.Errorf("restore %v: after the panic, %v, %v; want %v, %v", restore, path, length, wantPath, wantLength)
		}
		checkClean(t, &p)
	}
}

// TestPathFinderCopySequences runs random sequences of queries, copies and
// assignments over a few PathFinder variables, the moves under which state
// shared between copies would show, and compares every answer with a fresh
// search.
func TestPathFinderCopySequences(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	for range 50 {
		g := randomWeightedGraph(r, r.IntN(2) == 0, true, false, 2+r.IntN(40), r.IntN(120))
		n := g.NumNodes()
		var vars [3]PathFinder
		for range 60 {
			i, j := r.IntN(len(vars)), r.IntN(len(vars))
			switch r.IntN(3) {
			case 0:
				vars[j] = vars[i]
			default:
				src, dst := r.IntN(n), r.IntN(n)
				path, length, _ := vars[i].ShortestPath(g, src, dst)
				wantPath, wantLength, _ := ShortestPath(g, src, dst)
				if !slices.Equal(path, wantPath) || length != wantLength {
					t.Fatalf("variable %d, %d to %d: %v, %v; want %v, %v", i, src, dst, path, length, wantPath, wantLength)
				}
				checkClean(t, &vars[i])
			}
		}
	}
}

// TestPathFinderAllocations pins the claim that, once warmed up, a query
// allocates only the path it returns, and nothing when there is no path.
func TestPathFinderAllocations(t *testing.T) {
	g := weightedScaleFree(t)
	b := g.ToBuilder()
	b.AddNode() // isolated, so unreachable from everywhere
	g = b.Build()
	unreachable := g.NumNodes() - 1
	view := gonx.RestrictedView(g, []int{3}, [][2]int{{0, int(g.Neighbors(0)[0])}})
	var p PathFinder
	for _, c := range []struct {
		name string
		g    gonx.Adjacency
		dst  int
		want float64
	}{
		{"far", g, 9999, 1},
		{"view", view, 9999, 1},
		{"src == dst", g, 0, 1},
		{"unreachable", g, unreachable, 0},
	} {
		allocs := testing.AllocsPerRun(20, func() {
			if _, _, err := p.ShortestPath(c.g, 0, c.dst); err != nil {
				t.Fatal(err)
			}
		})
		if allocs != c.want {
			t.Errorf("%s: a query allocates %v times, want %v", c.name, allocs, c.want)
		}
	}
}

// TestPathFinderPerGoroutine runs one PathFinder per goroutine over a shared
// graph and view; under -race it checks that queries write only their own
// PathFinder.
func TestPathFinderPerGoroutine(t *testing.T) {
	g := weightedScaleFree(t)
	view := gonx.RestrictedView(g, []int{0}, nil)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var p PathFinder
			for q := range 50 {
				src, dst := (i*1000+q*37)%10_000, (i*7+q*911)%10_000
				for _, a := range []gonx.Adjacency{g, view} {
					path, length, _ := p.ShortestPath(a, src, dst)
					wantPath, wantLength, _ := ShortestPath(a, src, dst)
					if !slices.Equal(path, wantPath) || length != wantLength {
						t.Errorf("goroutine %d: ShortestPath(%d, %d) differs from a fresh search", i, src, dst)
						return
					}
				}
			}
		}()
	}
	wg.Wait()
}

var (
	gridOnce sync.Once
	grid     *gonx.Graph
)

// weightedGrid is the large input of the query benchmarks: a 1000 x 1000
// lattice, 1M nodes and 2M edges with weights in [1, 10). Its diameter of
// about 2000 hops makes near and far targets mean different amounts of work,
// which a scale-free graph, where almost every pair is a few hops apart, does
// not. It is built once per test binary.
func weightedGrid(tb testing.TB) *gonx.Graph {
	tb.Helper()
	gridOnce.Do(func() {
		const side = 1000
		r := gonx.NewRand(9)
		b := gonx.NewWeightedBuilder(side * side)
		for y := range side {
			for x := range side {
				u := y*side + x
				if x+1 < side {
					b.AddEdgeW(u, u+1, 1+r.Float64()*9)
				}
				if y+1 < side {
					b.AddEdgeW(u, u+side, 1+r.Float64()*9)
				}
			}
		}
		grid = b.Build()
	})
	return grid
}

// gridQueries returns count fixed (src, dst) pairs on the 1000 x 1000 grid:
// within reach steps of each other along each axis when reach > 0, and
// anywhere on the grid otherwise.
func gridQueries(count, reach int) [][2]int {
	const side = 1000
	r := gonx.NewRand(uint64(10 + reach))
	qs := make([][2]int, count)
	for i := range qs {
		x, y := r.IntN(side), r.IntN(side)
		tx, ty := r.IntN(side), r.IntN(side)
		if reach > 0 {
			tx = min(max(x+r.IntN(2*reach+1)-reach, 0), side-1)
			ty = min(max(y+r.IntN(2*reach+1)-reach, 0), side-1)
		}
		qs[i] = [2]int{y*side + x, ty*side + tx}
	}
	return qs
}

// BenchmarkPathQueries runs point-to-point queries one after another, each
// through ShortestPath and through one PathFinder kept across them. "near"
// targets are within 5 steps along each axis of the source, "far" ones
// anywhere, and "same" asks for src == dst, which leaves nothing but the
// per-query setup to measure.
func BenchmarkPathQueries(b *testing.B) {
	g := weightedGrid(b)
	sf := weightedScaleFree(b)
	sfQueries := make([][2]int, 256)
	r := gonx.NewRand(3)
	for i := range sfQueries {
		sfQueries[i] = [2]int{r.IntN(10_000), r.IntN(10_000)}
	}
	same := gridQueries(256, 0)
	for i := range same {
		same[i][1] = same[i][0]
	}
	for _, c := range []struct {
		name    string
		g       gonx.Adjacency
		queries [][2]int
	}{
		{"grid-near", g, gridQueries(256, 5)},
		{"grid-far", g, gridQueries(256, 0)},
		{"grid-same", g, same},
		{"scalefree", sf, sfQueries},
	} {
		b.Run(c.name+"/ShortestPath", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				q := c.queries[i%len(c.queries)]
				if _, _, err := ShortestPath(c.g, q[0], q[1]); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(c.name+"/PathFinder", func(b *testing.B) {
			var p PathFinder
			p.ShortestPath(c.g, 0, 0) // size the scratch outside the timing
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				q := c.queries[i%len(c.queries)]
				if _, _, err := p.ShortestPath(c.g, q[0], q[1]); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
