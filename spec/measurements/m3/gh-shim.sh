#!/bin/sh
# Measurement 3's gh: the real gh for everything except §3.5.1's thread
# ingestion, which is answered with an empty page.
#
# The human review IS the ground truth of this measurement, and cr attaches
# ingested threads to the units it reviews (§3.5.3), so an unshimmed run hands
# every role the answer key. The query cr sends for threads is the only one
# naming `reviewThreads`; every other read (the pull request, its reviews, the
# REST calls) passes through untouched. Every invocation is logged with its
# verdict, so the transcript proves which calls were answered by the shim.
LOG=${CR_M3_GH_LOG:-/tmp/m3-gh.log}
REAL=${CR_M3_GH_REAL:-/opt/homebrew/bin/gh}

for a in "$@"; do
  case "$a" in
    *reviewThreads*)
      printf '%s BLANKED %s\n' "$(date -u +%H:%M:%S)" "$*" >> "$LOG"
      printf '%s' '{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}}}}}'
      exit 0
      ;;
  esac
done

# A write must never reach GitHub from this measurement, whatever cr intends.
for a in "$@"; do
  case "$a" in
    --method|-X|--input)
      printf '%s REFUSED-WRITE %s\n' "$(date -u +%H:%M:%S)" "$*" >> "$LOG"
      echo "measurement 3 refuses a write: $*" >&2
      exit 1
      ;;
  esac
done

printf '%s passed %s\n' "$(date -u +%H:%M:%S)" "$*" >> "$LOG"
exec "$REAL" "$@"
