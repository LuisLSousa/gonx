"""Times networkx or igraph on the shared edge list, mirroring the Go
runners' protocol: directed build, PageRank (damping 0.85; networkx uses
tol=1e-6 with its n*tol L1 stopping rule, igraph uses PRPACK which is a
direct solver and takes no tolerance -- noted in the results), weakly
connected components, and reachability over out-edges from the highest
out-degree node.

With --op dijkstra it instead times single-source Dijkstra from that node
over edge_weight, in a process of its own so the weighted copy of the graph
does not inflate the peak memory of the core run.

Edge parsing happens before any timing starts. Output rows match the Go
runners: "lib,op,n,edges,repeat,seconds" plus a #check line.
"""

import argparse
import time


def read_edges(path):
    us, vs = [], []
    n = 0
    with open(path) as f:
        for line in f:
            a, b = line.split()
            u, v = int(a), int(b)
            us.append(u)
            vs.append(v)
            if u >= n:
                n = u + 1
            if v >= n:
                n = v + 1
    return us, vs, n


def edge_weight(u, v):
    """Matches the Go runners: the same integer hash mapped to [1, 2)."""
    return 1 + ((u * 7919 + v * 104729) % 1000) / 1000


def check_sp(lib, n, edges, src, dists):
    finite = [d for d in dists if d != float("inf")]
    print(
        f"#check,{lib},n={n},edges={edges},sp_src={src},sp_reached={len(finite)},"
        f"sp_sum={sum(finite):.3f},sp_max={max(finite):.6f}",
        flush=True,
    )


def dijkstra_networkx(us, vs, n, repeats):
    import networkx as nx

    edges = len(us)
    G = nx.DiGraph()
    G.add_nodes_from(range(n))
    G.add_weighted_edges_from((u, v, edge_weight(u, v)) for u, v in zip(us, vs))
    src = max(range(n), key=lambda u: G.out_degree(u))
    dist = None
    for i in range(repeats):
        start = time.perf_counter()
        dist = nx.single_source_dijkstra_path_length(G, src, weight="weight")
        emit("networkx", "dijkstra", n, edges, i, time.perf_counter() - start)
    check_sp("networkx", n, edges, src, dist.values())


def dijkstra_igraph(us, vs, n, repeats):
    import igraph as ig

    edges = len(us)
    G = ig.Graph(n=n, edges=list(zip(us, vs)), directed=True)
    G.es["weight"] = [edge_weight(u, v) for u, v in zip(us, vs)]
    src = max(range(n), key=lambda u: G.degree(u, mode="out"))
    dist = None
    for i in range(repeats):
        start = time.perf_counter()
        dist = G.distances(source=src, weights="weight", mode="out", algorithm="dijkstra")[0]
        emit("igraph", "dijkstra", n, edges, i, time.perf_counter() - start)
    check_sp("igraph", n, edges, src, dist)


def emit(lib, op, n, edges, repeat, seconds):
    print(f"{lib},{op},{n},{edges},{repeat},{seconds:.6f}", flush=True)


def bench_networkx(us, vs, n, repeats):
    import networkx as nx

    edges = len(us)
    edge_list = list(zip(us, vs))  # built outside timing

    G = None
    for i in range(repeats):
        start = time.perf_counter()
        G = nx.DiGraph()
        G.add_nodes_from(range(n))
        G.add_edges_from(edge_list)
        emit("networkx", "build", n, edges, i, time.perf_counter() - start)

    rank = None
    for i in range(repeats):
        start = time.perf_counter()
        # tol=1e-10 rather than the 1e-6 default: networkx stops when the
        # L1 delta drops below n*tol, and at n=1M the default makes that
        # threshold 1.0 (a couple of iterations). See bench/README.md.
        rank = nx.pagerank(G, alpha=0.85, tol=1e-10, max_iter=200)
        emit("networkx", "pagerank", n, edges, i, time.perf_counter() - start)
    top = max(rank, key=rank.get)

    comps = None
    for i in range(repeats):
        start = time.perf_counter()
        comps = list(nx.weakly_connected_components(G))
        emit("networkx", "wcc", n, edges, i, time.perf_counter() - start)

    src = max(range(n), key=lambda u: G.out_degree(u))
    reached = 0
    for i in range(repeats):
        start = time.perf_counter()
        reached = len(nx.descendants(G, src)) + 1
        emit("networkx", "bfs", n, edges, i, time.perf_counter() - start)

    print(
        f"#check,networkx,n={n},edges={edges},pr_top={top},"
        f"pr_top_score={rank[top]:.9f},wcc={len(comps)},bfs_src={src},bfs_reached={reached}",
        flush=True,
    )


def bench_igraph(us, vs, n, repeats):
    import igraph as ig

    edges = len(us)
    edge_list = list(zip(us, vs))  # built outside timing

    G = None
    for i in range(repeats):
        start = time.perf_counter()
        G = ig.Graph(n=n, edges=edge_list, directed=True)
        emit("igraph", "build", n, edges, i, time.perf_counter() - start)

    rank = None
    for i in range(repeats):
        start = time.perf_counter()
        rank = G.pagerank(damping=0.85)  # PRPACK direct solver, no tol knob
        emit("igraph", "pagerank", n, edges, i, time.perf_counter() - start)
    top = max(range(n), key=lambda v: rank[v])

    # Each repeat runs on a copy of G made outside the timing: igraph caches
    # whether a graph is connected, so a second call on the same object
    # answers from the cache in a tenth of the time instead of computing the
    # components. Copies carry the cache too, which is why G itself is never
    # asked. Copying is cheaper in memory than rebuilding from edge_list, and
    # the result, which holds a reference to its graph, goes with the copy, so
    # the process peaks no higher than without the copies.
    ncomps = 0
    for i in range(repeats):
        H = G.copy()
        start = time.perf_counter()
        comps = H.connected_components(mode="weak")
        emit("igraph", "wcc", n, edges, i, time.perf_counter() - start)
        ncomps = len(comps)
        del comps, H

    src = max(range(n), key=lambda u: G.degree(u, mode="out"))
    reached = 0
    for i in range(repeats):
        start = time.perf_counter()
        reached = len(G.subcomponent(src, mode="out"))
        emit("igraph", "bfs", n, edges, i, time.perf_counter() - start)

    print(
        f"#check,igraph,n={n},edges={edges},pr_top={top},"
        f"pr_top_score={rank[top]:.9f},wcc={ncomps},bfs_src={src},bfs_reached={reached}",
        flush=True,
    )


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--in", dest="path", required=True)
    ap.add_argument("--lib", choices=["networkx", "igraph"], required=True)
    ap.add_argument("--repeats", type=int, default=3)
    ap.add_argument("--op", choices=["core", "dijkstra"], default="core")
    args = ap.parse_args()

    us, vs, n = read_edges(args.path)
    if args.op == "dijkstra":
        run = dijkstra_networkx if args.lib == "networkx" else dijkstra_igraph
        run(us, vs, n, args.repeats)
    elif args.lib == "networkx":
        bench_networkx(us, vs, n, args.repeats)
    else:
        bench_igraph(us, vs, n, args.repeats)


if __name__ == "__main__":
    main()
