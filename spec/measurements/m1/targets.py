#!/usr/bin/env python3
"""Parse slice1-targets.md into targets.json: one entry per row with its
ranges (path -> list of [start, end]) including the widened omission ranges.
Used by match.py for the orchestrator's independent matching."""
import json, os, re

M = os.path.dirname(os.path.abspath(__file__))
rows = []
for line in open(os.path.join(M, "slice1-targets.md")):
    if not line.startswith("| ") or line.startswith("| # ") or line.startswith("|---"):
        continue
    cells = [c.strip() for c in line.strip().strip("|").split("|")]
    if len(cells) < 8 or not cells[0].isdigit():
        continue
    num, commit, defects, wrong, section, klass, inslice, ranges = cells[:8]
    spans = {}
    # "brief/brief.go:160-197, 348-350; coverage/cell.go:12 (note); **omission**: brief.go:104-177"
    text = ranges.replace("**omission**:", ";")
    # "pkg/file.go:12-15, 20" followed by another "file.go:" or "pkg/file.go:";
    # ranges are greedy digits/dashes separated by commas up to the next path.
    # A bare file name (the omission entries) resolves to the package the row's
    # prefixed entries gave that file, never to the previous entry's package.
    entries = list(re.finditer(r"(?:([a-z]+)/)?([a-z_]+\.go):((?:\d+(?:-\d+)?)(?:,\s*\d+(?:-\d+)?)*)", text))
    pkg_of = {fname: pkg for pkg, fname, _ in (m.groups() for m in entries) if pkg}
    for m in entries:
        pkg, fname, rng = m.groups()
        path = f"internal/{pkg or pkg_of[fname]}/{fname}"
        for r in rng.split(","):
            a, _, b = r.strip().partition("-")
            spans.setdefault(path, []).append([int(a), int(b or a)])
    rows.append({"row": int(num), "commit": commit, "defects": defects, "wrong": wrong, "section": section,
                 "class": klass, "in_slice": inslice.split(" ")[0], "ranges": spans})
json.dump(rows, open(os.path.join(M, "targets.json"), "w"), indent=1)
targets = [r for r in rows if r["class"] in ("code+section", "code+silent") and r["in_slice"] in ("yes", "partial")]
print("rows", len(rows), "targets", len(targets), "yes", sum(1 for r in targets if r["in_slice"] == "yes"),
      "partial", sum(1 for r in targets if r["in_slice"] == "partial"))
for r in targets:
    print(r["row"], r["in_slice"], {k.removeprefix("internal/"): v for k, v in r["ranges"].items()})
