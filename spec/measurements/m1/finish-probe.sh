#!/bin/sh
# Pass 2 close-out: cells, merge (every role file, intent's from pass 1 are not
# in this root), record, status; then match against the targets.
set -u
M=$(cd "$(dirname "$0")" && pwd)
export CR_HOME=$M/home-probe
cd "$M/clone"
FAN=$CR_HOME/state/deligoez/cr/pr-1/fanout/1
cat "$M"/cells-probe/*.ndjson > "$M/cells-probe.ndjson" 2>/dev/null; echo "cells: $(wc -l < "$M/cells-probe.ndjson")"
cr cells record 1 "$M/cells-probe.ndjson" > "$M/probe-cells-record.json" 2> "$M/probe-cells-record.err"; echo "cells record exit $?"; head -c 600 "$M/probe-cells-record.err"
FILES=$(ls "$FAN"/*/review-*.ndjson 2>/dev/null)
echo "role files: $(echo "$FILES" | wc -w); records: $(cat $FILES 2>/dev/null | wc -l)"
cr merge $FILES -o "$M/probe-merged.ndjson" --pr 1 > "$M/probe-merge.json" 2> "$M/probe-merge.err"; echo "merge exit $?"; head -c 600 "$M/probe-merge.err"
cr record 1 "$M/probe-merged.ndjson" > "$M/probe-record.json" 2> "$M/probe-record.err"; echo "record exit $?"; head -c 800 "$M/probe-record.err"
python3 -c 'import json; d=json.load(open("'"$M"'/probe-record.json")); r=d.get("recorded",[]); print("recorded", len(r), "finding", sum(1 for x in r if x["kind"]=="finding"), "question", sum(1 for x in r if x["kind"]=="question"), "grades", {g: sum(1 for x in r if x["grade"]==g) for g in ("probed","cited","argued")}, "probes", d.get("probes"), "dup", d.get("duplicates"))' 2>/dev/null
cr status 1 > "$M/probe-status.json" 2>/dev/null; python3 -c 'import json; d=json.load(open("'"$M"'/probe-status.json")); print("coverage", d.get("coverage")); print("probes", d.get("probes")); print("records", d.get("records"))' 2>/dev/null
cd "$M" && python3 match.py home-probe
