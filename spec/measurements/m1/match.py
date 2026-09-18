#!/usr/bin/env python3
"""Orchestrator's independent matching of recorded records to target rows.

Mechanical half only: a record is a *candidate* for a row when its anchor, or one
of its citations, lies inside one of the row's ranges (path and line overlap).
The summary test ("names the same behaviour") is a reading judgement and is made
by hand in matching.md, independently of the second reader, per the
pre-registration. Reads findings.ndjson from the experiment CR_HOME."""
import json, os, sys

M = os.path.dirname(os.path.abspath(__file__))
FINDINGS = os.path.join(M, sys.argv[1] if len(sys.argv) > 1 else "home", "state/deligoez/cr/pr-1/findings.ndjson")
rows = json.load(open(os.path.join(M, "targets.json")))

def inside(path, line, ranges):
    for a, b in ranges.get(path, []):
        if a <= line <= b:
            return True
    return False

records = [json.loads(l) for l in open(FINDINGS) if l.strip()]
print(f"records: {len(records)}; by kind: " + str({k: sum(1 for r in records if r.get('kind') == k) for k in ('finding', 'question')}))
cand = {}
for rec in records:
    a = rec.get("anchor") or {}
    locs = [(a.get("path"), n) for n in range(int(a.get("start_line", 0) or 0), int(a.get("line", 0) or 0) + 1)]
    locs += [(c.get("path"), int(c.get("line", 0) or 0)) for c in rec.get("citations") or []]
    for row in rows:
        if any(inside(p, n, row["ranges"]) for p, n in locs if p):
            cand.setdefault(row["row"], []).append(rec["id"])
for row in rows:
    ids = cand.get(row["row"], [])
    flag = "" if row["class"] in ("code+section", "code+silent") and row["in_slice"] in ("yes", "partial") else " (not a target)"
    print(f"row {row['row']:2d} {row['in_slice']:8s} {row['class']:13s} candidates: {', '.join(ids) or '-'}{flag}")
unmatched = [r["id"] for r in records if not any(r["id"] in v for v in cand.values())]
print(f"records matching no row: {len(unmatched)}: {', '.join(unmatched)}")
