# gonx

A performance-oriented graph library for Go, in the spirit of Python's
[networkx](https://networkx.org/) but built around dense integer node IDs and a
compact, cache-friendly representation.

`gonx` targets workloads that build a graph once and then read it intensively —
agent-based simulations, network metrics, repeated traversals. It separates
mutation from reading: a `Builder` assembles the topology, then freezes into an
immutable `Graph` stored in [Compressed Sparse Row](https://en.wikipedia.org/wiki/Sparse_matrix#Compressed_sparse_row_(CSR,_CRS_or_Yale_format))
form for zero-copy, O(1) neighbor iteration.

```go
import (
    "github.com/LuisLSousa/gonx"
    "github.com/LuisLSousa/gonx/generators"
    "github.com/LuisLSousa/gonx/metrics"
)

r := gonx.NewRand(42)                                  // reproducible PCG RNG
g, _ := generators.WattsStrogatz(1000, 8, 0.1, r)      // small-world graph

apl, _ := metrics.AveragePathLength(g)                 // parallel all-pairs BFS
cc := metrics.Transitivity(g)                          // global clustering

for u := range g.Nodes() {
    for _, v := range g.Neighbors(u) {                 // zero-alloc, cache-friendly
        _ = v
    }
}
```

## Design

- **Dense integer nodes.** IDs are `0..N-1`. No generic node types in the core —
  that would reintroduce a map indirection and defeat CSR's locality. A labeled
  wrapper can sit on top if needed.
- **Immutable `Graph` (CSR).** Neighbors of `u` live in a contiguous, sorted slice
  returned zero-copy by `Neighbors(u)`. `HasEdge` is O(log deg) via binary search.
- **Mutable `Builder`.** Add/remove nodes and edges, then `Build()` to freeze.
  The result is always simple (no self-loops or duplicate edges).
- **Reproducible randomness.** Every randomized operation takes an explicit
  `*math/rand/v2.Rand`. Same seed + parameters ⇒ byte-identical graph. The package
  never touches a global RNG.
- **Parallel where it pays.** All-pairs shortest paths and triangle counting are
  parallelized over independent source nodes with order-independent reductions, so
  results are deterministic regardless of `GOMAXPROCS`. Generators and edge swaps
  are inherently sequential and kept single-threaded.

## Packages

| Package | Contents |
|---|---|
| `gonx` | `Graph` (CSR), `Builder`, iterators, `NewRand` |
| `gonx/generators` | `WattsStrogatz`, `BarabasiAlbert`, `Complete`, `RandomAvgDegree`, `ErdosRenyi` |
| `gonx/transform` | `DoubleEdgeSwap`, `RelabelNodes`, `Shuffle`, `Copy` |
| `gonx/metrics` | `Transitivity`, `AverageClustering`, `AveragePathLength`(+`LCC`), `Diameter`, `ConnectedComponents`, `IsConnected`, `BFS` |

> Note on `BarabasiAlbert(n, m, r)`: `m` is the number of edges added per new node
> (matching networkx), **not** the average degree — the resulting average degree
> is approximately `2m`.

## Status

v1 focuses on undirected, unweighted graphs. Directed/weighted graphs, generic
node labels, serialization, and advanced algorithms (centralities, community
detection) are intentionally out of scope for now.

## Testing

```sh
go test ./... -race        # unit, property, determinism, known-value tests
go test -bench . -benchmem  # benchmarks
go test -fuzz FuzzDoubleEdgeSwap ./transform   # degree-preservation fuzzing
```

## License

MIT — see [LICENSE](LICENSE).
