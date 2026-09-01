#!/usr/bin/env python3
"""Compare a gremlins run's survivors against the classifications on record.

`gremlins unleash -o <file>` writes one record per mutant, carrying its file,
line, column, mutator and status. This reads that file and answers one question
about each LIVED mutant: has somebody already looked at this one and said why it
survives?

There are three answers, and the third is the reason this exists.

  not on the list        a new survivor -- classify it
  on the list            a judgement already made -- pass over it
  on the list, no mutant a judgement that no longer describes anything

The third is a failure, not a tidy-up. A list that silently drops entries it
cannot match is a record of a decision nobody is being asked to make again: the
code moved, the mutant with it, and the sentence explaining why it was harmless
now sits against nothing. That is the same shape as a `//nolint` protecting a
rule that no longer fires, which `nolintlint`'s `allow-unused: false` refuses
for the same reason.

Line numbers drift, so entries do go stale -- that is expected, and being told
about it is the whole point. Re-point the entry at the mutant that moved, or
drop it because the code it described is gone.

Exit 0 when every survivor is on the list and every entry matches a survivor.
Exit 1 otherwise, naming what changed.

    scripts/survivors.py <run.json> [known.json]
"""

import json
import sys
from pathlib import Path

DEFAULT_KNOWN = Path(__file__).parent / "known-survivors.json"


def key(entry):
    """A mutant is identified by where it is and what was done to it."""
    return (entry["file"], entry["line"], entry["column"], entry["type"])


def survivors(run_path):
    """Every LIVED mutant in a gremlins -o report.

    NOT COVERED and TIMED OUT are deliberately not read here. Neither is a
    survivor: the first was never reached by a test, the second is usually a
    detection that arrived as a hang rather than a failure.
    """
    doc = json.loads(Path(run_path).read_text())
    out = []
    for f in doc.get("files", []):
        for m in f.get("mutations", []):
            if m.get("status") == "LIVED":
                out.append(
                    {
                        "file": f["file_name"],
                        "line": m["line"],
                        "column": m["column"],
                        "type": m["type"],
                    }
                )
    return out


def main(argv):
    if len(argv) < 2:
        print(__doc__.strip().splitlines()[-1].strip(), file=sys.stderr)
        return 2

    run_path = argv[1]
    known_path = Path(argv[2]) if len(argv) > 2 else DEFAULT_KNOWN

    found = {key(s): s for s in survivors(run_path)}
    known = {}
    if known_path.exists():
        for e in json.loads(known_path.read_text()).get("survivors", []):
            known[key(e)] = e

    new = sorted(k for k in found if k not in known)
    stale = sorted(k for k in known if k not in found)

    for k in new:
        print("NEW      %s:%d:%d %s" % k)
    for k in stale:
        print("STALE    %s:%d:%d %s" % k)
        print("         was: %s" % known[k].get("reason", "(no reason recorded)"))

    print(
        "\n%d survivors, %d classified, %d new, %d stale"
        % (len(found), len(found) - len(new), len(new), len(stale))
    )

    if new:
        print("\nClassify each new survivor as one of:")
        print("  equivalent  -- nothing can observe the change; say why")
        print("  boundary    -- a real edge with no test; write the test")
        print("  contract    -- a documented rule with no assertion; assert it")
        print("Only the last two are work. Do not call one equivalent before")
        print("applying it by hand and watching the suite stay green.")
    if stale:
        print("\nA stale entry is a judgement with nothing left to judge.")
        print("Re-point it at the mutant that moved, or delete it because the")
        print("code it described is gone. Do not leave it.")

    return 1 if (new or stale) else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
