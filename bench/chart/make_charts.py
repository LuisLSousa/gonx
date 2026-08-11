"""Renders the benchmark receipts chart for the gonx README from
results/results.csv: a Cleveland dot plot on a log-x time axis (bars
would lie on a log scale), one row per operation at the largest size,
plus a peak-memory panel. Also prints the median/speedup table used in
the README.

Colors are the dataviz reference categorical palette in fixed library
order, validated for the light surface (#fcfcfb): gonx blue #2a78d6,
networkx orange #eb6834, igraph aqua #1baf7a, gonum yellow #eda100.
The aqua/yellow contrast WARN is relieved by visible ink labels and the
numbers table that always accompanies the chart.
"""

import csv
import math
import statistics
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
RESULTS = HERE.parent / "results" / "results.csv"
OUT = HERE.parent / "results" / "receipts.svg"

LIBS = ["gonx", "networkx", "igraph", "gonum"]  # fixed identity order
COLORS = {
    "gonx": "#2a78d6",
    "networkx": "#eb6834",
    "igraph": "#1baf7a",
    "gonum": "#eda100",
}
OPS = ["build", "pagerank", "wcc", "bfs"]
OP_LABELS = {
    "build": "build (5.0M edges)",
    "pagerank": "PageRank",
    "wcc": "weak components",
    "bfs": "BFS reachability",
}

INK = "#1e293b"
INK2 = "#475569"
MUTED = "#94a3b8"
GRID = "#e2e8f0"
SURFACE = "#fcfcfb"


def fmt_time(s):
    if s < 1e-3:
        return f"{s * 1e6:.0f} us"
    if s < 1:
        return f"{s * 1e3:.0f} ms"
    if s < 60:
        return f"{s:.1f} s" if s < 10 else f"{s:.0f} s"
    return f"{s / 60:.1f} min"


def fmt_mem(b):
    mb = b / (1 << 20)
    if mb < 1000:
        return f"{mb:.0f} MB"
    return f"{mb / 1024:.1f} GB"


def load(n_target):
    times = {}  # (lib, op) -> median seconds
    mem = {}  # lib -> peak rss bytes
    with open(RESULTS) as f:
        rows = list(csv.reader(f))
    for lib in LIBS:
        for op in OPS:
            vals = [
                float(r[5])
                for r in rows
                if r[0] == lib and r[1] == op and int(r[2]) == n_target
            ]
            if vals:
                times[(lib, op)] = statistics.median(vals)
        m = [
            int(r[5])
            for r in rows
            if r[0] == lib and r[1] == "mem" and int(r[2]) == n_target
        ]
        if m:
            mem[lib] = max(m)
    return times, mem


def log_x(v, lo, hi, x0, x1):
    return x0 + (math.log10(v) - math.log10(lo)) / (
        math.log10(hi) - math.log10(lo)
    ) * (x1 - x0)


def main():
    n_target = int(sys.argv[1]) if len(sys.argv) > 1 else 1_000_000
    times, mem = load(n_target)
    if not times:
        sys.exit(f"no rows for n={n_target} in {RESULTS}")

    W, H = 680, 432
    x0, x1 = 150, 640
    row_h = 44
    top = 92

    t_vals = list(times.values())
    t_lo = 10 ** math.floor(math.log10(min(t_vals)))
    t_hi = 10 ** math.ceil(math.log10(max(t_vals)))

    svg = []
    svg.append(
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}" '
        f'viewBox="0 0 {W} {H}" font-family="-apple-system, \'Segoe UI\', Roboto, sans-serif">'
    )
    svg.append(f'<rect width="{W}" height="{H}" fill="{SURFACE}"/>')
    svg.append(
        f'<text x="24" y="30" font-size="15" font-weight="600" fill="{INK}">'
        f"Four graph operations, one million nodes</text>"
    )
    svg.append(
        f'<text x="24" y="49" font-size="11" fill="{INK2}">'
        f"Barabasi-Albert n=1M, m=5 (5.0M directed edges) · medians of 3 runs · "
        f"log time scale, lower is better</text>"
    )

    # Legend row (identity is also carried by the ink labels below).
    lx = 24
    for lib in LIBS:
        svg.append(f'<circle cx="{lx + 5}" cy="68" r="5" fill="{COLORS[lib]}"/>')
        svg.append(
            f'<text x="{lx + 15}" y="72" font-size="11" fill="{INK2}">{lib}</text>'
        )
        lx += 15 + 8 * len(lib) + 26

    # Time axis decade gridlines, drawn across the op rows.
    plot_bottom = top + row_h * len(OPS)
    d = int(math.log10(t_lo))
    while d <= math.log10(t_hi):
        v = 10.0**d
        x = log_x(v, t_lo, t_hi, x0, x1)
        svg.append(
            f'<line x1="{x:.1f}" y1="{top - 14}" x2="{x:.1f}" y2="{plot_bottom}" stroke="{GRID}"/>'
        )
        svg.append(
            f'<text x="{x:.1f}" y="{plot_bottom + 16}" font-size="10" fill="{MUTED}" '
            f'text-anchor="middle">{fmt_time(v)}</text>'
        )
        d += 1

    for i, op in enumerate(OPS):
        y = top + i * row_h + row_h // 2 - 8
        svg.append(
            f'<text x="{x0 - 12}" y="{y + 4}" font-size="11" fill="{INK2}" '
            f'text-anchor="end">{OP_LABELS[op]}</text>'
        )
        svg.append(
            f'<line x1="{x0}" y1="{y}" x2="{x1}" y2="{y}" stroke="{GRID}" stroke-dasharray="2,3"/>'
        )
        row = [(lib, times[(lib, op)]) for lib in LIBS if (lib, op) in times]
        fastest = min(row, key=lambda t: t[1])
        slowest = max(row, key=lambda t: t[1])
        # Dodge collisions: dots whose centers land within 10px of an
        # already-placed dot get a small vertical offset instead of
        # overprinting it.
        placed = []
        for lib, v in sorted(row, key=lambda t: t[1]):
            x = log_x(v, t_lo, t_hi, x0, x1)
            dy = 0
            while any(abs(x - px) < 10 and pdy == dy for px, pdy in placed):
                dy += 9
            placed.append((x, dy))
            svg.append(
                f'<circle cx="{x:.1f}" cy="{y + dy}" r="6" fill="{COLORS[lib]}" '
                f'stroke="{SURFACE}" stroke-width="2"/>'
            )
            # Selective labels: value on the row extremes only; the table
            # beside the chart carries every number.
            if lib == fastest[0]:
                svg.append(
                    f'<text x="{x - 10:.1f}" y="{y - 10}" font-size="10" fill="{INK2}" '
                    f'text-anchor="start">{fmt_time(v)}</text>'
                )
            elif lib == slowest[0]:
                svg.append(
                    f'<text x="{x:.1f}" y="{y - 10}" font-size="10" fill="{INK2}" '
                    f'text-anchor="end">{fmt_time(v)}</text>'
                )

    # Memory panel: one row, its own log axis.
    my = plot_bottom + 56
    svg.append(
        f'<text x="24" y="{my - 14}" font-size="12" font-weight="600" fill="{INK}">'
        f"Peak memory, same workload</text>"
    )
    # Decade ticks in MB, not bytes, so labels land on 100 MB / 1 GB / ...
    MB = 1 << 20
    m_vals = [v / MB for v in mem.values()]
    m_lo = (10 ** math.floor(math.log10(min(m_vals)))) * MB
    m_hi = (10 ** math.ceil(math.log10(max(m_vals)))) * MB
    yb = my + 22
    d = int(math.log10(m_lo / MB))
    while d <= math.log10(m_hi / MB) + 1e-9:
        v = (10.0**d) * MB
        x = log_x(v, m_lo, m_hi, x0, x1)
        svg.append(f'<line x1="{x:.1f}" y1="{my}" x2="{x:.1f}" y2="{yb + 10}" stroke="{GRID}"/>')
        svg.append(
            f'<text x="{x:.1f}" y="{yb + 26}" font-size="10" fill="{MUTED}" '
            f'text-anchor="middle">{fmt_mem(v)}</text>'
        )
        d += 1
    svg.append(
        f'<text x="{x0 - 12}" y="{yb + 4}" font-size="11" fill="{INK2}" text-anchor="end">peak RSS</text>'
    )
    m_sorted = sorted(mem.items(), key=lambda kv: kv[1])
    for lib, v in mem.items():
        x = log_x(v, m_lo, m_hi, x0, x1)
        svg.append(
            f'<circle cx="{x:.1f}" cy="{yb}" r="6" fill="{COLORS[lib]}" '
            f'stroke="{SURFACE}" stroke-width="2"/>'
        )
        if lib in (m_sorted[0][0], m_sorted[-1][0]):
            anchor = "start" if lib == m_sorted[0][0] else "end"
            dx = -10 if lib == m_sorted[0][0] else 0
            svg.append(
                f'<text x="{x + dx:.1f}" y="{yb - 10}" font-size="10" fill="{INK2}" '
                f'text-anchor="{anchor}">{fmt_mem(v)}</text>'
            )

    svg.append(
        f'<text x="24" y="{H - 12}" font-size="9.5" fill="{MUTED}">'
        f"Whole-process RSS via /usr/bin/time -l (interpreter and runtime included) · "
        f"protocol and caveats: bench/README.md</text>"
    )
    svg.append("</svg>")
    OUT.write_text("\n".join(svg))
    print(f"wrote {OUT}")

    # Median + speedup table for the README.
    print(f"\nmedians at n={n_target:,} (seconds; speedup vs gonx in parens)")
    base = {op: times[("gonx", op)] for op in OPS}
    hdr = "op".ljust(12) + "".join(lib.rjust(22) for lib in LIBS)
    print(hdr)
    for op in OPS:
        cells = []
        for lib in LIBS:
            v = times.get((lib, op))
            if v is None:
                cells.append("-".rjust(22))
            else:
                cells.append(f"{fmt_time(v)} ({v / base[op]:.1f}x)".rjust(22))
        print(op.ljust(12) + "".join(cells))
    print("mem".ljust(12) + "".join(fmt_mem(mem[lib]).rjust(22) for lib in LIBS if lib in mem))


if __name__ == "__main__":
    main()
