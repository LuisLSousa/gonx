// Package generators builds graphs from classic random-graph models. Every
// randomized generator takes an explicit *math/rand/v2.Rand so output is
// reproducible: the same seed and parameters always produce a byte-identical
// graph.
package generators

import (
	"fmt"
	"math/rand/v2"

	"github.com/LuisLSousa/gonx"
)

// Complete returns the complete graph K_n, in which every pair of distinct nodes
// is connected. It has n*(n-1)/2 edges.
func Complete(n int) (*gonx.Graph, error) {
	if n < 0 {
		return nil, fmt.Errorf("%w: n must be >= 0, got %d", gonx.ErrInvalidParam, n)
	}
	b := gonx.NewBuilder(n)
	for u := 0; u < n; u++ {
		for v := u + 1; v < n; v++ {
			b.AddEdgeUnchecked(u, v) // each pair enumerated exactly once
		}
	}
	return b.Build(), nil
}

// WattsStrogatz builds a Watts-Strogatz small-world graph: a ring lattice in which
// every node is joined to its k nearest neighbors (k must be even), with each
// edge then rewired to a random endpoint with probability p. The edge count,
// n*k/2, is preserved by rewiring. With p == 0 the result is the pure ring
// lattice; with p == 1 it is essentially a random graph of the same degree.
//
// This mirrors networkx.watts_strogatz_graph (the non-connected variant).
func WattsStrogatz(n, k int, p float64, r *rand.Rand) (*gonx.Graph, error) {
	if k < 0 || k%2 != 0 {
		return nil, fmt.Errorf("%w: k must be even and >= 0, got %d", gonx.ErrInvalidParam, k)
	}
	if k >= n {
		return nil, fmt.Errorf("%w: k (%d) must be < n (%d)", gonx.ErrInvalidParam, k, n)
	}
	if !(p >= 0 && p <= 1) { // negated so NaN is rejected too
		return nil, fmt.Errorf("%w: p must be in [0,1], got %g", gonx.ErrInvalidParam, p)
	}
	b := gonx.NewBuilder(n)
	// Ring lattice: connect each node to the k/2 nearest nodes on each side.
	// Each ring edge {u, u+j} is produced exactly once: j ranges over 1..k/2,
	// and k < n keeps j below n/2, so no pair repeats under the mod-n wrap.
	half := k / 2
	for u := 0; u < n; u++ {
		for j := 1; j <= half; j++ {
			b.AddEdgeUnchecked(u, (u+j)%n)
		}
	}
	if p == 0 || n <= k+1 {
		return b.Build(), nil
	}
	// Rewire: for each forward edge (u, u+j), with probability p replace its far
	// endpoint with a uniformly random node, avoiding self-loops and duplicates.
	for j := 1; j <= half; j++ {
		for u := 0; u < n; u++ {
			if r.Float64() >= p {
				continue
			}
			v := (u + j) % n
			w := r.IntN(n)
			// Reject self-loops and existing edges; bounded retries keep it simple.
			tries := 0
			for (w == u || b.HasEdge(u, w)) && tries < n {
				w = r.IntN(n)
				tries++
			}
			if w == u || b.HasEdge(u, w) {
				continue // saturated; keep original edge
			}
			b.RemoveEdge(u, v)
			b.AddEdge(u, w)
		}
	}
	return b.Build(), nil
}

// BarabasiAlbert builds a scale-free graph by preferential attachment: starting
// from m seed nodes, each new node attaches to m existing nodes chosen with
// probability proportional to their current degree.
//
// Note: m is the number of edges added per new node, NOT the average degree (the
// resulting average degree is approximately 2m). This matches
// networkx.barabasi_albert_graph, whose second argument is likewise m.
func BarabasiAlbert(n, m int, r *rand.Rand) (*gonx.Graph, error) {
	if m < 1 || m >= n {
		return nil, fmt.Errorf("%w: need 1 <= m < n, got m=%d n=%d", gonx.ErrInvalidParam, m, n)
	}
	b := gonx.NewBuilder(n)
	// repeated holds each node once per incident edge, giving degree-proportional
	// sampling when we draw uniformly from it.
	repeated := make([]int32, 0, 2*m*n)
	// Seed: the first m nodes start connected to the very next node so the
	// attachment list is non-empty (any node with degree 0 can never be chosen).
	targets := make([]int32, m)
	for i := 0; i < m; i++ {
		targets[i] = int32(i)
	}
	for newNode := m; newNode < n; newNode++ {
		for _, t := range targets {
			b.AddEdge(newNode, int(t))
			repeated = append(repeated, int32(newNode), t)
		}
		// Pick m distinct targets for the next node from the degree-weighted pool.
		targets = sampleDistinct(repeated, m, r)
	}
	return b.Build(), nil
}

// sampleDistinct draws m distinct values from the degree-weighted pool. If the
// pool is empty it falls back to the lowest-indexed nodes.
func sampleDistinct(pool []int32, m int, r *rand.Rand) []int32 {
	out := make([]int32, 0, m)
	seen := make(map[int32]struct{}, m)
	if len(pool) == 0 {
		for i := int32(0); int(i) < m; i++ {
			out = append(out, i)
		}
		return out
	}
	for len(out) < m {
		c := pool[r.IntN(len(pool))]
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

// RandomAvgDegree adds uniformly random edges until the average degree reaches at
// least avgDegree. Unlike [ErdosRenyi] it targets a mean degree directly rather
// than an edge probability, which is convenient when generating graphs of equal
// density across different sizes.
func RandomAvgDegree(n int, avgDegree float64, r *rand.Rand) (*gonx.Graph, error) {
	if n < 0 {
		return nil, fmt.Errorf("%w: n must be >= 0, got %d", gonx.ErrInvalidParam, n)
	}
	if !(avgDegree >= 0) { // negated so NaN is rejected too
		return nil, fmt.Errorf("%w: avgDegree must be >= 0, got %g", gonx.ErrInvalidParam, avgDegree)
	}
	b := gonx.NewBuilder(n)
	if n < 2 {
		return b.Build(), nil
	}
	maxEdges := n * (n - 1) / 2
	want := int(avgDegree * float64(n) / 2.0)
	if want > maxEdges {
		want = maxEdges
	}
	for b.NumEdges() < want {
		u := r.IntN(n)
		v := r.IntN(n)
		b.AddEdge(u, v) // no-op for self-loops/duplicates
	}
	return b.Build(), nil
}

// ErdosRenyi builds a G(n, p) random graph: each of the n*(n-1)/2 possible edges
// is included independently with probability p.
func ErdosRenyi(n int, p float64, r *rand.Rand) (*gonx.Graph, error) {
	if n < 0 {
		return nil, fmt.Errorf("%w: n must be >= 0, got %d", gonx.ErrInvalidParam, n)
	}
	if !(p >= 0 && p <= 1) { // negated so NaN is rejected too
		return nil, fmt.Errorf("%w: p must be in [0,1], got %g", gonx.ErrInvalidParam, p)
	}
	b := gonx.NewBuilder(n)
	for u := 0; u < n; u++ {
		for v := u + 1; v < n; v++ {
			if r.Float64() < p {
				b.AddEdgeUnchecked(u, v) // each pair enumerated exactly once
			}
		}
	}
	return b.Build(), nil
}
