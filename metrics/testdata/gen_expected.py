"""Compute networkx reference answers for the edge lists next to this file.

Run after gen_edges.go, from anywhere:

    python3 metrics/testdata/gen_expected.py

Writes <name>.dijkstra.json: for each of a fixed set of sources, the
single-source shortest-path lengths as a dense list indexed by node, with -1
for unreachable nodes. For undirected graphs it also writes <name>.cuts.json:
the articulation points, and every bridge as [u, v, side], oriented so that v
is on the side without the component's smallest node and side counts the
nodes there. Requires networkx (the bench/ virtualenv has it).
"""
import json
import pathlib

import networkx as nx

HERE = pathlib.Path(__file__).parent
SOURCES = [0, 7, 42]


def load(path):
    with open(path) as f:
        kind, n = f.readline().split()
        g = nx.DiGraph() if kind == "directed" else nx.Graph()
        g.add_nodes_from(range(int(n)))
        for line in f:
            u, v, w = line.split()
            g.add_edge(int(u), int(v), weight=float(w))
    return g


for path in sorted(HERE.glob("*.edges")):
    g = load(path)
    out = {}
    for s in SOURCES:
        dist = nx.single_source_dijkstra_path_length(g, s)
        out[str(s)] = [dist.get(v, -1) for v in range(g.number_of_nodes())]
    target = path.with_suffix(".dijkstra.json")
    with open(target, "w") as f:
        json.dump({"sources": out}, f, indent=0)
    if not g.is_directed():
        bridges = []
        for a, b in nx.bridges(g):
            h = g.copy()
            h.remove_edge(a, b)
            side_a = nx.node_connected_component(h, a)
            smallest = min(side_a | nx.node_connected_component(h, b))
            u, v = (a, b) if smallest in side_a else (b, a)
            bridges.append([u, v, len(nx.node_connected_component(h, v))])
        bridges.sort(key=lambda t: t[1])
        cuts = {"articulation_points": sorted(nx.articulation_points(g)), "bridges": bridges}
        with open(path.with_suffix(".cuts.json"), "w") as f:
            json.dump(cuts, f, indent=0)
        print(f"{path.stem}.cuts.json: {len(bridges)} bridges, {len(cuts['articulation_points'])} articulation points")
    zeros = sum(1 for _, _, d in g.edges(data=True) if d["weight"] == 0)
    print(f"{target.name}: {g.number_of_nodes()} nodes, {g.number_of_edges()} edges, {zeros} zero-weight")
