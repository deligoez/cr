#!/usr/bin/env bash
# Run gremlins one internal package at a time, each with its own -o file.
#
# Why per package, measured 2026-09-13 (CLAUDE.md, "Tooling beyond the gate",
# rule 7): a whole-tree run was killed for low memory at ~45 minutes, wrote no
# result file, and its log held four false LIVED results from the minutes before
# the kill. Per package, a kill loses one package; a rerun skips every package
# whose result file already exists.
#
# Usage: scripts/mutation-run.sh <out-dir>
#   then merge: scripts/mutation-merge.py <out-dir> <merged.json>
set -u
out=${1:?usage: scripts/mutation-run.sh <out-dir>}
mkdir -p "$out"
cd "$(dirname "$0")/.."
go clean -testcache
pkgs=$(cd internal && ls -d */ | tr -d / | grep -v '^cli$')
for p in $pkgs cli; do
  if [ -s "$out/$p.json" ]; then echo "skip $p (done)"; continue; fi
  # internal/cli's suite is tens of seconds and spawns processes per test, so it
  # runs at two workers and the tree coefficient; every other package is
  # sub-second, where coefficient 5 reports the instrument's own timeouts.
  if [ "$p" = cli ]; then flags="--workers 2 --timeout-coefficient 5"; else flags="--workers 4"; fi
  start=$(date +%s)
  gremlins unleash $flags -o "$out/$p.json" "./internal/$p" > "$out/$p.log" 2>&1
  rc=$?
  swap=$(sysctl -n vm.swapusage 2>/dev/null | awk '{print $6}')
  echo "$p rc=$rc secs=$(( $(date +%s) - start )) swap_used=$swap $(grep -m1 -i 'gathering' "$out/$p.log" | sed 's/.*done in/gathering/')"
done
echo ALLDONE
