#!/bin/sh
# run-batch.sh <prompts.json> <model> <first-index> <last-index> [parallel]
# Runs run-roles.sh over an index range, N at a time, and appends one line per
# finished prompt to batch-<basename>.log. Nothing else heavy may run on the box
# meanwhile (CLAUDE.md: the wall-clock numbers must mean something).
set -u
M=$(cd "$(dirname "$0")" && pwd)
P=$1; MODEL=$2; FROM=$3; TO=$4; PAR=${5:-8}
LOG=$M/batch-$(basename "$P" .json).log
echo "start $(date +%H:%M:%S) $P $MODEL $FROM-$TO x$PAR" >> "$LOG"
seq "$FROM" "$TO" | xargs -P "$PAR" -I{} sh -c 'sh "$1" "$2" {} "$3" >/dev/null 2>&1; echo "done {} $(date +%H:%M:%S)" >> "$4"' _ "$M/run-roles.sh" "$P" "$MODEL" "$LOG"
echo "end $(date +%H:%M:%S)" >> "$LOG"
