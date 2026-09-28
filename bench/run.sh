#!/usr/bin/env bash
# Runs the full benchmark suite: every library on the same seeded
# Barabasi-Albert edge lists, each (library, size) pair in its own
# process under /usr/bin/time -l so peak RSS is captured per run.
#
# Outputs:
#   results/results.csv   lib,op,n,edges,repeat,seconds (+ mem rows in bytes)
#   results/checks.txt    per-library #check lines for cross-validation
#   results/env.txt       hardware, OS, toolchain, library versions
set -euo pipefail
cd "$(dirname "$0")"

SIZES="${SIZES:-10000 100000 1000000}"
M=5
SEED=42
REPEATS="${REPEATS:-3}"

mkdir -p data results
: > results/results.csv
: > results/checks.txt

echo "== building runners =="
go build -o bin/gen ./cmd/gen
go build -o bin/gonxbench ./cmd/gonxbench
go build -o bin/gonumbench ./cmd/gonumbench

PY=.venv/bin/python

run_one() { # lib n memop cmd...
  local lib="$1" n="$2" memop="$3"
  shift 3
  local tmpout tmptime
  tmpout=$(mktemp) tmptime=$(mktemp)
  /usr/bin/time -l "$@" > "$tmpout" 2> "$tmptime"
  grep -v '^#' "$tmpout" >> results/results.csv
  grep '^#check' "$tmpout" >> results/checks.txt || true
  local rss
  rss=$(grep "maximum resident set size" "$tmptime" | awk '{print $1}')
  echo "$lib,$memop,$n,0,0,$rss" >> results/results.csv
  rm -f "$tmpout" "$tmptime"
}

for n in $SIZES; do
  edges_file="data/ba-$n-m$M-seed$SEED.txt"
  if [ ! -f "$edges_file" ]; then
    echo "== generating $edges_file =="
    ./bin/gen -n "$n" -m "$M" -seed "$SEED" -out "$edges_file"
  fi
  # The core ops share one process per library, so "mem" is the peak of
  # that run; Dijkstra needs a weighted graph and runs separately, with its
  # own peak recorded as "mem_dijkstra".
  for op in core dijkstra; do
    memop=mem
    [ "$op" = dijkstra ] && memop=mem_dijkstra
    for lib in gonx gonum networkx igraph; do
      echo "== $lib $op n=$n =="
      case "$lib" in
        gonx)     run_one gonx "$n" "$memop" ./bin/gonxbench -in "$edges_file" -repeats "$REPEATS" -op "$op" ;;
        gonum)    run_one gonum "$n" "$memop" ./bin/gonumbench -in "$edges_file" -repeats "$REPEATS" -op "$op" ;;
        networkx) run_one networkx "$n" "$memop" "$PY" py/bench.py --in "$edges_file" --lib networkx --repeats "$REPEATS" --op "$op" ;;
        igraph)   run_one igraph "$n" "$memop" "$PY" py/bench.py --in "$edges_file" --lib igraph --repeats "$REPEATS" --op "$op" ;;
      esac
    done
  done
done

{
  echo "date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "host: $(sysctl -n machdep.cpu.brand_string), $(sysctl -n hw.ncpu) cores, $(($(sysctl -n hw.memsize) / 1073741824)) GB"
  echo "os: $(sw_vers -productName) $(sw_vers -productVersion)"
  echo "go: $(go version)"
  echo "gonx: $(cd .. && git rev-parse --short HEAD) (module: replace ../)"
  echo "python: $($PY --version 2>&1)"
  "$PY" -m pip freeze | grep -Ei "networkx|igraph|scipy|numpy"
  echo "protocol: BA(n, m=$M, seed=$SEED) low->high oriented; repeats=$REPEATS;"
  echo "  pagerank damping=0.85; gonx+networkx tol=1e-10 (n*tol L1 rule) max_iter=200;"
  echo "  gonum tol=1e-6 (absolute L1); igraph=PRPACK direct solver (no tol);"
  echo "  dijkstra: weight(u,v) = 1 + ((u*7919 + v*104729) mod 1000)/1000, source = highest out-degree node;"
  echo "  timings exclude file parsing; peak RSS per whole process via /usr/bin/time -l"
} > results/env.txt

echo "done -> results/"
