#!/bin/sh
# Measurement 4, part A: the pull request as its reviewer saw it.
#
# Three layers over the real gh:
#   1. §3.5.1's thread query is answered empty — the human review IS the ground
#      truth, and cr attaches ingested threads to the units it reviews.
#   2. the pull-request query's answer has headRefOid rewritten to the commit
#      the reviewer commented on (CR_M3_HEAD) and baseRefOid to that commit's
#      merge base (CR_M3_BASE), so the diff cr forms is the diff the human read
#      rather than the merged result of their review.
#   3. any write is refused.
# Every invocation is logged with its verdict.
# cr's gh runner inherits an allowlist of environment variables (PATH, HOME,
# TMPDIR and its own pinned values), so a shim cannot be configured through the
# environment: it reads its settings from files at absolute paths, as the QA
# harness recorded.
M3=/private/tmp/claude-501/-Users-deligoez-Developer-deligoez-projects-cr/ab43de41-7948-4a07-bc9b-b5622c5ddeda/scratchpad/measure-4
LOG=$M3/gh.log
REAL=/opt/homebrew/bin/gh
HEAD=$(sed -n 1p "$M3/commits.txt")
BASE=$(sed -n 2p "$M3/commits.txt")

for a in "$@"; do
  case "$a" in
    *reviewThreads*)
      printf '%s BLANKED-threads\n' "$(date -u +%H:%M:%S)" >> "$LOG"
      printf '%s' '{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}}}}}'
      exit 0
      ;;
  esac
done

for a in "$@"; do
  case "$a" in
    --method|-X|--input)
      printf '%s REFUSED-WRITE %s\n' "$(date -u +%H:%M:%S)" "$*" >> "$LOG"
      echo "measurement 4 refuses a write: $*" >&2
      exit 1
      ;;
  esac
done

answer=$("$REAL" "$@")
status=$?
if [ $status -ne 0 ]; then
  printf '%s failed-passthrough\n' "$(date -u +%H:%M:%S)" >> "$LOG"
  printf '%s' "$answer"
  exit $status
fi

case "$answer" in
  *headRefOid*)
    printf '%s REWROTE-head\n' "$(date -u +%H:%M:%S)" >> "$LOG"
    printf '%s' "$answer" | HEAD_OID="$HEAD" BASE_OID="$BASE" python3 -c '
import json,os,sys
d=json.load(sys.stdin)
pr=d.get("data",{}).get("repository",{}).get("pullRequest")
if pr:
    pr["headRefOid"]=os.environ["HEAD_OID"]
    pr["baseRefOid"]=os.environ["BASE_OID"]
json.dump(d,sys.stdout)'
    exit 0
    ;;
esac

printf '%s passed\n' "$(date -u +%H:%M:%S)" >> "$LOG"
printf '%s' "$answer"
