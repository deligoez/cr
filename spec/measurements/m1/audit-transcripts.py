#!/usr/bin/env python3
"""Post-batch contamination audit over the role transcripts (stream-json).

Pre-registered rules (2026-09-18, before any batch ran):
  1. every codedbpro call's `file`/`path` argument must be an absolute path under
     the clone, or one of the allowed writes/reads (the prompt's output file, the
     round's contract.md, the cells/ and mapping/ files of that prompt); a single
     relative or foreign path voids that role's cell for the run;
  2. no '[Honcho Memory' block may appear anywhere in the transcript;
  3. every Bash call must be a `git log`/`git show` in the clone.
Prints one line per transcript: OK or VOID with the offending calls.
"""
import json, os, sys, glob

M = os.path.dirname(os.path.abspath(__file__))
CLONE = os.path.join(M, "clone")
HOME = os.path.join(M, "home")
ALLOWED_PREFIXES = (CLONE + "/", HOME + "/state/deligoez/cr/pr-1/rounds/", HOME + "/state/deligoez/cr/pr-1/fanout/",
                    M + "/cells/", M + "/mapping/")

def calls(path):
    for line in open(path):
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            o = json.loads(line)
        except json.JSONDecodeError:
            continue
        if o.get("type") != "assistant":
            continue
        for block in (o.get("message") or {}).get("content", []):
            if block.get("type") == "tool_use":
                yield block.get("name"), block.get("input") or {}

def audit(path):
    text = open(path).read()
    bad = []
    if "[Honcho Memory" in text:
        bad.append("honcho-block")
    n = 0
    for name, inp in calls(path):
        n += 1
        if name and name.startswith("mcp__codedbpro__"):
            for key in ("file", "path"):
                v = inp.get(key)
                if v is None:
                    continue
                if not (isinstance(v, str) and (v == CLONE or v.startswith(ALLOWED_PREFIXES))):
                    bad.append(f"{name}:{key}={v!r}")
            if name == "mcp__codedbpro__batch":
                bad.append("batch-used")
        elif name == "Bash":
            cmd = str(inp.get("command", ""))
            if not (cmd.startswith("git log") or cmd.startswith("git show") or cmd.startswith("git -C " + CLONE)):
                bad.append(f"bash={cmd[:80]!r}")
        elif name in ("Read", "Write", "Edit", "Grep", "Glob", "Agent", "WebFetch", "WebSearch"):
            bad.append(f"native:{name}")
    return n, bad

if __name__ == "__main__":
    void = 0
    for path in sorted(glob.glob(os.path.join(M, "role-logs", "*.json"))):
        tag = os.path.basename(path)[:-5]
        n, bad = audit(path)
        if bad:
            void += 1
            print(f"VOID {tag} calls={n} {'; '.join(bad)}")
        else:
            print(f"OK   {tag} calls={n}")
    print(f"voided: {void}")
    sys.exit(1 if void else 0)
