#!/bin/sh
# Measurement 4: drive one fan-out to completion under a wall-clock budget.
#
#   run-batch.sh <prompts.json> <model> [budget-minutes] [passes]
#
# Three things this does that a bare `for` loop did not, each because the bare
# loop failed at it:
#
#   1. Every session is capped by run-roles.sh, so no single one can wait out a
#      five-hour usage window. The 76-session pass this replaces took 46 hours,
#      45 of them inside one session.
#   2. The whole pass is capped too. When the budget is spent the driver stops
#      and says what is left, rather than running until somebody asks.
#   3. It is resumable and it retries. A session already in usage.txt is
#      skipped, so re-running costs nothing; a session killed at the cap is
#      retried on the next pass, which is where a usage window that has since
#      turned gets its second chance.
#
# Progress goes to progress.txt with a timestamp on every line, so "how long
# has this been running" is answered by reading one file rather than by
# counting outputs and guessing.
set -u
M=$(cd "$(dirname "$0")" && pwd)
PROMPTS=$1; MODEL=$2; BUDGET=${3:-120}; PASSES=${4:-3}
LOG=$M/role-logs; mkdir -p "$LOG"
PROGRESS=$M/progress.txt
say() { printf '%s %s\n' "$(date -u +%H:%M:%S)" "$*" >> "$PROGRESS"; }

total=$(python3 -c 'import json,sys; print(len(json.load(open(sys.argv[1]))["prompts"]))' "$PROMPTS")
deadline=$(( $(date +%s) + BUDGET * 60 ))
say "batch start: $total prompts, budget ${BUDGET}m, $PASSES pass(es), model $MODEL"

pass=1
while [ "$pass" -le "$PASSES" ]; do
  left=0
  i=0
  while [ "$i" -lt "$total" ]; do
    tag=$(python3 -c 'import json,sys; p=json.load(open(sys.argv[1]))["prompts"][int(sys.argv[2])]; print(p["unit"]+"-"+p["role"])' "$PROMPTS" "$i")
    if [ -f "$LOG/usage.txt" ] && grep -q "^$tag " "$LOG/usage.txt"; then
      i=$((i + 1)); continue
    fi
    if [ "$(date +%s)" -ge "$deadline" ]; then
      say "budget spent; stopping with work left"
      say "done $(grep -c . "$LOG/usage.txt" 2>/dev/null || echo 0) of $total"
      exit 3
    fi
    "$M/run-roles.sh" "$PROMPTS" "$i" "$MODEL" >> "$M/batch.log" 2>&1
    rc=$?
    if [ "$rc" -eq 124 ]; then
      left=$((left + 1))
      say "stalled $tag (pass $pass); will retry"
    fi
    say "done $(grep -c . "$LOG/usage.txt" 2>/dev/null || echo 0) of $total after $tag"
    i=$((i + 1))
  done
  finished=$(grep -c . "$LOG/usage.txt" 2>/dev/null || echo 0)
  if [ "$finished" -ge "$total" ]; then
    say "batch complete: $finished of $total"
    exit 0
  fi
  say "pass $pass ended with $left stalled; $finished of $total done"
  pass=$((pass + 1))
done
say "passes exhausted: $(grep -c . "$LOG/usage.txt" 2>/dev/null || echo 0) of $total"
exit 4
