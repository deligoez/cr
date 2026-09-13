#!/usr/bin/env python3
"""Merge scripts/mutation-run.sh's per-package results into one gremlins file.

gremlins names each file relative to the package it was run on, so the package
directory is prefixed back on, giving the same `pkg/file.go` names a whole-tree
run writes and scripts/survivors.py reads.

Usage: scripts/mutation-merge.py <out-dir> <merged.json>
"""
import glob
import json
import os
import sys
from collections import Counter


def main() -> int:
    if len(sys.argv) != 3:
        print(__doc__.strip().splitlines()[-1], file=sys.stderr)
        return 2
    out_dir, merged_path = sys.argv[1], sys.argv[2]
    merged = {"files": []}
    counts = Counter()
    results = sorted(glob.glob(os.path.join(out_dir, "*.json")))
    if not results:
        print(f"no result files in {out_dir}", file=sys.stderr)
        return 1
    for path in results:
        package = os.path.basename(path)[: -len(".json")]
        with open(path) as f:
            run = json.load(f)
        for entry in run["files"]:
            merged["files"].append(
                {"file_name": f"{package}/{entry['file_name']}", "mutations": entry["mutations"]}
            )
            counts.update(m["status"] for m in entry["mutations"])
    with open(merged_path, "w") as f:
        json.dump(merged, f)
    print(f"{len(results)} packages: " + ", ".join(f"{k} {v}" for k, v in sorted(counts.items())))
    return 0


if __name__ == "__main__":
    sys.exit(main())
