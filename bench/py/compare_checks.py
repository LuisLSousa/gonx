"""Confirms that every library computed the same answers, from the #check
lines run.sh collects in results/checks.txt. Exits non-zero, naming each
disagreement, when they differ, so a run whose libraries disagree fails
instead of publishing timings for different work.

Lines are grouped by graph size and by run (the core run reports PageRank,
components and BFS; the Dijkstra run reports shortest paths), and within a
group every field must be equal across libraries. The one exception is
pr_top_score: the libraries stop PageRank on different criteria (see
env.txt), so it must agree to a relative 1e-3 rather than exactly.
"""

import sys
from collections import defaultdict

LIBS = {"gonx", "gonum", "networkx", "igraph"}
LOOSE = {"pr_top_score": 1e-3}


def main(path):
    groups = defaultdict(dict)  # (n, run) -> lib -> fields
    with open(path) as f:
        for line in f:
            parts = line.strip().split(",")
            if not parts or parts[0] != "#check":
                continue
            lib = parts[1]
            fields = dict(p.split("=", 1) for p in parts[2:])
            run = "dijkstra" if "sp_src" in fields else "core"
            groups[(int(fields["n"]), run)][lib] = fields

    problems = []
    for (n, run), libs in sorted(groups.items()):
        if set(libs) != LIBS:
            problems.append(f"n={n} {run}: checks from {sorted(libs)}, want {sorted(LIBS)}")
            continue
        ref = libs["gonx"]
        for lib, fields in sorted(libs.items()):
            if set(fields) != set(ref):
                problems.append(f"n={n} {run}: {lib} reports {sorted(fields)}, gonx {sorted(ref)}")
                continue
            for key, want in ref.items():
                got = fields[key]
                if key in LOOSE:
                    a, b = float(got), float(want)
                    if abs(a - b) > LOOSE[key] * max(abs(a), abs(b)):
                        problems.append(f"n={n} {run}: {lib} {key}={got}, gonx {want}")
                elif got != want:
                    problems.append(f"n={n} {run}: {lib} {key}={got}, gonx {want}")

    if not groups:
        problems.append(f"no #check lines in {path}")
    for p in problems:
        print("check mismatch:", p, file=sys.stderr)
    if problems:
        sys.exit(1)
    print(f"checks agree: {len(groups)} groups x {len(LIBS)} libraries")


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else "results/checks.txt")
