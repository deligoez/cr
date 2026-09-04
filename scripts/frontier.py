#!/usr/bin/env python3
"""Read the task graph and say what is worth starting next.

An orchestrator picks the next slice from what it remembers of the dependency
graph, and remembering is the part that fails: three ownership claims in one
session named the wrong task, each time because the graph was reconstructed from
a handful of `tp show` calls rather than read. The units caught all three, at
the cost of their attention and once at the cost of a whole slice.

So this reads the graph instead. Four questions, in the order they matter:

  ready       what can be started right now
  leverage    how many blocked tasks each ready one releases, transitively --
              a task nothing waits on is worth less than one twenty wait on,
              and that ordering is invisible without counting
  critical    the longest remaining chain, which is the floor on how many
              slices are left no matter how much runs in parallel
  regions     where the open mass sits, so a whole area cannot go unstarted
              until the end, when context is worst and the tasks are hardest

    scripts/frontier.py [tasks.json]
"""

import collections
import json
import sys
from pathlib import Path

DEFAULT = Path(__file__).parent.parent / "spec" / "0.1.0.tasks.json"


def load(path):
    doc = json.loads(Path(path).read_text())
    tasks = doc.get("tasks", doc)
    return {t["id"]: t for t in tasks}


def unblocks(tasks):
    """For each task, the set of open tasks that transitively wait on it."""
    waiters = collections.defaultdict(set)
    for tid, t in tasks.items():
        for dep in t.get("depends_on", []):
            waiters[dep].add(tid)

    out = {}
    for tid in tasks:
        seen, stack = set(), list(waiters[tid])
        while stack:
            w = stack.pop()
            if w in seen:
                continue
            seen.add(w)
            stack.extend(waiters[w])
        out[tid] = {w for w in seen if tasks[w].get("status") == "open"}
    return out


def longest_chain(tasks):
    """The longest path through the open subgraph, as a list of ids.

    This is the floor on remaining slices: no amount of parallelism shortens a
    chain, only the work inside each link.
    """
    memo = {}

    def depth(tid):
        if tid in memo:
            return memo[tid]
        memo[tid] = ([], 0)  # cycle guard; a cycle would be a task-file defect
        best, blen = [], 0
        for dep in tasks[tid].get("depends_on", []):
            if dep in tasks and tasks[dep].get("status") == "open":
                path, plen = depth(dep)
                if plen > blen:
                    best, blen = path, plen
        memo[tid] = (best + [tid], blen + 1)
        return memo[tid]

    best = ([], 0)
    for tid, t in tasks.items():
        if t.get("status") == "open" and depth(tid)[1] > best[1]:
            best = depth(tid)
    return best[0]


def region(tid):
    """A coarse area name, taken from the id's first word."""
    return tid.split("-", 1)[0]


def main(argv):
    tasks = load(argv[1] if len(argv) > 1 else DEFAULT)
    status = collections.Counter(t.get("status") for t in tasks.values())
    done = {tid for tid, t in tasks.items() if t.get("status") == "done"}

    ready = [
        tid
        for tid, t in tasks.items()
        if t.get("status") == "open"
        and all(d in done for d in t.get("depends_on", []))
    ]
    releases = unblocks(tasks)

    print(
        "%d done, %d open, %d wip — %d ready"
        % (status["done"], status["open"], status["wip"], len(ready))
    )

    print("\nready, by what each releases:")
    for tid in sorted(ready, key=lambda t: (-len(releases[t]), t)):
        n = len(releases[tid])
        print("  %3d  %s" % (n, tid) if n else "    ·  %s" % tid)

    chain = longest_chain(tasks)
    print("\ncritical path — %d slices minimum, however much runs at once:" % len(chain))
    for i, tid in enumerate(chain):
        print("  %d. %s%s" % (i + 1, tid, "  ← ready" if tid in ready else ""))

    print("\nopen mass by region:")
    counts = collections.Counter(region(t) for t in tasks if tasks[t].get("status") == "open")
    started = {region(t) for t in done}
    for name, n in counts.most_common(12):
        mark = "" if name in started else "   ← nothing here has been closed yet"
        print("  %3d  %s%s" % (n, name, mark))

    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
