"""Confirms that every library computed the same answers, from the #check
lines run.sh collects in results/checks.txt. Exits non-zero, naming each
disagreement, when they differ, so a run whose libraries disagree fails
instead of publishing timings for different work.

Lines are grouped by graph size and by run (the core run reports PageRank,
components and BFS; the Dijkstra run reports shortest paths), and within a
group every field must agree across libraries: exactly for counts and node
IDs, and within a tolerance for the floats, where libraries that compute the
same answer differently can still differ in the last digit printed.

- pr_top_score, the top PageRank score, agrees to a relative 1e-3: the
  libraries stop iterating on different criteria (see env.txt).
- sp_sum, the sum of about n shortest-path distances printed to 3
  decimals, and sp_max, printed to 6, agree to one unit in the last place
  printed (or a relative 1e-9 on large sums): summing in a different order
  moves the rounding.
- pr_top, the top-ranked node, may differ when the top scores agree: two
  hubs whose scores are that close can swap between runs of a library that
  starts PageRank from a random vector, as gonum does.
"""

import sys
from collections import defaultdict

LIBS = {"gonx", "gonum", "networkx", "igraph"}
# field -> (relative, absolute) tolerance; a value passes within either.
# The absolute ones are a unit and a half in the last place printed, since a
# difference of exactly one unit can come out a hair above it in floats.
LOOSE = {"pr_top_score": (1e-3, 0), "sp_sum": (1e-9, 1.5e-3), "sp_max": (1e-9, 1.5e-6)}


def close(key, got, want):
    rel, abs_ = LOOSE[key]
    a, b = float(got), float(want)
    return abs(a - b) <= max(rel * max(abs(a), abs(b)), abs_)


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
                    ok = close(key, got, want)
                elif key == "pr_top" and got != want:
                    ok = close("pr_top_score", fields["pr_top_score"], ref["pr_top_score"])
                    if ok:
                        print(f"note: n={n} {lib} ranks node {got} first, gonx node {want}, "
                              f"with top scores {fields['pr_top_score']} and {ref['pr_top_score']}")
                else:
                    ok = got == want
                if not ok:
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
