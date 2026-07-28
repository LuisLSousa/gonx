// Package render lays out gonx graphs and writes them as self-contained SVG
// images. It exists so the examples in examples/ can regenerate the README
// gallery with zero dependencies beyond the standard library; it is NOT part of
// the gonx API (serialization and rendering are out of scope for the library).
package render

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"

	"github.com/LuisLSousa/gonx"
)

// Shared palette so the gallery images read as one set.
const (
	Bg     = "#0d1117" // canvas (GitHub-dark)
	Ink    = "#e6edf3" // light text
	Indigo = "#6366f1"
	Cyan   = "#22d3ee"
	Amber  = "#fbbf24"
	Rose   = "#fb7185"
)

// EdgeAlpha returns the standard muted edge gray at the given opacity.
func EdgeAlpha(a float64) string {
	return fmt.Sprintf("rgba(139,148,158,%.2f)", a)
}

// Ramp linearly interpolates through the given "#rrggbb" color stops at
// t in [0, 1]. With three stops, t = 0.5 lands exactly on the middle one.
func Ramp(t float64, stops ...string) string {
	if len(stops) == 0 {
		return Ink
	}
	if len(stops) == 1 {
		return stops[0]
	}
	t = math.Max(0, math.Min(1, t))
	seg := t * float64(len(stops)-1)
	i := int(seg)
	if i >= len(stops)-1 {
		i = len(stops) - 2
	}
	f := seg - float64(i)
	r0, g0, b0 := hexRGB(stops[i])
	r1, g1, b1 := hexRGB(stops[i+1])
	lerp := func(a, b int) int { return a + int(math.Round(f*float64(b-a))) }
	return fmt.Sprintf("#%02x%02x%02x", lerp(r0, r1), lerp(g0, g1), lerp(b0, b1))
}

func hexRGB(s string) (r, g, b int) {
	if _, err := fmt.Sscanf(strings.TrimPrefix(s, "#"), "%02x%02x%02x", &r, &g, &b); err != nil {
		return 0, 0, 0
	}
	return
}

// Point is a node position in layout space (arbitrary units; SVG fits it later).
type Point struct{ X, Y float64 }

// CircleLayout places n nodes evenly on a unit circle, node 0 at twelve o'clock,
// proceeding clockwise so ascending IDs read like a clock face.
func CircleLayout(n int) []Point {
	pos := make([]Point, n)
	for u := range n {
		a := 2*math.Pi*float64(u)/float64(n) - math.Pi/2
		pos[u] = Point{X: math.Cos(a), Y: math.Sin(a)}
	}
	return pos
}

// RandomLayout scatters n nodes deterministically in the unit square.
func RandomLayout(n int, seed uint64) []Point {
	r := gonx.NewRand(seed)
	pos := make([]Point, n)
	for u := range pos {
		pos[u] = Point{X: r.Float64(), Y: r.Float64()}
	}
	return pos
}

// ForceLayout computes a Fruchterman-Reingold layout from deterministic random
// initial positions. Same graph + seed + iters always yields the same picture.
func ForceLayout(g *gonx.Graph, seed uint64, iters int) []Point {
	pos := RandomLayout(g.NumNodes(), seed)
	Relax(g, pos, iters)
	return pos
}

// Relax refines pos in place with the Fruchterman-Reingold force model:
// all pairs repel with k^2/d, edges attract with d^2/k, a mild gravity pulls
// toward the centroid, and a linearly cooling temperature caps each step.
// O(iters * n^2); intended for the few-hundred-node graphs in the gallery.
func Relax(g *gonx.Graph, pos []Point, iters int) {
	n := g.NumNodes()
	if n < 2 || iters <= 0 {
		return
	}
	k := math.Sqrt(1.0 / float64(n)) // ideal edge length in a unit-area layout
	disp := make([]Point, n)
	for it := range iters {
		t := 0.1 * (1 - float64(it)/float64(iters))
		for i := range disp {
			disp[i] = Point{}
		}
		for u := range n {
			for v := u + 1; v < n; v++ {
				dx, dy := pos[u].X-pos[v].X, pos[u].Y-pos[v].Y
				d2 := dx*dx + dy*dy
				if d2 < 1e-9 {
					d2 = 1e-9
				}
				f := k * k / d2 // repulsion force divided by distance
				disp[u].X += dx * f
				disp[u].Y += dy * f
				disp[v].X -= dx * f
				disp[v].Y -= dy * f
			}
		}
		for u, v := range g.Edges() {
			dx, dy := pos[u].X-pos[v].X, pos[u].Y-pos[v].Y
			d := math.Hypot(dx, dy)
			if d < 1e-9 {
				continue
			}
			f := d / k // attraction force divided by distance
			disp[u].X -= dx * f
			disp[u].Y -= dy * f
			disp[v].X += dx * f
			disp[v].Y += dy * f
		}
		var cx, cy float64
		for _, p := range pos {
			cx += p.X
			cy += p.Y
		}
		cx /= float64(n)
		cy /= float64(n)
		for u := range n {
			disp[u].X += (cx - pos[u].X) * 0.03
			disp[u].Y += (cy - pos[u].Y) * 0.03
			d := math.Hypot(disp[u].X, disp[u].Y)
			if d < 1e-9 {
				continue
			}
			s := math.Min(d, t) / d
			pos[u].X += disp[u].X * s
			pos[u].Y += disp[u].Y * s
		}
	}
}

// Jitter nudges each position by up to ±amount in both axes, deterministically.
// Useful to break symmetry in hand-seeded layouts before calling Relax.
func Jitter(pos []Point, amount float64, r *rand.Rand) {
	for i := range pos {
		pos[i].X += amount * (2*r.Float64() - 1)
		pos[i].Y += amount * (2*r.Float64() - 1)
	}
}

// Style controls how a graph is drawn. Per-node and per-edge callbacks may be
// nil, in which case sensible defaults from the shared palette are used.
type Style struct {
	Width, Height int     // canvas size in px (default 1600x1000)
	Pad           float64 // inner margin in px (default 70)
	Background    string  // default Bg

	NodeFill   func(u int) string  // default Indigo
	NodeRadius func(u int) float64 // default 8
	NodeStroke string              // ring separating overlapping nodes; default Bg

	EdgeStroke func(u, v int) string  // default EdgeAlpha(0.30)
	EdgeWidth  func(u, v int) float64 // default 1.4

	Label     func(u int) string // empty string = no label (default: no labels)
	LabelFill func(u int) string // default Bg (dark text on bright nodes)
}

// separate nudges overlapping nodes apart in pixel space so no circle covers
// another (labels stay readable). O(iters·n²) — fine at gallery sizes.
func separate(px []Point, radius func(int) float64, w, h, pad float64) {
	n := len(px)
	const gap = 3.0
	for range 60 {
		moved := false
		for u := range n {
			for v := u + 1; v < n; v++ {
				dx, dy := px[v].X-px[u].X, px[v].Y-px[u].Y
				d := math.Hypot(dx, dy)
				min := radius(u) + radius(v) + gap
				if d >= min {
					continue
				}
				if d < 1e-9 {
					dx, dy, d = 1, 0, 1
				}
				push := (min - d) / 2
				ux, uy := dx/d, dy/d
				px[u].X -= ux * push
				px[u].Y -= uy * push
				px[v].X += ux * push
				px[v].Y += uy * push
				moved = true
			}
		}
		if !moved {
			break
		}
	}
	for i := range px {
		px[i].X = math.Min(math.Max(px[i].X, pad), w-pad)
		px[i].Y = math.Min(math.Max(px[i].Y, pad), h-pad)
	}
}

// SVG renders the graph at the given positions into an SVG document string.
// Positions are fitted to the canvas preserving aspect ratio and centered.
func SVG(g *gonx.Graph, pos []Point, st Style) string {
	if st.Width == 0 {
		st.Width = 1600
	}
	if st.Height == 0 {
		st.Height = 1000
	}
	if st.Pad == 0 {
		st.Pad = 70
	}
	if st.Background == "" {
		st.Background = Bg
	}
	if st.NodeFill == nil {
		st.NodeFill = func(int) string { return Indigo }
	}
	if st.NodeRadius == nil {
		st.NodeRadius = func(int) float64 { return 8 }
	}
	if st.NodeStroke == "" {
		st.NodeStroke = Bg
	}
	if st.EdgeStroke == nil {
		s := EdgeAlpha(0.30)
		st.EdgeStroke = func(int, int) string { return s }
	}
	if st.EdgeWidth == nil {
		st.EdgeWidth = func(int, int) float64 { return 1.4 }
	}
	if st.LabelFill == nil {
		st.LabelFill = func(int) string { return Bg }
	}

	px := fit(pos, float64(st.Width), float64(st.Height), st.Pad)
	separate(px, st.NodeRadius, float64(st.Width), float64(st.Height), st.Pad)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`+"\n",
		st.Width, st.Height, st.Width, st.Height)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" rx="16" fill="%s"/>`+"\n", st.Width, st.Height, st.Background)

	for u, v := range g.Edges() {
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%.1f"/>`+"\n",
			px[u].X, px[u].Y, px[v].X, px[v].Y, st.EdgeStroke(u, v), st.EdgeWidth(u, v))
	}
	for u := range g.Nodes() {
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s" stroke="%s" stroke-width="1.5"/>`+"\n",
			px[u].X, px[u].Y, st.NodeRadius(u), st.NodeFill(u), st.NodeStroke)
	}
	if st.Label != nil {
		for u := range g.Nodes() {
			lbl := st.Label(u)
			if lbl == "" {
				continue
			}
			size := st.NodeRadius(u) * 0.95
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" dominant-baseline="central" `+
				`font-family="-apple-system, 'Segoe UI', Helvetica, sans-serif" font-weight="600" `+
				`font-size="%.1f" fill="%s">%s</text>`+"\n",
				px[u].X, px[u].Y, size, st.LabelFill(u), lbl)
		}
	}
	b.WriteString("</svg>\n")
	return b.String()
}

// WriteSVG renders the graph and writes it to path, creating parent directories.
func WriteSVG(path string, g *gonx.Graph, pos []Point, st Style) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(SVG(g, pos, st)), 0o644)
}

// fit maps layout positions into the canvas, preserving aspect ratio, centered.
func fit(pos []Point, w, h, pad float64) []Point {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, p := range pos {
		minX, maxX = math.Min(minX, p.X), math.Max(maxX, p.X)
		minY, maxY = math.Min(minY, p.Y), math.Max(maxY, p.Y)
	}
	spanX, spanY := maxX-minX, maxY-minY
	if spanX < 1e-12 {
		spanX = 1
	}
	if spanY < 1e-12 {
		spanY = 1
	}
	scale := math.Min((w-2*pad)/spanX, (h-2*pad)/spanY)
	offX := (w - spanX*scale) / 2
	offY := (h - spanY*scale) / 2
	out := make([]Point, len(pos))
	for i, p := range pos {
		out[i] = Point{
			X: offX + (p.X-minX)*scale,
			Y: offY + (p.Y-minY)*scale,
		}
	}
	return out
}
