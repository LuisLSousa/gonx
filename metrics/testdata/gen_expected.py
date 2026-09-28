"""Compute networkx reference answers for the edge lists next to this file.

Run after gen_edges.go, from anywhere:

    python3 metrics/testdata/gen_expected.py

Writes <name>.dijkstra.json: for each of a fixed set of sources, the
single-source shortest-path lengths as a dense list indexed by node, with -1
for unreachable nodes, and <name>.restricted.json: the same over
restricted_view with a few nodes and edges hidden, along with what was hidden.
For undirected graphs it also writes <name>.cuts.json:
the articulation points, and every bridge as [u, v, side], oriented so that v
is on the side without the component's smallest node and side counts the
nodes there. Requires networkx (the bench/ virtualenv has it).
"""
import json
import pathlib

import networkx as nx

HERE = pathlib.Path(__file__).parent
SOURCES = [0, 7, 42]
HIDDEN_NODES = [3, 11]  # not among SOURCES: networkx has no path from a hidden node


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
    # Every ninth edge, and on an undirected graph every other one of those
    # named backwards, since either spelling must hide the same edge.
    hidden = [list(e) for e in list(g.edges())[::9]]
    if not g.is_directed():
        for e in hidden[::2]:
            e.reverse()
    view = nx.restricted_view(g, HIDDEN_NODES, [tuple(e) for e in hidden])
    rout = {}
    for s in SOURCES:
        dist = nx.single_source_dijkstra_path_length(view, s)
        rout[str(s)] = [dist.get(v, -1) for v in range(g.number_of_nodes())]
    with open(path.with_suffix(".restricted.json"), "w") as f:
        json.dump({"nodes": HIDDEN_NODES, "edges": hidden, "sources": rout}, f, indent=0)
    print(f"{path.stem}.restricted.json: {len(hidden)} edges and {len(HIDDEN_NODES)} nodes hidden")
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
