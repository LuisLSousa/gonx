package metrics

import (
	"errors"
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/LuisLSousa/gonx"
)

// pageRankReference is an independent push-based implementation of the same
// model, kept deliberately different in structure from the library's pull-based
// one so a shared bug is unlikely to hide in both.
func pageRankReference(g *gonx.Digraph, damping float64, iters int) []float64 {
	n := g.NumNodes()
	rank := make([]float64, n)
	for i := range rank {
		rank[i] = 1 / float64(n)
	}
	for range iters {
		next := make([]float64, n)
		var dangling float64
		for u := range n {
			out := g.OutNeighbors(u)
			if len(out) == 0 {
				dangling += rank[u]
				continue
			}
			share := rank[u] / float64(len(out))
			for _, v := range out {
				next[v] += share
			}
		}
		for v := range next {
			next[v] = (1-damping)/float64(n) + damping*(next[v]+dangling/float64(n))
		}
		rank = next
	}
	return rank
}

func TestPageRankTwoNodeCycle(t *testing.T) {
	b := gonx.NewDigraphBuilder(2)
	b.AddEdge(0, 1)
	b.AddEdge(1, 0)
	ranks, err := PageRank(b.Build(), 0.85, 1e-10, 100)
	if err != nil {
		t.Fatal(err)
	}
	// Perfect symmetry: both nodes must get exactly half.
	if math.Abs(ranks[0]-0.5) > 1e-9 || math.Abs(ranks[1]-0.5) > 1e-9 {
		t.Errorf("ranks = %v, want [0.5 0.5]", ranks)
	}
}

func TestPageRankMatchesReference(t *testing.T) {
	// A messy random digraph with cycles, dangling nodes, and isolated
	// nodes; the library must agree with the independent implementation.
	r := gonx.NewRand(42)
	b := gonx.NewDigraphBuilder(200)
	for range 900 {
		b.AddEdge(r.IntN(200), r.IntN(200))
	}
	g := b.Build()

	got, err := PageRank(g, 0.85, 1e-12, 500)
	if err != nil {
		t.Fatal(err)
	}
	want := pageRankReference(g, 0.85, 500)
	var sum float64
	for v := range got {
		if math.Abs(got[v]-want[v]) > 1e-9 {
			t.Fatalf("rank[%d] = %.12f, reference %.12f", v, got[v], want[v])
		}
		sum += got[v]
	}
	if math.Abs(sum-1) > 1e-9 {
		t.Errorf("ranks sum to %.12f, want 1", sum)
	}
}

func TestPageRankDangling(t *testing.T) {
	// 0 -> 1 with an isolated node 2. Node 1 is dangling; without the
	// dangling correction the total rank would leak away each pass.
	b := gonx.NewDigraphBuilder(3)
	b.AddEdge(0, 1)
	ranks, err := PageRank(b.Build(), 0.85, 1e-10, 200)
	if err != nil {
		t.Fatal(err)
	}
	var sum float64
	for _, x := range ranks {
		sum += x
	}
	if math.Abs(sum-1) > 1e-9 {
		t.Fatalf("ranks sum to %.12f, want 1", sum)
	}
	if !(ranks[1] > ranks[0]) {
		t.Errorf("rank of 1 (%.6f) should exceed rank of its only source 0 (%.6f)", ranks[1], ranks[0])
	}
	// 0 and 2 both receive only teleport + dangling mass; equal by symmetry.
	if math.Abs(ranks[0]-ranks[2]) > 1e-9 {
		t.Errorf("ranks of 0 and 2 differ: %.12f vs %.12f", ranks[0], ranks[2])
	}
}

func TestPageRankParamErrors(t *testing.T) {
	g := gonx.NewDigraphBuilder(2).Build()
	if _, err := PageRank(g, 1.0, 1e-6, 10); !errors.Is(err, gonx.ErrInvalidParam) {
		t.Errorf("damping = 1.0: err = %v, want ErrInvalidParam", err)
	}
	if _, err := PageRank(g, -0.1, 1e-6, 10); !errors.Is(err, gonx.ErrInvalidParam) {
		t.Errorf("damping = -0.1: err = %v, want ErrInvalidParam", err)
	}
	if _, err := PageRank(g, 0.85, 0, 10); !errors.Is(err, gonx.ErrInvalidParam) {
		t.Errorf("tol = 0: err = %v, want ErrInvalidParam", err)
	}
}

func TestPageRankNoConvergence(t *testing.T) {
	// Not a cycle: on a regular cycle the uniform start vector is already
	// the fixed point and a single pass "converges". This asymmetric
	// graph cannot settle in one iteration.
	b := gonx.NewDigraphBuilder(3)
	b.AddEdge(0, 1)
	b.AddEdge(0, 2)
	b.AddEdge(1, 2)
	ranks, err := PageRank(b.Build(), 0.85, 1e-15, 1)
	if !errors.Is(err, ErrNoConvergence) {
		t.Fatalf("err = %v, want ErrNoConvergence", err)
	}
	if len(ranks) != 3 {
		t.Error("partial ranks not returned alongside ErrNoConvergence")
	}
}

func TestPageRankEmptyGraph(t *testing.T) {
	ranks, err := PageRank(gonx.NewDigraphBuilder(0).Build(), 0.85, 1e-6, 10)
	if err != nil || len(ranks) != 0 {
		t.Errorf("empty graph: ranks = %v, err = %v", ranks, err)
	}
}

func TestWeaklyConnectedComponents(t *testing.T) {
	// Component {0, 1, 4} is only connected against the arrows
	// (1 -> 0, 1 -> 4); weak connectivity must ignore that. 2 -> 3 is a
	// second component and 5 is isolated.
	b := gonx.NewDigraphBuilder(6)
	b.AddEdge(1, 0)
	b.AddEdge(1, 4)
	b.AddEdge(2, 3)
	comps := WeaklyConnectedComponents(b.Build())
	want := [][]int{{0, 1, 4}, {2, 3}, {5}}
	if len(comps) != len(want) {
		t.Fatalf("got %d components, want %d: %v", len(comps), len(want), comps)
	}
	for i := range want {
		got := slices.Clone(comps[i])
		slices.Sort(got)
		if !reflect.DeepEqual(got, want[i]) {
			t.Errorf("component %d = %v, want %v", i, got, want[i])
		}
	}
}
