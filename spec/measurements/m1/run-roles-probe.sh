#!/bin/sh
# Pass 2 (probes): like run-roles.sh, with the `go` profile's state root and a
# system-prompt addendum letting a role PROPOSE probes it cannot run itself.
#   run-roles-probe.sh <prompts.json> <index> <model>
set -eu
M=$(cd "$(dirname "$0")" && pwd)
PROMPTS=$1; IDX=$2; MODEL=$3
CLONE=$M/clone; export CR_HOME=$M/home-probe
LOG=$M/role-logs-probe; mkdir -p "$LOG"
OUT=$(python3 -c 'import json,sys; p=json.load(open(sys.argv[1]))[int(sys.argv[2])]; print(p["output"])' "$PROMPTS" "$IDX")
TAG=$(python3 -c 'import json,sys; p=json.load(open(sys.argv[1]))[int(sys.argv[2])]; print(p["unit"]+"-"+p["role"])' "$PROMPTS" "$IDX")
UNIT=${TAG%%-*}; ROLE=${TAG#*-}
PROPOSE=$M/probes-proposed
mkdir -p "$(dirname "$OUT")" "$M/cells-probe" "$PROPOSE"
python3 -c 'import json,sys; p=json.load(open(sys.argv[1]))[int(sys.argv[2])]; sys.stdout.write(p["prompt"])' "$PROMPTS" "$IDX" > "$LOG/$TAG.prompt"
cd "$CLONE"
claude -p \
  --model "$MODEL" \
  --output-format stream-json --verbose \
  --no-session-persistence \
  --permission-mode acceptEdits \
  --allowedTools "mcp__codedbpro__read,mcp__codedbpro__faster_search,mcp__codedbpro__create,Bash(git log:*),Bash(git show:*)" \
  --disallowedTools "Read,Write,Edit,Grep,Glob,Agent,WebFetch,WebSearch,mcp__codedbpro__batch,mcp__codedbpro__meta_search,mcp__codedbpro__edit,mcp__codedbpro__patch,mcp__codedbpro__replace" \
  --append-system-prompt "You are one review role in a measured run, driven by a script rather than a person, so the prompt's instructions are the whole task. The repository under review is the clone at $CLONE; read it only through mcp__codedbpro__read and mcp__codedbpro__faster_search with ABSOLUTE paths under $CLONE (a relative path resolves against another directory and reads the wrong tree); the one file outside the clone you may read is the contract file the prompt names. Native Read/Grep are blocked here. You cannot run cr, go or tests. Write with mcp__codedbpro__create, absolute paths, and only these files: (1) the output path the prompt names ($OUT), one JSON record per line, or no file when you raise nothing; (2) $M/cells-probe/$TAG.ndjson holding this unit's coverage cell for your role as one JSON line {\"unit\":\"$UNIT\",\"role\":\"$ROLE\",\"result\":\"pass\"|\"finding\"|\"question\"|\"na\"} plus \"reason\" when na, and for role test-adequacy also \"coverage\":{\"classification\":\"covered\"|\"partially-covered\"|\"uncovered\",\"test_paths\":[...]} (test_paths may be empty only beside uncovered); (3) PROBES YOU PROPOSE, which the orchestrator runs with cr after you finish: for each experiment that would turn one of your records from a question into evidence, write its input under $PROPOSE/ and one manifest line to $PROPOSE/$TAG.ndjson: {\"record\":\"<the id of your record it supports>\",\"kind\":\"mutation\"|\"gap\",\"file\":\"<absolute path you wrote>\",\"filter\":\"<go test -run regex selecting the tests that should catch it>\",\"paths\":[\"internal/<pkg>\"],\"target\":\"<path:line>\" (gap only),\"claim\":\"<claim id>\" (gap only; a gap probe supports a finding only through a claim mapped to this unit)}. A mutation input is a unified diff against ONE file of this unit as it is at the head (paths a/internal/... b/internal/..., a hunk header with correct pre-image line numbers, no rename/mode lines); its target is the first removed line; it should be a behaviour-changing edit a correct test would catch, and the filter should select the existing tests that ought to catch it (paths narrows go test to that package). A gap input is a Go test FILE that cr places at internal/cli/cr_probe_<id>_test.go (package cli of the head's own internal/cli package; read that package's existing *_test.go in the clone for the helpers that build and drive the cr binary) with ONE test function whose name your filter selects; it asserts the behaviour the spec claim requires, so that it FAILS on the head if the defect you suspect is real. Propose probes only where they would decide something; none is fine. Do not run tests, do not build, do not create or edit any other file. Finish by writing those files; no summary is needed." \
  < "$LOG/$TAG.prompt" > "$LOG/$TAG.json" 2> "$LOG/$TAG.err" || echo "exit $? for $TAG" >> "$LOG/failures.txt"
python3 -c 'import json,sys
d={}
for line in open(sys.argv[1]):
    line=line.strip()
    if line.startswith("{"):
        o=json.loads(line)
        if o.get("type")=="result": d=o
u=d.get("usage",{}) or {}
print(sys.argv[2], "turns", d.get("num_turns"), "in", u.get("input_tokens"), "cache", u.get("cache_read_input_tokens"), "out", u.get("output_tokens"), "cost", d.get("total_cost_usd"))' "$LOG/$TAG.json" "$TAG" >> "$LOG/usage.txt" 2>/dev/null || true
