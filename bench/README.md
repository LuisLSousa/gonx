# gonx benchmark receipts

Reproducible comparison of gonx against networkx, igraph, and
gonum/graph on identical inputs. Every number in gonx's README comes
from this harness; run it yourself:

```sh
cd bench
python3 -m venv .venv && .venv/bin/pip install networkx python-igraph scipy
./run.sh                      # full suite: n = 10k / 100k / 1M, 3 repeats
SIZES="10000" REPEATS=1 ./run.sh   # quick smoke run
```

Results land in `results/`: raw timings (`results.csv`), cross-library
answer checks (`checks.txt`), and the exact environment (`env.txt`).

## What is measured

Four operations, on seeded Barabasi-Albert graphs (m = 5, seed 42)
written once as an edge-list file that every library loads:

| op | definition |
|---|---|
| build | edge arrays (already parsed, in memory) -> the library's native directed structure |
| pagerank | damping 0.85; converged scores (see fairness notes) |
| wcc | weakly connected components, fully materialized |
| bfs | reachability over out-edges from the highest out-degree node |

Each (library, size) pair runs as its own process under
`/usr/bin/time -l`, which gives peak RSS for the whole process —
interpreter and runtime overhead included, because that is what a user
actually pays. File parsing is excluded from every timed section.

## Fairness notes

- **networkx** runs with scipy installed, which is its fast path
  (`nx.pagerank` is scipy-backed and refuses to run without it).
- **igraph** PageRank uses PRPACK, a direct solver with no tolerance
  parameter; it is a different algorithm class and is reported as such.
- **gonum** uses `network.PageRankSparse`; the dense `network.PageRank`
  builds an n x n matrix and cannot fit n = 1M in memory. WCC uses the
  `graph.Undirect` adapter over `topo.ConnectedComponents`.
- **gonx** BFS is a ~15-line loop over `OutNeighbors` in this harness
  (`cmd/gonxbench`), since the metrics package has no directed BFS
  helper; the loop is idiomatic use of the public API and the code is
  right there to read.
- **PageRank stopping rules differ by library family and are pinned
  deliberately.** gonx and networkx share the rule L1 delta < n * tol;
  the harness sets tol = 1e-10 (not the networkx default of 1e-6,
  which at n = 1M makes the threshold 1.0 and stops after a couple of
  iterations — a trap this harness's own cross-checks caught). gonum's
  tol is an absolute L1 threshold and stays at 1e-6, a comparable
  convergence depth. With these settings gonx and networkx scores agree
  to ~9 decimals in `checks.txt`, and all four libraries agree on the
  top-ranked node, component count, and BFS reach for every input.

## Caveats

- Wall-clock numbers were captured on a laptop under light interactive
  use; treat small deltas as noise. The published numbers come from a
  quiet re-run (see `results/env.txt` for the exact conditions of the
  run you are looking at).
- BA graphs are a single topology; ratios on your graph will differ.
- This measures four operations, not the libraries' full breadth.
  networkx in particular trades speed for an enormous algorithm
  catalog, pure-Python hackability, and maturity; the comparison here
  is about what a compiled CSR core buys you, not about which library
  to love.
