#!/bin/bash
# Independent hand-verification of measurement 4 Part B's probed records.
# Each stored mutation patch is applied to a copy and mapped over the original
# with go test -overlay; a green suite confirms cr's "no-test-failed".
set -u
H="$(cd "$(dirname "$0")" && pwd)"
CLONE="$(cd "$H/../../clone" && pwd)"
OUT="$H/overlay-results.txt"
: > "$OUT"
for p in $(python3 -c 'import json,sys;print(" ".join(r["probe"] for r in json.load(open(sys.argv[1]))))' "$H/applied.json"); do
  map="$H/apply/$p/overlay.json"
  start=$(date +%s)
  cd "$CLONE" || exit 1
  log="$H/apply/$p/gotest.log"
  GOFLAGS="-overlay=$map" go test -count=1 -overlay="$map" ./... > "$log" 2>&1
  rc=$?
  fails=$(grep -c '^FAIL' "$log")
  printf '%s rc=%s FAIL_lines=%s secs=%s\n' "$p" "$rc" "$fails" "$(( $(date +%s) - start ))" >> "$OUT"
done
echo DONE >> "$OUT"
