#!/usr/bin/env python3
"""Structural consistency check for a tp spec.

Two failure modes, both produced by editing one clause and stranding another:

  DANGLING  a §X.Y reference that resolves to no heading and no numbered item
  NUMBER    a numbered list that skips or repeats an index

Both are cheap to introduce during a review round and expensive to find by
reading. Run after every spec edit, before the next round.

    python3 scripts/speccheck.py spec/0.1.0.md

Exit 0 when clean, 1 otherwise.
"""

import collections
import re
import sys


def check(path):
    text = open(path, encoding="utf-8").read()
    lines = text.split("\n")

    headings = set()
    items = collections.defaultdict(set)
    current = None
    previous = 0
    numbering = []

    for lineno, line in enumerate(lines, 1):
        match = re.match(r"^#{2,4} (\d+(?:\.\d+)*)\.? ", line)
        if match:
            current = match.group(1)
            headings.add(current)
            previous = 0
            continue
        if re.match(r"^#{2,4} ", line):
            current = None
            previous = 0
            continue

        match = re.match(r"^(\d+)\. ", line)
        if match:
            number = int(match.group(1))
            if number != previous + 1:
                numbering.append((lineno, current, previous, number))
            previous = number
            if current:
                items[current].add(number)
        elif line.strip() == "" or line.startswith(("   ", "|", "#")):
            pass  # continuation, table row, blank: the list stays open
        else:
            previous = 0  # unindented prose closes the list

    dangling = []
    for match in re.finditer(r"§(\d+(?:\.\d+)*)", text):
        ref = match.group(1)
        if ref in headings:
            continue
        section, _, item = ref.rpartition(".")
        if section in headings and item.isdigit() and int(item) in items[section]:
            continue
        dangling.append((text[: match.start()].count("\n") + 1, ref))

    for lineno, section, before, after in numbering:
        print(f"NUMBER   L{lineno} [§{section}] {before} -> {after}")

    seen = set()
    for lineno, ref in dangling:
        if ref in seen:
            continue
        seen.add(ref)
        print(f"DANGLING L{lineno}: §{ref}")

    print(
        f"headings={len(headings)} "
        f"numbering_breaks={len(numbering)} dangling={len(seen)}"
    )
    return 0 if not numbering and not seen else 1


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit("usage: speccheck.py <spec.md>")
    sys.exit(check(sys.argv[1]))
