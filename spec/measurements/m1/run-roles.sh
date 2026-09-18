#!/bin/sh
# Run one cr review prompt as a headless claude session from the fresh clone.
#
#   run-roles.sh <prompts.json> <index> <model>
#
# <prompts.json> is the `prompts` array cr review printed (one object per
# prompt: role, axis, unit, output, first_id, last_id, prompt). The session
# runs with cwd = the experiment clone at exp/m1-head, so the instruction file
# it sees is v0.1.0's CLAUDE.md and the project memory directory is empty. It
# may read the clone and write exactly one file: the prompt's output path.
# Its stdout (json) is kept beside the output for the token/turn accounting
# and for the contamination check (no "[Honcho Memory" block may appear in
# the transcript it reports).
set -eu
M=$(cd "$(dirname "$0")" && pwd)
PROMPTS=$1; IDX=$2; MODEL=$3
CLONE=$M/clone
LOG=$M/role-logs; mkdir -p "$LOG"
OUT=$(python3 -c 'import json,sys; p=json.load(open(sys.argv[1]))[int(sys.argv[2])]; print(p["output"])' "$PROMPTS" "$IDX")
TAG=$(python3 -c 'import json,sys; p=json.load(open(sys.argv[1]))[int(sys.argv[2])]; print(p["unit"]+"-"+p["role"])' "$PROMPTS" "$IDX")
UNIT=${TAG%%-*}; ROLE=${TAG#*-}
mkdir -p "$(dirname "$OUT")" "$M/cells" "$M/mapping"
python3 -c 'import json,sys; p=json.load(open(sys.argv[1]))[int(sys.argv[2])]; sys.stdout.write(p["prompt"])' "$PROMPTS" "$IDX" > "$LOG/$TAG.prompt"
cd "$CLONE"
claude -p \
  --model "$MODEL" \
  --output-format stream-json --verbose \
  --no-session-persistence \
  --permission-mode acceptEdits \
  --allowedTools "mcp__codedbpro__read,mcp__codedbpro__faster_search,mcp__codedbpro__create,Bash(git log:*),Bash(git show:*)" \
  --disallowedTools "Read,Write,Edit,Grep,Glob,Agent,WebFetch,WebSearch,mcp__codedbpro__batch,mcp__codedbpro__meta_search,mcp__codedbpro__edit,mcp__codedbpro__patch,mcp__codedbpro__replace" \
  --append-system-prompt "You are one review role in a measured run, driven by a script rather than a person, so the prompt's instructions are the whole task. The repository under review is the clone at $CLONE; read it only through mcp__codedbpro__read and mcp__codedbpro__faster_search with ABSOLUTE paths under $CLONE (a relative path resolves against another directory and reads the wrong tree); the one file outside the clone you may read is the contract file the prompt names. Native Read/Grep are blocked here. Write with mcp__codedbpro__create, absolute paths, and only these files: (1) the output path the prompt names ($OUT), one JSON record per line, or no file when you raise nothing; (2) $M/cells/$TAG.ndjson holding this unit's coverage cell for your role as one JSON line {\"unit\":\"$UNIT\",\"role\":\"$ROLE\",\"result\":\"pass\"|\"finding\"|\"question\"|\"na\",\"reason\":\"...\" when na}; (3) only when your role is intent-coverage: $M/mapping/$UNIT.ndjson holding the claim-to-unit pairs you decided, one JSON line {\"claim\":\"<claim id>\",\"unit\":\"$UNIT\"} per claim this unit implements, or an empty file when none. Do not run tests, do not build, do not create or edit any other file. Finish by writing those files; no summary is needed." \
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
