// Renders Zachary's Karate Club — the classic 34-node social network from
// Zachary (1977) — for the README gallery. The club split into two factions
// after a dispute; nodes are colored by the faction each member actually
// joined (indigo: Mr. Hi, node 0; amber: the Officer, node 33), sized by
// degree, and labeled with the standard 0-based IDs.
//
// Usage (from the repo root):
//
//	go run ./examples/karate
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"strconv"

	"github.com/LuisLSousa/gonx"
	"github.com/LuisLSousa/gonx/examples/internal/render"
)

// edges is the standard 78-edge list, 0-indexed (as in networkx.karate_club_graph).
var edges = [][2]int{
	{0, 1}, {0, 2}, {0, 3}, {0, 4}, {0, 5}, {0, 6}, {0, 7}, {0, 8}, {0, 10},
	{0, 11}, {0, 12}, {0, 13}, {0, 17}, {0, 19}, {0, 21}, {0, 31},
	{1, 2}, {1, 3}, {1, 7}, {1, 13}, {1, 17}, {1, 19}, {1, 21}, {1, 30},
	{2, 3}, {2, 7}, {2, 8}, {2, 9}, {2, 13}, {2, 27}, {2, 28}, {2, 32},
	{3, 7}, {3, 12}, {3, 13},
	{4, 6}, {4, 10},
	{5, 6}, {5, 10}, {5, 16},
	{6, 16},
	{8, 30}, {8, 32}, {8, 33},
	{9, 33},
	{13, 33},
	{14, 32}, {14, 33},
	{15, 32}, {15, 33},
	{18, 32}, {18, 33},
	{19, 33},
	{20, 32}, {20, 33},
	{22, 32}, {22, 33},
	{23, 25}, {23, 27}, {23, 29}, {23, 32}, {23, 33},
	{24, 25}, {24, 27}, {24, 31},
	{25, 31},
	{26, 29}, {26, 33},
	{27, 33},
	{28, 31}, {28, 33},
	{29, 32}, {29, 33},
	{30, 32}, {30, 33},
	{31, 32}, {31, 33},
	{32, 33},
}

// mrHi marks the members who sided with instructor Mr. Hi (node 0) after the
// split; everyone else followed the club officer (node 33).
var mrHi = map[int]bool{
	0: true, 1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true,
	8: true, 10: true, 11: true, 12: true, 13: true, 16: true, 17: true,
	19: true, 21: true,
}

func main() {
	out := flag.String("out", "docs/images/karate.svg", "output SVG path")
	flag.Parse()

	b := gonx.NewBuilder(34)
	for _, e := range edges {
		b.AddEdge(e[0], e[1])
	}
	g := b.Build()

	pos := render.ForceLayout(g, 5, 600)

	st := render.Style{
		NodeFill: func(u int) string {
			if mrHi[u] {
				return render.Indigo
			}
			return render.Amber
		},
		NodeRadius: func(u int) float64 { return 11 + 3.2*math.Sqrt(float64(g.Degree(u))) },
		EdgeStroke: func(u, v int) string {
			if mrHi[u] != mrHi[v] {
				return render.EdgeAlpha(0.60) // ties that crossed the split
			}
			return render.EdgeAlpha(0.30)
		},
		Label: func(u int) string { return strconv.Itoa(u) },
		LabelFill: func(u int) string {
			if mrHi[u] {
				return render.Ink // light text on indigo
			}
			return render.Bg // dark text on amber
		},
	}
	if err := render.WriteSVG(*out, g, pos, st); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s (%d nodes, %d edges)\n", *out, g.NumNodes(), g.NumEdges())
}
