// Renders a Barabási–Albert scale-free network for the README gallery.
// Preferential attachment concentrates edges on a few hubs; nodes are sized and
// colored by degree so those hubs pop out of the periphery.
//
// Usage (from the repo root):
//
//	go run ./examples/scalefree
package main

import (
	"flag"
	"fmt"
	"log"
	"math"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/examples/internal/render"
	"github.com/LuisLSousa/gonx/generators"
)

func main() {
	out := flag.String("out", "docs/images/scalefree.svg", "output SVG path")
	flag.Parse()

	r := gonx.NewRand(7)
	g, err := generators.BarabasiAlbert(150, 2, r) // 150 nodes, 2 edges per arrival
	if err != nil {
		log.Fatal(err)
	}

	pos := render.ForceLayout(g, 7, 600)

	maxDeg := 1
	for u := range g.Nodes() {
		if d := g.Degree(u); d > maxDeg {
			maxDeg = d
		}
	}
	// sqrt scaling keeps the many degree-2 leaves visible next to the big hubs.
	heat := func(u int) float64 {
		return math.Sqrt(float64(g.Degree(u)-1) / float64(maxDeg-1))
	}

	st := render.Style{
		NodeRadius: func(u int) float64 { return 5 + 21*heat(u) },
		NodeFill: func(u int) string {
			return render.Ramp(heat(u), render.Indigo, render.Rose, render.Amber)
		},
		EdgeStroke: func(u, v int) string { return render.EdgeAlpha(0.28) },
	}
	if err := render.WriteSVG(*out, g, pos, st); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s (%d nodes, %d edges, max degree %d)\n", *out, g.NumNodes(), g.NumEdges(), maxDeg)
}
