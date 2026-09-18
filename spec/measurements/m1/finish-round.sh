#!/bin/sh
# After both batches: record the remaining cells, merge every role file,
# record, draft, dry-run post, status. Everything lands under $M; nothing is
# sent (no --confirm anywhere).
set -u
M=$(cd "$(dirname "$0")" && pwd)
export CR_HOME=$M/home
cd "$M/clone"
FAN=$CR_HOME/state/deligoez/cr/pr-1/fanout/1

cat "$M"/cells/*-correctness.ndjson "$M"/cells/*-convention.ndjson > "$M/cells-rest.ndjson" 2>/dev/null
echo "cells rest: $(wc -l < "$M/cells-rest.ndjson")"
cr cells record 1 "$M/cells-rest.ndjson" > "$M/cells-rest-record.json" 2> "$M/cells-rest-record.err"; echo "cells record exit $?"; head -c 600 "$M/cells-rest-record.err"

FILES=$(ls "$FAN"/*/review-*.ndjson 2>/dev/null)
echo "role files: $(echo "$FILES" | wc -w); records: $(cat $FILES | wc -l)"
cr merge $FILES -o "$M/merged.ndjson" --pr 1 > "$M/merge.json" 2> "$M/merge.err"; echo "merge exit $?"; head -c 600 "$M/merge.err"
python3 -c 'import json; d=json.load(open("'"$M"'/merge.json")); print("merged", d.get("merged"), "by_grade", d.get("by_grade"), "by_kind", d.get("by_kind"), "dup", d.get("duplicates"), "possible_duplicates", len(d.get("possible_duplicates") or []))' 2>/dev/null

cr record 1 "$M/merged.ndjson" > "$M/record.json" 2> "$M/record.err"; echo "record exit $?"; head -c 800 "$M/record.err"
python3 -c 'import json; d=json.load(open("'"$M"'/record.json")); r=d.get("recorded",[]); print("recorded", len(r), "finding", sum(1 for x in r if x["kind"]=="finding"), "question", sum(1 for x in r if x["kind"]=="question"), "grades", {g: sum(1 for x in r if x["grade"]==g) for g in ("probed","cited","argued")}, "dup", d.get("duplicates"))' 2>/dev/null

cr draft 1 > "$M/draft.json" 2> "$M/draft.err"; echo "draft exit $?"; head -c 400 "$M/draft.err"
python3 -c 'import json; d=json.load(open("'"$M"'/draft.json")); print("queued", d.get("queued"), "forced_to_question", d.get("forced_to_question"), "comments", d.get("comments"), "warnings", (d.get("warnings") or [])[:3])' 2>/dev/null
cr post 1 > "$M/post-dry.json" 2> "$M/post-dry.err"; echo "post dry exit $?"; head -c 400 "$M/post-dry.err"
python3 -c 'import json; d=json.load(open("'"$M"'/post-dry.json")); print("posted", d.get("posted"), "comments", len(d.get("comments") or []), "honesty", d.get("honesty"))' 2>/dev/null
cr status 1 > "$M/status.json" 2> "$M/status.err"; echo "status exit $?"
python3 -c 'import json; d=json.load(open("'"$M"'/status.json")); print("coverage", d.get("coverage")); print("intent", d.get("intent")); print("completeness", d.get("completeness")); print("records", d.get("records"))' 2>/dev/null
