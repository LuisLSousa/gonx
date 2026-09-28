//go:build ignore

// Command gen_edges writes the weighted edge lists that the metrics parity
// tests share with networkx. Run it from the module root:
//
//	go run ./metrics/testdata/gen_edges.go
//	python3 metrics/testdata/gen_expected.py
//
// The first line of each file says "undirected N" or "directed N"; every
// following line is "u v w". Regenerate both files together when the graphs
// change, and commit them: the tests read the files and need no Python.
//
// Weights are drawn from [0.5, 10), except that every seventh edge weighs 0
// and every fifth is rounded to a multiple of 0.5, so the fixtures contain
// zero-weight edges and exact ties, the two cases where a shortest-path
// implementation is most likely to be subtly wrong.
package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/generators"
)

type weigher struct {
	r *rand.Rand
	i int
}

func (w *weigher) next() float64 {
	w.i++
	x := 0.5 + 9.5*w.r.Float64()
	switch {
	case w.i%7 == 0:
		return 0
	case w.i%5 == 0:
		return math.Round(x*2) / 2
	}
	return x
}

func main() {
	dir := filepath.Join("metrics", "testdata")

	// Undirected: a small-world graph on 60 nodes plus four isolated nodes, so
	// every source has unreachable targets.
	{
		topo, err := generators.WattsStrogatz(60, 4, 0.3, gonx.NewRand(2))
		if err != nil {
			panic(err)
		}
		w := &weigher{r: gonx.NewRand(3)}
		var lines []string
		for u, v := range topo.Edges() {
			lines = append(lines, fmt.Sprintf("%d %d %.6f", u, v, w.next()))
		}
		write(filepath.Join(dir, "ws_undirected.edges"), "undirected 64", lines)
	}

	// Directed: an Erdos-Renyi topology on 80 nodes with each edge oriented at
	// random, and a quarter of them also given a reverse edge with a different
	// weight, so u->v and v->u exist with distinct weights in places.
	{
		topo, err := generators.ErdosRenyi(80, 0.06, gonx.NewRand(5))
		if err != nil {
			panic(err)
		}
		r := gonx.NewRand(6)
		w := &weigher{r: gonx.NewRand(7)}
		var lines []string
		for u, v := range topo.Edges() {
			if r.IntN(2) == 0 {
				u, v = v, u
			}
			lines = append(lines, fmt.Sprintf("%d %d %.6f", u, v, w.next()))
			if r.IntN(4) == 0 {
				lines = append(lines, fmt.Sprintf("%d %d %.6f", v, u, w.next()))
			}
		}
		write(filepath.Join(dir, "er_directed.edges"), "directed 80", lines)
	}

	// Undirected and sparse: Erdos-Renyi on 150 nodes at an average degree
	// near 2, where a giant component with cycles coexists with trees hanging
	// off it and small separate pieces, so bridges, cut nodes and isolated
	// nodes all occur. The cuts tests read it; weights are there only because
	// the format has them.
	{
		topo, err := generators.ErdosRenyi(150, 0.014, gonx.NewRand(11))
		if err != nil {
			panic(err)
		}
		w := &weigher{r: gonx.NewRand(12)}
		var lines []string
		for u, v := range topo.Edges() {
			lines = append(lines, fmt.Sprintf("%d %d %.6f", u, v, w.next()))
		}
		write(filepath.Join(dir, "er_sparse.edges"), "undirected 150", lines)
	}
}

func write(path, header string, lines []string) {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	fmt.Fprintln(f, header)
	for _, l := range lines {
		fmt.Fprintln(f, l)
	}
	fmt.Fprintf(os.Stderr, "%s: %d edges\n", path, len(lines))
}
