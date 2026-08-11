// gen writes a seeded Barabasi-Albert edge list to a text file, one
// "u v" pair per line with u < v. Every benchmark runner loads the same
// file, so all libraries measure identical input; nothing about the
// graph depends on which library reads it.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"os"

	"github.com/LuisLSousa/gonx/generators"
)

func main() {
	n := flag.Int("n", 100000, "number of nodes")
	m := flag.Int("m", 5, "edges attached per new node")
	seed := flag.Uint64("seed", 42, "PCG seed")
	out := flag.String("out", "", "output path (required)")
	flag.Parse()
	if *out == "" {
		log.Fatal("-out is required")
	}

	r := rand.New(rand.NewPCG(*seed, *seed))
	g, err := generators.BarabasiAlbert(*n, *m, r)
	if err != nil {
		log.Fatal(err)
	}

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// Orient every undirected edge low->high: arbitrary but deterministic,
	// and identical for every library. Emitting only u < v also dedupes if
	// the iterator ever yields both directions; the count check below
	// catches the opposite failure mode (dropped edges).
	w := bufio.NewWriterSize(f, 1<<20)
	written := 0
	for u, v := range g.Edges() {
		if u > v {
			u, v = v, u
		}
		fmt.Fprintf(w, "%d %d\n", u, v)
		written++
	}
	if err := w.Flush(); err != nil {
		log.Fatal(err)
	}
	if written != g.NumEdges() {
		log.Fatalf("wrote %d edges, graph has %d", written, g.NumEdges())
	}
	fmt.Printf("wrote %s: n=%d edges=%d\n", *out, g.NumNodes(), written)
}
