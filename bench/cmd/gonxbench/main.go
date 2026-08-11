// gonxbench times gonx on the shared edge list. Operations: directed
// build (edge arrays -> CSR), PageRank (damping 0.85, tolerance 1e-6,
// networkx-style n*tol L1 stopping rule), weakly connected components,
// and BFS reachability over out-edges from the highest out-degree node.
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
	"os"
	"strconv"
	"time"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/metrics"
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
	top, topScore := 0, rank[0]
	for v, s := range rank {
		if s > topScore {
			top, topScore = v, s
		}
	}

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
	for i := range *repeats {
		start := time.Now()
		reached = bfsOut(g, src)
		emit("gonx", "bfs", n, edges, i, time.Since(start))
	}

	fmt.Printf("#check,gonx,n=%d,edges=%d,pr_top=%d,pr_top_score=%.9f,wcc=%d,bfs_src=%d,bfs_reached=%d\n",
		n, edges, top, topScore, len(comps), src, reached)
}

// bfsOut counts the nodes reachable from src by following out-edges,
// including src itself.
func bfsOut(g *gonx.Digraph, src int) int {
	visited := make([]bool, g.NumNodes())
	queue := make([]int32, 0, 1024)
	visited[src] = true
	queue = append(queue, int32(src))
	count := 0
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		count++
		for _, v := range g.OutNeighbors(int(u)) {
			if !visited[v] {
				visited[v] = true
				queue = append(queue, v)
			}
		}
	}
	return count
}

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
