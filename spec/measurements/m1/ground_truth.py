#!/usr/bin/env python3
"""Ground-truth table for measurement 1 (recall against known defects).

For every fix(...) commit in v0.1.0..v0.2.0: the qa- task that closed on it,
the defect ids its closure reason names, the source sections, the production
files it touched, and the fix hunks' pre-image ranges translated to v0.1.0
line numbers (the head lines an anchor must land on when the subject PR adds
the v0.1.0 packages from nothing).

Reads git and `tp list --json`; writes ground-truth.json and ground-truth.md
next to itself. Measurement only: nothing in the repository is written.
"""
import difflib, json, os, re, subprocess, sys
from collections import defaultdict

REPO = "/Users/deligoez/Developer/deligoez/projects/cr"
OUT = os.path.dirname(os.path.abspath(__file__))
TASKS = "spec/0.1.0.tasks.json"


def git(*args, text=True):
    return subprocess.run(["git", "-C", REPO, *args], check=True, capture_output=True, text=text).stdout


def show(rev, path):
    r = subprocess.run(["git", "-C", REPO, "show", f"{rev}:{path}"], capture_output=True, text=True)
    return r.stdout.splitlines() if r.returncode == 0 else None


# --- tasks -------------------------------------------------------------------
tasks = json.loads(subprocess.run(["tp", "--file", TASKS, "list", "--json"], cwd=REPO, check=True,
                                  capture_output=True, text=True).stdout)
tasks = tasks if isinstance(tasks, list) else tasks.get("tasks", tasks)
by_commit = {}
for t in tasks:
    if not str(t.get("id", "")).startswith("qa-"):
        continue
    full = json.loads(subprocess.run(["tp", "--file", TASKS, "show", t["id"], "--json"], cwd=REPO,
                                     check=True, capture_output=True, text=True).stdout)
    for sha in full.get("commit_shas") or [full.get("commit_sha")]:
        if sha:
            by_commit[sha[:7]] = full

DEFECT_RE = re.compile(r"\b(D-[SW]\d+-\d+|S-S\d+-\d+|KL\d+)\b")

# --- fix commits -------------------------------------------------------------
rows = []
for sha in git("log", "--format=%h", "--reverse", "v0.1.0..v0.2.0").split():
    subject = git("log", "-1", "--format=%s", sha).strip()
    if not subject.startswith("fix("):
        continue
    parent = git("rev-parse", f"{sha}^").strip()[:7]
    files = [f for f in git("show", "--format=", "--name-only", sha).split()
             if f.endswith(".go") and not f.endswith("_test.go") and f.startswith(("internal/", "cmd/"))]
    task = by_commit.get(sha)
    hunks = []
    for f in files:
        # pre-image ranges of the fix in the parent tree
        # the machine sets diff.external; pin the unified format or the hunk
        # headers never appear (CLAUDE.md: every git diff pins its knobs)
        diff = git("-c", "diff.external=", "diff", "--no-ext-diff", "-U0", parent, sha, "--", f)
        ranges = []
        for m in re.finditer(r"^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@", diff, re.M):
            start, count = int(m.group(1)), int(m.group(2) or 1)
            if count == 0:  # pure insertion: the anchor is the insertion point
                ranges.append((start, start))
            else:
                ranges.append((start, start + count - 1))
        # translate parent line numbers to v0.1.0 line numbers
        base = show("v0.1.0", f)
        par = show(parent, f)
        mapped = []
        if base is None:
            status = "absent-at-v0.1.0"
        elif par == base:
            status = "same-as-v0.1.0"
            mapped = ranges
        else:
            status = "translated"
            sm = difflib.SequenceMatcher(a=par, b=base, autojunk=False)
            # map each parent line -> base line (None when the line does not exist at v0.1.0)
            p2b = {}
            for tag, i1, i2, j1, j2 in sm.get_opcodes():
                if tag == "equal":
                    for k in range(i2 - i1):
                        p2b[i1 + k + 1] = j1 + k + 1
            for s, e in ranges:
                bs = [p2b.get(n) for n in range(s, e + 1) if p2b.get(n)]
                if bs:
                    mapped.append((min(bs), max(bs)))
                else:
                    # the changed lines themselves differ; take the nearest mapped neighbours
                    lo = max([p2b[n] for n in p2b if n < s], default=None)
                    hi = min([p2b[n] for n in p2b if n > e], default=None)
                    if lo and hi:
                        mapped.append((lo, hi))
                    elif lo or hi:
                        mapped.append((lo or hi, lo or hi))
        hunks.append({"path": f, "parent_ranges": ranges, "v010_ranges": mapped, "translation": status})
    pkgs = sorted({os.path.dirname(f) for f in files})
    rows.append({
        "commit": sha,
        "subject": subject,
        "task": task["id"] if task else None,
        "defects": sorted(set(DEFECT_RE.findall((task or {}).get("closed_reason", "") + " " +
                                                (task or {}).get("acceptance", "")))),
        "sections": (task or {}).get("source_sections", []),
        "packages": pkgs,
        "hunks": hunks,
        "class": None,  # filled by hand: code+section | code+silent | spec-text | harness
    })

with open(os.path.join(OUT, "ground-truth.json"), "w") as fh:
    json.dump(rows, fh, indent=2)

# --- markdown summary --------------------------------------------------------
lines = ["| commit | task | defects | packages | sections | hunks (v0.1.0 lines) |", "|---|---|---|---|---|---|"]
for r in rows:
    hs = "; ".join(f"{h['path'].removeprefix('internal/')}:" +
                   ",".join(f"{s}-{e}" if s != e else str(s) for s, e in h["v010_ranges"]) +
                   ("" if h["translation"] != "absent-at-v0.1.0" else "(new)")
                   for h in r["hunks"])
    lines.append(f"| {r['commit']} | {r['task'] or '—'} | {', '.join(r['defects']) or '—'} | "
                 f"{', '.join(p.removeprefix('internal/') for p in r['packages'])} | "
                 f"{', '.join(s.lstrip('# ') for s in r['sections'])} | {hs} |")
with open(os.path.join(OUT, "ground-truth.md"), "w") as fh:
    fh.write("\n".join(lines) + "\n")

pk = defaultdict(int)
for r in rows:
    for p in r["packages"]:
        pk[p] += 1
print(f"fix commits: {len(rows)}; with task: {sum(1 for r in rows if r['task'])}; "
      f"with defect ids: {sum(1 for r in rows if r['defects'])}")
print("packages:", dict(sorted(pk.items(), key=lambda kv: -kv[1])))
