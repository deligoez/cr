#!/bin/sh
# Fail when a function is reachable from nothing at all — not from a main
# package, not from a test. golangci-lint's `unused` skips exported identifiers
# by design, so an exported function with zero callers passes that check;
# `deadcode` builds a call graph and does not care about case.
#
# `deadcode` exits 0 whether or not it finds anything, so the exit code is ours.
#
# Note the -test flag. Without it this would report work in progress: a unit
# built before the task that wires it is reachable only from its tests, which is
# normal here and must not fail the gate. `deadcode ./...` without -test is the
# phase-boundary check, run by hand and diffed against the previous run.
set -eu

if ! command -v deadcode >/dev/null 2>&1; then
	echo "deadcode not installed: go install golang.org/x/tools/cmd/deadcode@latest" >&2
	exit 1
fi

out=$(deadcode -test ./...)
if [ -n "$out" ]; then
	echo "$out" >&2
	echo "unreachable from any main package or test: delete it, or wire it up." >&2
	exit 1
fi
