"""Times networkx or igraph on the shared edge list, mirroring the Go
runners' protocol: directed build, PageRank (damping 0.85; networkx uses
tol=1e-6 with its n*tol L1 stopping rule, igraph uses PRPACK which is a
direct solver and takes no tolerance -- noted in the results), weakly
connected components, and reachability over out-edges from the highest
out-degree node.

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

    comps = None
    for i in range(repeats):
        start = time.perf_counter()
        comps = G.connected_components(mode="weak")
        emit("igraph", "wcc", n, edges, i, time.perf_counter() - start)

    src = max(range(n), key=lambda u: G.degree(u, mode="out"))
    reached = 0
    for i in range(repeats):
        start = time.perf_counter()
        reached = len(G.subcomponent(src, mode="out"))
        emit("igraph", "bfs", n, edges, i, time.perf_counter() - start)

    print(
        f"#check,igraph,n={n},edges={edges},pr_top={top},"
        f"pr_top_score={rank[top]:.9f},wcc={len(comps)},bfs_src={src},bfs_reached={reached}",
        flush=True,
    )


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--in", dest="path", required=True)
    ap.add_argument("--lib", choices=["networkx", "igraph"], required=True)
    ap.add_argument("--repeats", type=int, default=3)
    args = ap.parse_args()

    us, vs, n = read_edges(args.path)
    if args.lib == "networkx":
        bench_networkx(us, vs, n, args.repeats)
    else:
        bench_igraph(us, vs, n, args.repeats)


if __name__ == "__main__":
    main()
