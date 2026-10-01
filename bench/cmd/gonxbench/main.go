// gonxbench times gonx on the shared edge list. Operations: directed
// build (edge arrays -> CSR), PageRank (damping 0.85, tolerance 1e-6,
// networkx-style n*tol L1 stopping rule), weakly connected components,
// and BFS reachability over out-edges from the highest out-degree node.
// With -op dijkstra it instead times single-source Dijkstra from that node
// over edgeWeight, in a process of its own so the weighted copy of the
// graph does not inflate the peak memory of the core run.
//
// Edge parsing happens before any timing starts; every library's runner
// times the same work on the same arrays. Results go to stdout as CSV
// rows "lib,op,n,edges,repeat,seconds", plus "#check" comment lines the
// orchestrator uses to confirm all libraries computed the same answers.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/metrics"
)

func main() {
	in := flag.String("in", "", "edge list path (required)")
	repeats := flag.Int("repeats", 3, "repeats per operation")
	op := flag.String("op", "core", "core (build, pagerank, wcc, bfs) or dijkstra")
	flag.Parse()
	if *in == "" {
		log.Fatal("-in is required")
	}

	us, vs, n := readEdges(*in)
	edges := len(us)
	if *op == "dijkstra" {
		benchDijkstra(us, vs, n, *repeats)
		return
	}

	var g *gonx.Digraph
	for i := range *repeats {
		start := time.Now()
		b := gonx.NewDigraphBuilder(n)
		for j := range us {
			b.AddEdgeUnchecked(us[j], vs[j])
		}
		g = b.Build()
		emit("gonx", "build", n, edges, i, time.Since(start))
	}

	// tolerance 1e-10, not the networkx-default 1e-6: the shared stopping
	// rule is L1 delta < n*tol, and at n=1M a 1e-6 tol makes the threshold
	// 1.0, which stops after a couple of iterations. 1e-10 keeps the rule
	// biting at every benchmarked size so the timed work is real
	// convergence, comparable with igraph's direct solver.
	var rank []float64
	for i := range *repeats {
		start := time.Now()
		var err error
		rank, err = metrics.PageRank(g, 0.85, 1e-10, 200)
		if err != nil {
			log.Fatal(err)
		}
		emit("gonx", "pagerank", n, edges, i, time.Since(start))
	}
	top, second := topTwo(n, func(v int) float64 { return rank[v] })

	var comps [][]int
	for i := range *repeats {
		start := time.Now()
		comps = metrics.WeaklyConnectedComponents(g)
		emit("gonx", "wcc", n, edges, i, time.Since(start))
	}

	src := 0
	for u := 1; u < n; u++ {
		if g.OutDegree(u) > g.OutDegree(src) {
			src = u
		}
	}
	reached := 0
	var hops []int32
	for i := range *repeats {
		// The result slice is allocated inside the timing, as for Dijkstra,
		// since the other libraries allocate theirs too.
		start := time.Now()
		hops = make([]int32, n)
		metrics.BreadthFirst(g, src, hops)
		emit("gonx", "bfs", n, edges, i, time.Since(start))
	}
	for _, h := range hops {
		if h >= 0 {
			reached++
		}
	}

	fmt.Printf("#check,gonx,n=%d,edges=%d,pr_top=%d,pr_top_score=%.9f,pr_second=%d,pr_second_score=%.9f,wcc=%d,bfs_src=%d,bfs_reached=%d\n",
		n, edges, top, rank[top], second, rank[second], len(comps), src, reached)
}

// edgeWeight is the weight every runner gives the edge u->v: a fixed
// integer hash mapped to [1, 2) in steps of 1/1000, so all four libraries
// see bit-identical weights without a weighted edge-list format.
func edgeWeight(u, v int) float64 {
	return 1 + float64((u*7919+v*104729)%1000)/1000
}

// benchDijkstra times single-source shortest path lengths from the highest
// out-degree node. Building the weighted graph is not timed; allocating the
// distance slice is, since the other libraries allocate their result too.
func benchDijkstra(us, vs []int, n, repeats int) {
	edges := len(us)
	b := gonx.NewWeightedDigraphBuilder(n)
	for j := range us {
		b.AddEdgeW(us[j], vs[j], edgeWeight(us[j], vs[j]))
	}
	g := b.Build()
	src := 0
	for u := 1; u < n; u++ {
		if g.OutDegree(u) > g.OutDegree(src) {
			src = u
		}
	}
	var dist []float64
	for i := range repeats {
		start := time.Now()
		dist = make([]float64, n)
		if err := metrics.Dijkstra(g, src, dist, nil); err != nil {
			log.Fatal(err)
		}
		emit("gonx", "dijkstra", n, edges, i, time.Since(start))
	}
	reached, sum, far := 0, int64(0), int64(0)
	for _, d := range dist {
		if !math.IsInf(d, 1) {
			reached++
			sum += milli(d)
			far = max(far, milli(d))
		}
	}
	fmt.Printf("#check,gonx,n=%d,edges=%d,sp_src=%d,sp_reached=%d,sp_sum_milli=%d,sp_max_milli=%d\n",
		n, edges, src, reached, sum, far)
}

// topTwo returns the two highest-scoring of n nodes, ties going to the smaller
// ID, as every runner picks them. The runner-up lets the check tell two hubs
// whose scores are close enough to swap between runs from a wrong answer.
func topTwo(n int, score func(int) float64) (first, second int) {
	first, second = -1, -1
	for v := range n {
		s := score(v)
		switch {
		case first < 0 || s > score(first):
			first, second = v, first
		case second < 0 || s > score(second):
			second = v
		}
	}
	return first, second
}

// milli is a distance in thousandths. Every edge weight is a multiple of
// 1/1000, so every distance is too, up to a float error far below half a
// unit; summing milli values is exact, which lets the check compare the
// libraries' totals digit for digit whatever order they add in.
func milli(d float64) int64 { return int64(math.Round(d * 1000)) }

// readEdges parses the "u v" edge list into two arrays and returns them
// with the node count (max id + 1). This is deliberately outside all
// timed sections.
func readEdges(path string) (us, vs []int, n int) {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		sp := -1
		for i := range line {
			if line[i] == ' ' {
				sp = i
				break
			}
		}
		if sp < 0 {
			continue
		}
		u, err1 := strconv.Atoi(line[:sp])
		v, err2 := strconv.Atoi(line[sp+1:])
		if err1 != nil || err2 != nil {
			log.Fatalf("bad line %q", line)
		}
		us = append(us, u)
		vs = append(vs, v)
		if u >= n {
			n = u + 1
		}
		if v >= n {
			n = v + 1
		}
	}
	if err := sc.Err(); err != nil {
		log.Fatal(err)
	}
	return us, vs, n
}

func emit(lib, op string, n, edges, repeat int, d time.Duration) {
	fmt.Printf("%s,%s,%d,%d,%d,%.6f\n", lib, op, n, edges, repeat, d.Seconds())
}
