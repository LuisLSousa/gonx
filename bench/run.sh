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

run_one() { # lib cmd...
  local lib="$1" n="$2"
  shift 2
  local tmpout tmptime
  tmpout=$(mktemp) tmptime=$(mktemp)
  /usr/bin/time -l "$@" > "$tmpout" 2> "$tmptime"
  grep -v '^#' "$tmpout" >> results/results.csv
  grep '^#check' "$tmpout" >> results/checks.txt || true
  local rss
  rss=$(grep "maximum resident set size" "$tmptime" | awk '{print $1}')
  echo "$lib,mem,$n,0,0,$rss" >> results/results.csv
  rm -f "$tmpout" "$tmptime"
}

for n in $SIZES; do
  edges_file="data/ba-$n-m$M-seed$SEED.txt"
  if [ ! -f "$edges_file" ]; then
    echo "== generating $edges_file =="
    ./bin/gen -n "$n" -m "$M" -seed "$SEED" -out "$edges_file"
  fi
  for lib in gonx gonum networkx igraph; do
    echo "== $lib n=$n =="
    case "$lib" in
      gonx)     run_one gonx "$n" ./bin/gonxbench -in "$edges_file" -repeats "$REPEATS" ;;
      gonum)    run_one gonum "$n" ./bin/gonumbench -in "$edges_file" -repeats "$REPEATS" ;;
      networkx) run_one networkx "$n" "$PY" py/bench.py --in "$edges_file" --lib networkx --repeats "$REPEATS" ;;
      igraph)   run_one igraph "$n" "$PY" py/bench.py --in "$edges_file" --lib igraph --repeats "$REPEATS" ;;
    esac
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
  echo "  timings exclude file parsing; peak RSS per whole process via /usr/bin/time -l"
} > results/env.txt

echo "done -> results/"
