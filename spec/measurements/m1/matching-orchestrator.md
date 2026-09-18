# Measurement 1 — orchestrator's matching (independent of the second reader's)

Test per pre-registration: a record is *found* for a row when its anchor or a citation lies inside the row's ranges (mechanical, `match.py`) **and** its summary names the same behaviour (reading). Written before reading the second reader's list.

## Per record

| record | kind/grade | mechanical candidates | reading verdict | row |
|---|---|---|---|---|
| f9601 | question/cited | — | `cr brief` on a moved head reports only the new head, not both (§9.3.1). Not row 13 (staled records). Row 24 (D-W3-1) is `cr status` crashing on a moved head; its fix 8814bb2 also made `headReport` name both heads, and W3 push-03 quotes §9.3.1's "report both". Same section, different command: **disputed partial for row 24**, else beyond-table candidate. | 24? |
| f19201 | question/argued | — | payload items no claim names (files listed, candidate notes). Ordinary unmapped-item question. | none |
| f19202 | question/argued | — | rekey refusal unstated by any claim. | none |
| f19203 | question/argued | — | round-index exception for `post_unresolved` that no claim states. **True beyond-spec observation**: the §9.3.3 departure the v0.1.0 release notes declared and 0.2.0 codified. | beyond-spec |
| f10601 | **finding**/cited | — | KeyRewriteError's remedy `--issue <recorded>` cannot succeed when `intent.key_pattern` changed, because `ResolveKey` applies the pattern to the flag too (key.go:98-105 says so by design). **True by reading**; not a table row. | beyond-table, true |
| f20201 | question/argued | — | same as f19202. | none |
| f1801 | question/cited | — | fourth copy of a helper. Convention. | none |
| f11401 | question/cited | — | `na` cell on the test axis exempt from `coverage`. Not row 3 (a non-na cell without test_paths accepted). | none |
| f21501 | question/cited | — | Expect's comment says a skipped role is still expected. Not row 1. | none |
| f2701 | question/cited | row 2 (cites cell.go:143,150) | private cellKey vs Seat. Reinvention, not D-S04-2 (repeated seat accepted). | none |
| f12701 | question/argued | row 4 (draft.go:206) | fence length vs a suggestion containing ```. Not D-S09-1 (indentation compares the stored suggestion). Beyond-table candidate, unverified. | none |
| f3301 | question/cited | — | duplicated predicate. | none |
| f3701 | question/cited | — | tally reinvention. | none |
| f13901 | question/cited | row 17 (ingest.go:126) | marker with leading whitespace read as prose. Not D-S08-4 (duplicate id blamed on the untouched block). Beyond-table candidate; related to v0.3.0's D-G2-1 family. | none |
| f5301 | **finding**/cited | — | corrupt rendered.json → bare error → exit 2 with the usage hint instead of 3 + UnusableHint. **True by reading** (rendered_decode.go:19 is a bare fmt.Errorf; the same class as D-S11-1 for meta.json, which 98df0c8 fixed for meta.json only; main's rendered_decode.go still has no UnusableHint). Exit code to be measured. | beyond-table, true |
| f15801 | question/cited | — | gap rung 2 answers `error` on a signal though 0.1.0's §5.4.3 names only an unstartable runner. **True beyond-spec observation**: S07's "not filed" note, codified in 0.2.0 §5.4.3. | beyond-spec |
| f26701 | question/argued | — | Span.Holds rule unstated by the claims given (§6.2.2 was not in the intent slice). Correct relative to its input. | none |
| f7701 | question/cited | — | MapClaim reinvention. | none |
| f18101 | question/cited | rows 8, 25 (cites sandbox/run.go:181) | Verdict ignores TimedOut, so a timed-out run with exit 0 would be `passed`. Not D-S06-4 (no reason on error) nor D-S06-5 (orphan group). Beyond-table candidate, to verify. | none |
| f27701 | question/argued | — | `!Contaminated` clause and fields unstated (c19 arguably states it). | none |
| f18401 | question/cited | row 11 (clean.go:98-101) | Ensure errors instead of recreating when the sandbox dir is not a readable worktree. Not D-S07-4 (voided probe unexplained). Close to S10's harness note ("missing but already registered worktree" until `git worktree prune`). Beyond-table candidate, to verify. | none |
| f9301 | question/argued | rows 8, 29 (run.go:69-92) | duplicated doc comment. Not D-… (patch naming a missing file exits 3). | none |
| f18901 | question/cited | row 8 (run.go:163-173) | after the timeout kill, Run blocks on <-finished unbounded if a descendant left the group holding the pipe. Not D-S06-4; related to the v0.2.1 limitation on the runner start window, but about the wait. Beyond-table candidate, to verify. | none |

## Totals (orchestrator)

- Targets found: **0 of 16 yes rows, 0 of 8 partial rows** (one disputed partial, f9601 → row 24, to be settled against the second reader's list).
- Findings (assertions): 2, both true by reading (f10601, f5301); false assertions: 0 pending f5301's exit-code measurement.
- Beyond-spec observations that are true and were later codified: 2 (f19203, f15801).
- Beyond-table candidates needing verification: f12701, f13901, f18101, f18401, f18901, f9601.
- Omission rows as gaps: 0 of 6 (rows 8, 10, 11, 13, 14, 30); the mapping had 31 gaps, all claims whose implementation lies outside the six packages.
