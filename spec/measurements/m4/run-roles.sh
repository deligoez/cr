#!/bin/sh
# Measurement 4, part A: run one cr review prompt as a headless claude session
# against a detached worktree of tarfin-labs/backend at the reviewed commit.
#
#   run-roles.sh <prompts.json> <index> <model>
#
# Measurement 3's run-roles-b.sh with one change: the role may also write the
# §5.7 proposals file its prompt names. Nothing else moves, because the whole
# design is one variable.
#
# Read-only by construction: the tools allow reading the worktree and writing
# the prompt's own outputs, the round's cell and (intent only) the mapping.
# Nothing runs tests, nothing writes into the worktree, nothing reaches gh.
set -eu
M=$(cd "$(dirname "$0")" && pwd)
PROMPTS=$1; IDX=$2; MODEL=$3
WT=$M/wt
export CR_HOME=$M/home; export PATH=$M/gh-shim:$PATH
LOG=$M/role-logs; mkdir -p "$LOG"
field() {
  python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["prompts"][int(sys.argv[2])][sys.argv[3]])' \
    "$PROMPTS" "$IDX" "$1"
}
OUT=$(field output)
PROPOSALS=$(field proposals)
UNIT=$(field unit)
ROLE=$(field role)
TAG="$UNIT-$ROLE"

# Precondition, checked per session rather than trusted: the memory plugin's
# per-turn injection reaches a headless session and carries this user's stored
# conclusions, including the ones this measurement is generating. It leaked
# into a batch twice, voiding 123 transcripts and then 22. The key is
# `hosts.claude_code.enabled`, not a top-level `enabled`.
if python3 -c 'import json,sys; c=json.load(open("'"$HOME"'/.honcho/config.json")); sys.exit(0 if (c.get("hosts") or {}).get("claude_code",{}).get("enabled") is False else 1)' 2>/dev/null; then
  :
else
  echo "refusing to run $TAG: the honcho plugin is not disabled (see ~/.honcho/config.json)" >&2
  echo "precondition $TAG: honcho enabled" >> "$LOG/failures.txt"
  exit 2
fi

# A session is capped in wall clock, because one was not. Measured over this
# measurement's own 76 sessions: median 0.6 min, p90 1.6 min, and one session
# at 1745 min — a five-hour usage window that `claude -p` waits out rather than
# failing. That wait is not an error the runner can see, so the runner bounds
# it: a session past the cap is killed, recorded as stalled, and left for the
# driver to retry once the window has turned.
CAP=${CR_M4_SESSION_TIMEOUT:-600}
mkdir -p "$(dirname "$OUT")" "$(dirname "$PROPOSALS")" "$M/cells" "$M/mapping"
python3 -c 'import json,sys; sys.stdout.write(json.load(open(sys.argv[1]))["prompts"][int(sys.argv[2])]["prompt"])' \
  "$PROMPTS" "$IDX" > "$LOG/$TAG.prompt"
cd "$WT"
started=$(date +%s)
gtimeout --signal=TERM --kill-after=30 "$CAP" \
claude -p \
  --model "$MODEL" \
  --output-format stream-json --verbose \
  --no-session-persistence \
  --permission-mode acceptEdits \
  --allowedTools "mcp__codedbpro__read,mcp__codedbpro__faster_search,mcp__codedbpro__create,Bash(git log:*),Bash(git show:*)" \
  --disallowedTools "Read,Write,Edit,Grep,Glob,Agent,WebFetch,WebSearch,mcp__codedbpro__batch,mcp__codedbpro__meta_search,mcp__codedbpro__edit,mcp__codedbpro__patch,mcp__codedbpro__replace" \
  --append-system-prompt "You are one review role in a measured run, driven by a script rather than a person, so the prompt's instructions are the whole task. The repository under review is a Laravel PHP codebase in the worktree at $WT; read it only through mcp__codedbpro__read and mcp__codedbpro__faster_search with ABSOLUTE paths under $WT (a relative path resolves against another directory and reads the wrong tree); the one file outside it you may read is the contract file the prompt names. Native Read/Grep are blocked here. This session cannot execute anything — no tests, no composer, no artisan, no cr — and you must not write anything inside $WT. Write with mcp__codedbpro__create, absolute paths, and only these files: (1) the records output path the prompt names ($OUT), one JSON record per line, or no file when you raise nothing; (2) the proposals path the prompt names ($PROPOSALS), one JSON proposal per line as its §5.7 section describes, or no file when you propose nothing; (3) $M/cells/$TAG.ndjson holding this unit's coverage cell for your role as one JSON line {\"unit\":\"$UNIT\",\"role\":\"$ROLE\",\"result\":\"pass\"|\"finding\"|\"question\"|\"na\"} plus \"reason\" when na, and for role test-adequacy also \"coverage\":{\"classification\":\"covered\"|\"partially-covered\"|\"uncovered\",\"test_paths\":[...]} (test_paths may be empty only beside uncovered); (4) only when your role is intent-coverage: $M/mapping/$UNIT.ndjson holding one JSON line {\"claim\":\"<claim id>\",\"unit\":\"$UNIT\"} per claim this unit implements, or an empty file when none. Finish by writing those files; no summary is needed." \
  < "$LOG/$TAG.prompt" > "$LOG/$TAG.json" 2> "$LOG/$TAG.err" || status=$?
status=${status:-0}
elapsed=$(( $(date +%s) - started ))
# 124 is gtimeout's: the session was killed at the cap. It is reported as
# `stalled` rather than `exit`, because the two ask for different things — a
# stalled session is retried when the usage window has turned, and an exit is
# a session that failed and will fail again.
if [ "$status" -eq 124 ] || [ "$status" -eq 137 ]; then
  echo "stalled $TAG after ${elapsed}s (cap ${CAP}s)" >> "$LOG/failures.txt"
  exit 124
fi
if [ "$status" -ne 0 ]; then
  echo "exit $status for $TAG after ${elapsed}s" >> "$LOG/failures.txt"
fi
echo "$TAG elapsed ${elapsed}s" >> "$LOG/elapsed.txt"
python3 -c 'import json,sys
d={}
for line in open(sys.argv[1]):
    line=line.strip()
    if line.startswith("{"):
        o=json.loads(line)
        if o.get("type")=="result": d=o
u=d.get("usage",{}) or {}
print(sys.argv[2], "turns", d.get("num_turns"), "in", u.get("input_tokens"), "cache", u.get("cache_read_input_tokens"), "out", u.get("output_tokens"), "cost", d.get("total_cost_usd"))' "$LOG/$TAG.json" "$TAG" >> "$LOG/usage.txt" 2>/dev/null || true
