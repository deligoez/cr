#!/bin/sh
# Measurement 3: run one cr review prompt as a headless claude session against a
# detached worktree of tarfin-labs/backend at the pull request's head.
#
#   run-roles.sh <prompts.json> <index> <model>
#
# Read-only by construction: the tools allow reading the worktree and writing
# the prompt's own output, the round's cell and (intent only) the mapping.
# Nothing runs tests, nothing writes into the worktree, nothing reaches gh.
set -eu
M=$(cd "$(dirname "$0")" && pwd)
PROMPTS=$1; IDX=$2; MODEL=$3
WT=$M/wtb
export CR_HOME=$M/home-b; export PATH=$M/gh-shim-b:$PATH
LOG=$M/role-logs-b; mkdir -p "$LOG"
OUT=$(python3 -c 'import json,sys; p=json.load(open(sys.argv[1]))[int(sys.argv[2])]; print(p["output"])' "$PROMPTS" "$IDX")
TAG=$(python3 -c 'import json,sys; p=json.load(open(sys.argv[1]))[int(sys.argv[2])]; print(p["unit"]+"-"+p["role"])' "$PROMPTS" "$IDX")
UNIT=${TAG%%-*}; ROLE=${TAG#*-}

# Precondition, checked per session rather than trusted: the memory plugin's
# per-turn injection reaches a headless session and carries this user's stored
# conclusions, including the ones this measurement is generating. Measured
# twice, both times after the plugin had been re-enabled between passes, and
# both times the post-batch audit voided every transcript. A run that starts
# with it enabled is money spent on unusable output, so it does not start.
# The key is `hosts.claude_code.enabled`, not a top-level `enabled`: the tool
# reports the resolved value, the file stores it per host. Checked by reading
# the file the plugin actually loads.
if python3 -c 'import json,sys; c=json.load(open("'"$HOME"'/.honcho/config.json")); sys.exit(0 if (c.get("hosts") or {}).get("claude_code",{}).get("enabled") is False else 1)' 2>/dev/null; then
  :
else
  echo "refusing to run $TAG: the honcho plugin is not disabled (see ~/.honcho/config.json)" >&2
  echo "precondition $TAG: honcho enabled" >> "$LOG/failures.txt"
  exit 2
fi

mkdir -p "$(dirname "$OUT")" "$M/cells-b" "$M/mapping-b"
python3 -c 'import json,sys; p=json.load(open(sys.argv[1]))[int(sys.argv[2])]; sys.stdout.write(p["prompt"])' "$PROMPTS" "$IDX" > "$LOG/$TAG.prompt"
cd "$WT"
claude -p \
  --model "$MODEL" \
  --output-format stream-json --verbose \
  --no-session-persistence \
  --permission-mode acceptEdits \
  --allowedTools "mcp__codedbpro__read,mcp__codedbpro__faster_search,mcp__codedbpro__create,Bash(git log:*),Bash(git show:*)" \
  --disallowedTools "Read,Write,Edit,Grep,Glob,Agent,WebFetch,WebSearch,mcp__codedbpro__batch,mcp__codedbpro__meta_search,mcp__codedbpro__edit,mcp__codedbpro__patch,mcp__codedbpro__replace" \
  --append-system-prompt "You are one review role in a measured run, driven by a script rather than a person, so the prompt's instructions are the whole task. The repository under review is a Laravel PHP codebase in the worktree at $WT; read it only through mcp__codedbpro__read and mcp__codedbpro__faster_search with ABSOLUTE paths under $WT (a relative path resolves against another directory and reads the wrong tree); the one file outside it you may read is the contract file the prompt names. Native Read/Grep are blocked here. You cannot run tests, composer, artisan or cr, and you must not write anything inside $WT. Write with mcp__codedbpro__create, absolute paths, and only these files: (1) the output path the prompt names ($OUT), one JSON record per line, or no file when you raise nothing; (2) $M/cells-b/$TAG.ndjson holding this unit's coverage cell for your role as one JSON line {\"unit\":\"$UNIT\",\"role\":\"$ROLE\",\"result\":\"pass\"|\"finding\"|\"question\"|\"na\"} plus \"reason\" when na, and for role test-adequacy also \"coverage\":{\"classification\":\"covered\"|\"partially-covered\"|\"uncovered\",\"test_paths\":[...]} (test_paths may be empty only beside uncovered); (3) only when your role is intent-coverage: $M/mapping-b/$UNIT.ndjson holding one JSON line {\"claim\":\"<claim id>\",\"unit\":\"$UNIT\"} per claim this unit implements, or an empty file when none. No probe can be run in this measurement, so a suspicion you cannot establish stays a question, which is what cr asks of you. Finish by writing those files; no summary is needed." \
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
