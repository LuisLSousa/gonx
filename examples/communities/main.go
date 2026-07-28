// Renders a planted-partition graph for the README gallery: four dense
// communities joined by sparse bridges, built directly with gonx.Builder and
// colored by community. The force layout is seeded with each block at its own
// compass point so the clusters settle cleanly apart.
//
// Usage (from the repo root):
//
//	go run ./examples/communities
package main

import (
	"flag"
	"fmt"
	"log"
	"math"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/examples/internal/render"
)

func main() {
	out := flag.String("out", "docs/images/communities.svg", "output SVG path")
	flag.Parse()

	const (
		blocks   = 4
		perBlock = 22
		pIn      = 0.28  // edge probability inside a community
		pOut     = 0.005 // edge probability between communities
	)
	n := blocks * perBlock
	community := func(u int) int { return u / perBlock }

	r := gonx.NewRand(3)
	b := gonx.NewBuilder(n)
	for u := range n {
		for v := u + 1; v < n; v++ {
			p := pOut
			if community(u) == community(v) {
				p = pIn
			}
			if r.Float64() < p {
				b.AddEdge(u, v)
			}
		}
	}
	g := b.Build()

	bridges := 0
	for u, v := range g.Edges() {
		if community(u) != community(v) {
			bridges++
		}
	}

	// Seed each community around its own compass point, then relax.
	pos := make([]render.Point, n)
	for u := range n {
		a := 2 * math.Pi * (float64(community(u)) + 0.5) / blocks
		pos[u] = render.Point{X: 0.6 * math.Cos(a), Y: 0.6 * math.Sin(a)}
	}
	render.Jitter(pos, 0.18, gonx.NewRand(99))
	render.Relax(g, pos, 400)

	palette := [blocks]string{render.Indigo, render.Cyan, render.Amber, render.Rose}
	st := render.Style{
		NodeFill:   func(u int) string { return palette[community(u)] },
		NodeRadius: func(u int) float64 { return 6 + 1.4*math.Sqrt(float64(g.Degree(u))) },
		EdgeStroke: func(u, v int) string {
			if community(u) != community(v) {
				return render.EdgeAlpha(0.55) // bridges slightly brighter
			}
			return render.EdgeAlpha(0.25)
		},
	}
	if err := render.WriteSVG(*out, g, pos, st); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s (%d nodes, %d edges, %d bridges)\n", *out, g.NumNodes(), g.NumEdges(), bridges)
}
