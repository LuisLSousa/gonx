// Renders a Watts–Strogatz small-world network for the README gallery.
// Nodes sit on the original ring lattice; edges that survived rewiring hug the
// rim in muted gray while the rewired long-range shortcuts — the "small world"
// part — cut across the circle in cyan.
//
// Usage (from the repo root):
//
//	go run ./examples/smallworld
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/examples/internal/render"
	"github.com/LuisLSousa/gonx/generators"
)

func main() {
	out := flag.String("out", "docs/images/smallworld.svg", "output SVG path")
	flag.Parse()

	const (
		n = 48
		k = 4    // each node wired to its k nearest ring neighbors
		p = 0.12 // rewire probability
	)
	r := gonx.NewRand(11)
	g, err := generators.WattsStrogatz(n, k, p, r)
	if err != nil {
		log.Fatal(err)
	}

	// A shortcut is any edge longer than the lattice reach on the ring.
	shortcut := func(u, v int) bool {
		d := u - v
		if d < 0 {
			d = -d
		}
		if n-d < d {
			d = n - d
		}
		return d > k/2
	}
	shortcuts := 0
	for u, v := range g.Edges() {
		if shortcut(u, v) {
			shortcuts++
		}
	}

	st := render.Style{
		NodeFill:   func(int) string { return render.Indigo },
		NodeRadius: func(int) float64 { return 10 },
		EdgeStroke: func(u, v int) string {
			if shortcut(u, v) {
				return render.Cyan
			}
			return render.EdgeAlpha(0.45)
		},
		EdgeWidth: func(u, v int) float64 {
			if shortcut(u, v) {
				return 2.2
			}
			return 1.6
		},
	}
	if err := render.WriteSVG(*out, g, render.CircleLayout(n), st); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s (%d nodes, %d edges, %d shortcuts)\n", *out, g.NumNodes(), g.NumEdges(), shortcuts)
}
