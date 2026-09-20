#!/usr/bin/env python3
"""Measurement 4, part A: audit every role transcript before its output is counted.

Measurement 1's and 3's fences, applied again because each failed once:

  1. The memory plugin's blocks must appear in no transcript. It leaked twice,
     and both times the whole batch was voided.
  2. Every tool call must be one the runner allowed, and every path argument
     must resolve inside the worktree or the state root. A codedbpro relative
     path resolves against the daemon's tree, not the clone, so a relative path
     read the wrong repository.
  3. No transcript may have reached the answer key: the human's review threads,
     the pull request's conversation, or anything under the cr repository that
     holds this measurement's own notes.

A transcript that fails any check is listed as voided; its outputs are not
counted, and the count of voided transcripts is reported with the result.

    audit.py <role-logs-dir> <worktree> <state-root> <repo-under-measurement>

The write roots are the three the runner's own system prompt names: the
worktree (read only), the state root (the prompt's record and proposal files),
and the measurement directory (the cells and mapping the runner collects). A
path outside all three is a session that went somewhere nobody asked it to.
"""
import json
import pathlib
import sys

ALLOWED = {
    "mcp__codedbpro__read",
    "mcp__codedbpro__faster_search",
    "mcp__codedbpro__create",
    "Bash",
    "TodoWrite",
    # The harness's deferred-tool loader. It takes a query and returns
    # schemas; it reads no file and writes none, so it is not a channel to
    # anything this audit is looking for.
    "ToolSearch",
}

# Text that means the session saw something it must not have.
LEAKS = [
    "[Honcho Memory",
    "Relevant conclusions",
    "reviewThreads",
    "/projects/cr/spec/measurements",
    "MEASUREMENT",
]


def path_args(payload):
    """Every path-shaped argument of one tool call."""
    out = []
    for key in ("file", "path", "file_path", "notebook_path"):
        value = payload.get(key)
        if isinstance(value, str):
            out.append(value)
    for value in payload.get("paths", []) or []:
        if isinstance(value, str):
            out.append(value)
    command = payload.get("command")
    if isinstance(command, str):
        out.append(command)
    return out


def audit(path, roots, repo):
    """Return the reasons this transcript is voided, empty when it is clean."""
    reasons = []
    text = path.read_text(errors="replace")
    for leak in LEAKS:
        if leak in text:
            reasons.append(f"leak: {leak!r} appears in the transcript")
    for line in text.splitlines():
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        message = event.get("message")
        if not isinstance(message, dict):
            continue
        content = message.get("content")
        if not isinstance(content, list):
            continue
        for block in content:
            if not isinstance(block, dict) or block.get("type") != "tool_use":
                continue
            name = block.get("name", "")
            if name not in ALLOWED:
                reasons.append(f"tool: {name}")
                continue
            for arg in path_args(block.get("input") or {}):
                if name == "Bash":
                    continue
                if not arg.startswith("/"):
                    reasons.append(f"relative path: {arg}")
                elif not any(arg.startswith(root) for root in roots):
                    reasons.append(f"outside: {arg}")
                if repo and arg.startswith(repo):
                    reasons.append(f"reached the measurement's own repository: {arg}")
    return sorted(set(reasons))


def main():
    logs, worktree, state, repo = (sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4])
    roots = (worktree, state, str(pathlib.Path(logs).parent))
    voided, clean = {}, 0
    for path in sorted(pathlib.Path(logs).glob("*.json")):
        reasons = audit(path, roots, repo)
        if reasons:
            voided[path.stem] = reasons
        else:
            clean += 1
    print(f"clean {clean}, voided {len(voided)}")
    for tag, reasons in sorted(voided.items()):
        print(f"  {tag}")
        for reason in reasons:
            print(f"    - {reason}")
    json.dump(voided, open(pathlib.Path(logs).parent / "voided.json", "w"), indent=1)


if __name__ == "__main__":
    main()
