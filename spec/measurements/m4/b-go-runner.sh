#!/bin/sh
# go test runner for the measurement's `go` profile: runs `go test -v -count=1`
# with the arguments cr passes (-run <filter>, ./<path>...), then prints one
# summary line cr's count patterns read. Exit status is go test's.
#
# cr passes the filter as `-run <expr>` (tests.filter_flag) and each --path
# through tests.paths_arg as `./<path>`; with no path, the whole module.
set -u
haspath=0
for a in "$@"; do case "$a" in ./*) haspath=1;; esac; done
if [ "$haspath" -eq 1 ]; then set -- "$@"; else set -- "$@" ./...; fi
out=$(mktemp)
go test -v -count=1 "$@" > "$out" 2>&1
status=$?
cat "$out"
ran=$(grep -c '^\(=== RUN\|--- \(PASS\|FAIL\|SKIP\)\)' "$out" | head -1)
# count tests by their terminal lines only (a subtest reports its own line)
ran=$(grep -c '^--- \(PASS\|FAIL\|SKIP\)' "$out")
failed=$(grep -c '^--- FAIL' "$out")
rm -f "$out"
echo "Tests: $ran ran, $failed failed"
exit $status
