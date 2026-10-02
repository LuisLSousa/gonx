"""Confirms that every library computed the same answers, from the #check
lines the runners print and run.sh collects into one file. Exits non-zero,
naming each disagreement, when they differ, so a run whose libraries
disagree fails instead of publishing timings for different work.

Lines are grouped by graph size and by run (the core run reports PageRank,
components and BFS; the Dijkstra run reports shortest paths), and within a
group every field must equal gonx's, with two exceptions:

- The PageRank scores, pr_top_score and pr_second_score, agree to a relative
  1e-3, since the libraries stop iterating on different criteria (see
  env.txt).
- gonum, whose PageRank starts from a random vector, may report the top two
  nodes in the opposite order, but only when its own two scores are that
  close, so that a swap between near-tied hubs passes and a wrong node does
  not. Each node's score is then compared with gonx's score for that node.

Shortest-path totals need no tolerance: the runners report them as integer
thousandths, which every distance is (see edge_weight), so they are exact.
"""

import math
import sys
from collections import defaultdict

LIBS = {"gonx", "gonum", "networkx", "igraph"}
SCORES = {"pr_top_score", "pr_second_score"}
SCORE_TOL = 1e-3
# Libraries whose PageRank result varies between runs.
NONDETERMINISTIC = {"gonum"}


def close(a, b):
    return math.isclose(float(a), float(b), rel_tol=SCORE_TOL)


def swapped(fields, ref):
    """fields with its top two exchanged, when they are gonx's top two in the
    opposite order and sit as close as the scores are checked to; otherwise
    None."""
    if not (fields["pr_top"] == ref["pr_second"] and fields["pr_second"] == ref["pr_top"]
            and close(fields["pr_top_score"], fields["pr_second_score"])):
        return None
    out = dict(fields)
    out["pr_top"], out["pr_second"] = fields["pr_second"], fields["pr_top"]
    out["pr_top_score"], out["pr_second_score"] = fields["pr_second_score"], fields["pr_top_score"]
    return out


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
            if run == "core" and lib in NONDETERMINISTIC and fields["pr_top"] != ref["pr_top"]:
                if (s := swapped(fields, ref)) is not None:
                    print(f"note: n={n} {lib} ranks nodes {fields['pr_top']} and {fields['pr_second']} "
                          f"in the opposite order to gonx, at scores {fields['pr_top_score']} and "
                          f"{fields['pr_second_score']}")
                    fields = s
            for key, want in ref.items():
                got = fields[key]
                if not (close(got, want) if key in SCORES else got == want):
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
