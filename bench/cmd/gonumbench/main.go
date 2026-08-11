// gonumbench times gonum/graph on the shared edge list, mirroring
// gonxbench's protocol: directed build into simple.DirectedGraph,
// PageRank (damping 0.85, tolerance 1e-6), weakly connected components
// (via the graph.Undirect adapter over topo.ConnectedComponents), and
// BFS reachability over out-edges from the highest out-degree node
// (traverse.BreadthFirst).
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/network"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"
	"gonum.org/v1/gonum/graph/traverse"
)

func main() {
	in := flag.String("in", "", "edge list path (required)")
	repeats := flag.Int("repeats", 3, "repeats per operation")
	flag.Parse()
	if *in == "" {
		log.Fatal("-in is required")
	}

	us, vs, n := readEdges(*in)
	edges := len(us)

	var g *simple.DirectedGraph
	for i := range *repeats {
		start := time.Now()
		g = simple.NewDirectedGraph()
		for j := range us {
			g.SetEdge(simple.Edge{F: simple.Node(us[j]), T: simple.Node(vs[j])})
		}
		emit("gonum", "build", n, edges, i, time.Since(start))
	}

	// PageRankSparse is gonum's adjacency-list implementation; the dense
	// network.PageRank builds an n*n matrix and cannot fit n=1M in memory.
	var rank map[int64]float64
	for i := range *repeats {
		start := time.Now()
		rank = network.PageRankSparse(g, 0.85, 1e-6)
		emit("gonum", "pagerank", n, edges, i, time.Since(start))
	}
	var top int64
	topScore := -1.0
	for v, s := range rank {
		if s > topScore || (s == topScore && v < top) {
			top, topScore = v, s
		}
	}

	var comps [][]graph.Node
	for i := range *repeats {
		start := time.Now()
		comps = topo.ConnectedComponents(graph.Undirect{G: g})
		emit("gonum", "wcc", n, edges, i, time.Since(start))
	}

	outDeg := make([]int, n)
	for j := range us {
		outDeg[us[j]]++
	}
	src := 0
	for u := 1; u < n; u++ {
		if outDeg[u] > outDeg[src] {
			src = u
		}
	}
	reached := 0
	for i := range *repeats {
		start := time.Now()
		reached = 0
		bf := traverse.BreadthFirst{Visit: func(graph.Node) { reached++ }}
		bf.Walk(g, g.Node(int64(src)), nil)
		emit("gonum", "bfs", n, edges, i, time.Since(start))
	}

	// gonum's PageRank normalizes differently from the n*tol L1 stopping
	// rule; scores are cross-checked for top-node agreement, not value.
	fmt.Printf("#check,gonum,n=%d,edges=%d,pr_top=%d,pr_top_score=%.9f,wcc=%d,bfs_src=%d,bfs_reached=%d\n",
		n, edges, top, topScore, len(comps), src, reached)
}

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
