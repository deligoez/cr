# Measurement 1 — recall of cr's reading roles against known defects

2026-09-18. ROADMAP.md "Next 1, Measure the bet", third measurement. Orchestrated by one session, pre-registered and second-read by another (`cr-smart`); every number below was produced before either reader saw the other's matching, and the two matchings agreed on every record of both rounds after one concession (f9601, see below).

## Subject

- Ground truth: the 53 `fix(...)` commits between v0.1.0 and v0.2.0, each tied to a qa- task whose closure reason names the QA defect (`ground-truth.json`). Slice 1: the six packages `internal/{brief,sandbox,probe,run,draft,coverage}` — 32 rows in `slice1-targets.md`, of which **24 recall targets**: 16 whose defective location is inside the slice, 8 partial (split with `internal/cli`); 5 rows whose defect lives outside the slice, 2 fixed by 0.2.0 spec text (compliant with 0.1.0's words, bonus only), 1 harness.
- Pull request: deligoez/cr#1, base `exp/m1-base` (the v0.1.0 tree minus the six packages, parented on the v0.1.0 tag), head `exp/m1-head` (the v0.1.0 tree). The diff adds the packages from nothing, tests included: 96 files, 14,656 lines, no fix in the branch history. A reversed v0.2.0→v0.1.0 diff was rejected because 83% of it would have been the fixes as removed lines (LEFT units), which hands the roles the answer.
- Intent: spec/0.1.0.md §3.7, §4.5, §5, §7, §9.3, §10 as the text read at the v0.1.0 tag (455 lines). Claims extracted **mechanically**, one per numbered item and table row, spans verbatim: 110 claims. This departs from §3.3's "produced by the agent" on purpose, so the number reads as *given perfect claims*, the upper bound P1 can offer.
- Profile `generic` by configuration: test axis disabled, reinvention and test-symbol lenses unavailable, role test-adequacy skipped — the brief's honesty lines, verbatim in `brief.json`. Three roles ran: intent-coverage, correctness, convention, the built-in role files untouched.
- Units: cr formed 96 — every added file is one hunk, and §3.4.5 makes an over-cap single hunk one oversized unit, so the roles read whole files (25–51 KB prompts) flagged oversized. A hunk-sized pull request is not comparable to this without saying so.
- Prompt shape: `cr review --units` over the 41 production units; the 55 test-file units were left unfilled and the round is incomplete by design. The second intent pass (§4.6.5's unmapped-unit re-emission) was not run; the ordinary flow would have.
- Roles: Opus, one `claude -p` session per prompt from a fresh clone at the head commit (its CLAUDE.md is v0.1.0's, its memory directory empty), eight in parallel, tools limited to codedbpro read/search/create with absolute paths under the clone plus `git log`/`git show`. The memory plugin was off for the run (measured beforehand: with it on, a headless session in the clone received the per-turn "Relevant conclusions" block, which carries the QA's defect descriptions verbatim). Transcript audit after each batch: 0 of 123 voided (every codedbpro path under the clone or an allowed write, no memory block, no native tool); the clone tree clean after every batch. Most roles judged from the prompt's own diff and read little else, so the audit mostly audited the writes.

## Pre-registered rules

Found: a record whose anchor or a citation lies inside the row's v0.1.0 ranges (widened to the enclosing declaration for the six omission rows) **and** whose summary names the same behaviour, judged by both readers independently. False assertion: a `kind: finding` whose claim does not hold on the head tree, checked by hand; target 0. Headline over the 16 `yes` rows; partials reported separately. Comparator: the same 41 units with the intent axis unavailable, pre-decided.

## Result

| | intent round | no-intent comparator |
|---|---|---|
| prompts | 123 (41 intent, 41 correctness, 41 convention) | 82 (correctness, convention) |
| wall clock (8 parallel) | 3 min + 9 min | 5 min |
| cost | $89.30 | $59.95 |
| rate-limit events in transcripts | 231 | 184 |
| records | 23: 2 findings, 21 questions | 10: 4 finding records (3 distinct defects: f5301 and f14901 are one defect raised by two roles under two classes, so §6.4.1 did not fold them), 6 questions (f18902 a duplicate of f9301) |
| grades | cited 15, argued 8, probed 0 | cited 8, argued 2, probed 0 |
| **targets found, 16 yes rows** | **0** | **0** |
| targets found, 8 partial rows | 0 | 0 |
| omission rows raised as gaps (6) | 0 | — |
| false assertions | **0** (both findings true by reading, both readers) | **0** (all four true by reading) |
| true findings outside the ground truth | 2 (f10601, f5301) | 3 distinct in 4 records (f10601; f5301/f14901; f18401) |
| true beyond-spec questions later codified in 0.2.0 | 2 (f19203 §9.3.3, f15801 §5.4.3) | 0 |
| near-misses (within lines of a target, different question) | 5 (f9301, f11401, f12701, f18401, f18901) | 4 (f2701, f9301, f18901, f18401 now a finding) |
| transcript audit | 0 of 123 voided | 0 of 82 voided |
| claims mapped / gaps | 79 of 110 mapped, 31 gaps, almost all claims implemented outside the slice (activation, the probe lock in `state`, §7.3/§7.4 in `cli`/`finding`, §10 in `cli`); the rest (c16, c26, c33) describe the agent's inputs or a prohibition and are implemented by no code | — |

The one disagreement between the readers: the orchestrator had f9601 (`cr brief` on a moved head reports one head, §9.3.1) as a possible partial for row 24 (`cr status` failing on a head the clone lacks); the second reader's "different command, different behaviour" is right, and it is counted as none.

## Reading

cr's reading roles, given the spec's own items as claims and whole files as units, found none of the 24 defects that a 478-case QA pass found by executing scenarios — and said four true things about v0.1.0 that the 14 recorded audit rounds under spec/.tp-review/0.1.0 did not say either. Measured, one grep anyone can repeat (`"evidence_file":"internal/brief/rekey.go"` across `spec/.tp-review/0.1.0/audit-round-*.ndjson`): every one of the 14 rounds carries exactly one such row, and all 14 read `"status":"PASS"`, `"evidence_lines":"14-40"`, item `task-issue-key-rewrite-refusal` ("KeyRewriteError (exit 4) with a --issue hint refuses before any write"), the exact lines f10601 flags; every rendered.json mention in the rounds is a triage-body row, none about DecodeRendered's error class; no row mentions Unclean, an orphaned sandbox, or git.Head. The audit did not merely fail to say it; it passed the lines fourteen times. The four: two real defects (a hint that cannot succeed when `intent.key_pattern` changed; a corrupt rendered.json exiting 2 with the usage hint instead of 3 with UnusableHint, a class the QA found only for meta.json; still present on main, checked by reading main's rendered_decode.go:19, a bare fmt.Errorf, regenerate.go:63 returning it, and exit.go's rows, none of which claims a json error, so the floor's usage hint applies), and two departures from the 0.1.0 text that 0.2.0 later codified, raised as questions. No assertion was wrong.

The near-misses are the informative part. Five records landed within a few lines of a target and asked a different question: the roles reached the right places and did not have the scenario that turns the place into the defect. That is the case for probes and for QA-shaped gap tests over reading, and it bounds what P1 buys: given perfect claims, the intent axis mapped 79 of 110 and raised 7 questions, none about the six omission rows. The reading we offer is that a claim maps to code that implements it in general and the defects were specific cases (a runner that never started, a probe voided by the post-run check).

What the claims bought, measured by the comparator: with the intent axis unavailable the same 41 units produced 9 distinct records instead of 23 — the 7 intent questions vanished and correctness/convention wrote 9 instead of 16 — and the recall on the 24 targets was 0 either way. The comparator asserted more (3 distinct defects against 2) and asked less, and every assertion was true; the intent round's extra records were questions about what the claims did not state (an unmapped payload item, an unstated refusal, a rule the slice's sections did not carry). Both conditions are single runs of a nondeterministic model, and nothing here separates the 16→9 difference in correctness/convention records from run-to-run variance; what survives that caveat is the recall (0 and 0) and the identity of the two defects found in both rounds. So on this subject the claims bought questions about the spec, not defects in the code, and the same two real defects were found with and without them; a third (`Unclean` returning git's error instead of an unclean reason, so an orphaned sandbox is never rebuilt) was found in both rounds, as a question with claims and as a finding without.

"Found nothing the QA found" and "found nothing" are different sentences; the trust economy's own number — wrong assertions reaching a colleague — is 0 of 2 with claims and 0 of 4 finding records (3 defects) without, and the recall number says what the reading roles are not: a substitute for execution.

## Defects found by the run itself, not targets

- **D-M1-1** `cr draft 1` exits 1 refusing record f13901 because its stored evidence quotes `<!-- cr:` (a convention role reviewing draft/marker.go quoted the marker), and the hint says "edit that record's body in the draft" — but no draft exists yet, `cr triage` needs one, and findings.ndjson is not hand-editable. A record whose evidence carries the reserved sequence can never be drafted; the hint names the file the refusal prevents.
- f10601 (rekey hint cannot succeed when `intent.key_pattern` changed), f5301/f14901 (corrupt rendered.json exits 2 with the usage hint; still on main), f18401 (an unreadable sandbox worktree is never recreated; `Unclean` returns `git.Head`'s error before reaching the recreate path) — as defects to file. f19001 (copy through a symlink the head checks out may land outside the sandbox) and f18901 (timeout wait unbounded when a descendant leaves the group holding the pipe) — as questions to verify.

## Files

In this repository under `spec/measurements/m1/`: `ground_truth.py` → `ground-truth.json` (all 53 fix commits with their v0.1.0 ranges), `targets.md` (the 32-row table, → `targets.json` through `targets.py`), `matching-orchestrator.md`, and the method: `run-roles.sh`, `run-batch.sh`, `audit-transcripts.py`, `finish-round.sh`, `match.py`. Kept outside the repository, in the orchestrating session's scratchpad `measure-1/` (205 stream-json transcripts with their prompts and usage lines, the two cr state roots `home/` and `home-noint/`, `intent-0.1.0-slice1.md`, `claims.ndjson`): re-derivable from the tag, the spec and the scripts, except the transcripts, which are the n = 1 record of what each role did.
