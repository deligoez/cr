# Field feedback

Observations from running a released cr against a real pull request, recorded as they arrive so each can
become a task, a roadmap item, or a documented decision. An entry states what was observed and by whom;
a suggestion is the reporter's, and the triage line is cr's. Nothing here is normative, and no entry is
taken as true on arrival: each is verified against the code, the spec and a measurement before it becomes
work, and its triage records the verdict and the evidence.

## FT-1 · tarfin-labs/backend#6233 (WB-3155), cr v0.2.1, laravel-pest profile

Reporter: a Claude Code session reviewing the pull request on the user's behalf, with nothing posted.
Shape: 81 files, ~9.8k added lines, a module move; 106 units, 4 roles, 25 claims, 57 mapping pairs.

### Batch 1 (2026-09-15, after the intent pass and the first record)

| # | Observation | Reporter's suggestion |
|---|-------------|-----------------------|
| 1.1 | The issue links a Google Docs spec; cr read only the tracker text, and `honesty` says nothing about the link. A local tp spec for the same key was reachable only through `cr note`. | `cr brief` lists URLs in the issue text in `honesty`; a repeatable `--intent-file` or `intent.extra_sources`; `candidate_notes` surfaces a local spec file. |
| 1.2 | `jira issue view --plain` wraps, pads and appends ANSI footer lines; spans across a wrap are unwritable, and one span failed on a U+00A0. | Normalise whitespace, NBSP and ANSI when storing and matching; or fetch the raw description. |
| 1.3 | 106 units × 4 roles = 424 prompts of 13.5–15k chars; one sub-agent per prompt is not feasible; the full issue text repeats in every prompt (~40%). | Document batching; `cr review --shard k/n` or `--units`; a size warning; one shared context file. |
| 1.4 | A module move produced LEFT units for the originals and RIGHT units for the destinations, ~40 cells confirming "moved, unchanged". | Rename-aware units; review only the delta; default cells to `na`. |
| 1.5 | Notes recorded after the intent prompts were emitted never reached them; an intent record asked what note n4 settles, and `cr record` accepted it silently. | Stamp prompts with a notes/claims hash and report stale records; `cr note` hints emitted prompts are out of date. |
| 1.6 | `cr review` after the mapping re-emits all 424 prompts, including the recorded intent cells. | Emit only unrecorded cells, or mark prompts `already_covered`. |
| 1.7 | The laravel-pest `tests.cmd` has no path, so `cr test` without `--filter` runs the whole suite, against the team's rule; `--filter` is name-only. | Test path arguments; confirm or warn when neither filter nor path is given. |

Worked well, as reported: per-(role, unit) id blocks kept eight parallel writers collision-free; `cr record`
graded all five intent records `cited` because their citations lay outside the unit.

### Verification of batch 1 (2026-09-15, v0.2.1, scratch fixture with a gh shim and `--intent-file`)

| # | Verdict | Trust-relevant | Evidence |
|---|---------|----------------|----------|
| 1.1 | Confirmed; by design today | Yes: a rule only behind a link can never become a claim, and nothing says so | `internal/intent/run.go:234-243` reads only `--intent-file` or `intent.cmd`, with no link scan; a fixture issue with a `docs.google.com` URL left `honesty` silent; `candidate_notes` (§3.5.5) offers only the author's thread replies. No spec clause covers linked documents or extra sources. |
| 1.2 | Partly | Mostly friction; a claim the agent cannot record tends to be dropped or reworded | `internal/intent/claimspan.go:92-94` is a plain substring test by design (§3.3, "verbatim substring"); §1.4 normalisation feeds only `span_hash` and `issue_hash` and collapses only space and tab. Measured: two spaces against NBSP+space, exit 1; the exact bytes, exit 0; one space across a hard wrap, exit 1; exact padding and newline, exit 0. ANSI escapes are stored in the issue text. "Unwritable" is overstated: the exact bytes match. Not measured: whether jira's wrap follows terminal width and so moves `issue_hash`. |
| 1.3 | Partly; cause misattributed | Friction | The issue text is in no prompt. A 6,307-byte fixture intent prompt was 48% cr's output contract (the §6.1 schema); on the first intent pass every prompt lists all claims (`internal/review/text.go:162-174`), the likely repeated bulk. `cr review` has only `--axis` (`internal/cli/review.go:109`). `skills/cr/SKILL.md:144` says to spawn one sub-agent per prompt, while §4.6.1 does not forbid batching agents, so the instruction is the defect. |
| 1.4 | Partly | Friction | Rename detection is on (50% similarity, rename limit 1000; `internal/git/diff.go:32`, `internal/git/run.go:73`). Measured: a pure move forms no unit; a larger file with a namespace change forms one RIGHT unit; a 4-line file with namespace and `use` changed falls under 50% and forms a LEFT and a RIGHT unit. The reported pairs arise when git does not pair the files, likely small PHP files. §4.5.6 forbids cr from inventing a cell. |
| 1.5 | Confirmed | Yes: a question a note already settles can reach the author | Measured: a note recorded after `cr review --axis intent` is absent from the emitted prompts and present on re-emission. No notes or claims hash exists; `cr note` prints no hint; `cr record` reads no notes; the draft uses notes only for provenance; §3.6.6 reacts only to a retraction. Comparing a note's time with a prompt's emission is mechanical, so a stamp does not make cr form an opinion. |
| 1.6 | Confirmed; matches §4.6.1 as written | Friction, with a small trust risk: a second, conflicting judgement for a recorded cell | Measured: with every intent cell recorded, `cr review` emitted the intent prompts again; `expected_cells` lists every cell and nothing marks recorded ones (`internal/review/run.go:375-380`, `internal/review/emit.go:179-181`). The skill's own sequence (`cr map record`, then `cr review`) leads into it. Id blocks prevent an id collision, not the duplicate judgement. |
| 1.7 | By design, and worse than reported | Friction; on a suite with one failing test, no probe can establish a missing test | `internal/profile/builtin/laravel-pest.json:25-27` has no path; `cr test` has only `--filter` (`internal/cli/test.go:283`). §5.2.2 requires an unfiltered baseline once per head, and `probe.Required` runs it before any probe (`internal/probe/baseline.go:79-85`), so a filtered probe still runs the whole suite first; §5.2.5 counts a baseline as passed only with zero failures. |

### Proposed disposition of batch 1 (settled with a second reviewing session; not yet the user's decision)

The test: an addition to output, state or the skill is a patch; a new intent source, a flag in §11's
table, a prompt-content rule or a baseline rule needs spec text and belongs to v0.3.

Claims in this proposal that were checked before any task is written (2026-09-15, read-only, on the
stored #6233 run and the backend repository; scripts in the verification scratch directory `fbv2/`):

| Claim | Verdict | Evidence and consequence |
|-------|---------|--------------------------|
| §4.6.5 requires the full claim list in every first-pass prompt | True, with a caveat | §4.6.5: the first pass "MUST emit prompts carrying the units and the claims but no mapping"; with no mapping the only way to carry them is the whole list (`internal/review/text.go:135-137,162-174`). "Carrying" does not say "printed inline", so a shared file is a reading to decide in v0.3. |
| A computed record field for 1.5 needs no normative text | False | No MUST fixes a record's key set, but `finding_test.go` pins `finding.Fields()` to §6.1's table, §6.1.4's refusal list is built from that table (`fields.go:120-132`), and §4.6.2 prints the schema from it into every prompt: a field without a row either is refused under no clause or can be written by an agent. The patch uses a separate state file (`emissions.ndjson`) with "notes newer than the prompt" computed when read, as `note.Standing` is, which avoids §6.1 and §6.1.4. |
| Transport cleanup in 1.2 cannot move a span | Partly | No span matches anywhere new (substring test, no stored position; occurrence counts unchanged for all 25 claims on #6233). But the issue hash changes (`3eb14e94…` to `fb4dd119…`), so `drift.go:112` reports every claim of an existing round as drifted, and a stored span holding an NBSP (c15) stops occurring. The patch must map stored spans the same way or scope the cleanup to new rounds, and say whether `--intent-file` text is cleaned too. |
| M1: `jira --plain` wrap width | Measured | The stored #6233 issue text is word-wrapped at 118 columns with space padding (129 of 138 lines exactly 118), 3 NBSPs, 14 ANSI sequences all in the two footer lines. A piped run on 2026-09-03 also gave 118; whether it follows `COLUMNS` is unmeasured. Recorded spans stop at wraps. cr keeps no copy of the issue text in state; the run's `brief.json` holds it. |
| M2: rename threshold on #6233 | Measured | Base `999e6dcf`, head `073b6f4e`: at 50% 11 renames / 3 deletions / 53 additions; 40% 13/1/51; 30% 14/0/50; 20% as 30%. The three extra pairs are real moves (same file name into the new module). 30% removes 3 LEFT units (12 cells); the other RIGHT units under the moved module belong to pairs git already makes at 50%, so most of the reported ~40 cells do not depend on the threshold. |

Found while verifying, not reported: on #6233 all 106 first-pass intent prompts say "The round recorded no
claim". `cr review --axis intent` ran at 20:35:02 and `cr claims record` at 20:36:57, so the first pass was
emitted before the claims existed and cr neither refused nor warned. The intent agents mapped 57 pairs, so
they reached the claims some other way; whether §4.6.5's "carrying the claims" makes this a refusal is to
be decided. Trust-relevant: an intent pass run from those prompts alone judges no claim.

| # | Patch (v0.2.x) | Spec (v0.3) |
|---|----------------|-------------|
| 1.1 | `cr brief` lists every URL in the issue text under `honesty` as linked and not read; the skill names today's workaround, a note-sourced claim (§3.3.2) from `cr note --source other`. | `intent.extra_sources` or a repeatable `--intent-file`; a §3.1 clause on linked documents. |
| 1.2 | At ingestion, strip terminal control sequences and map U+00A0 to U+0020 before storing; match verbatim on the stored text, keep line breaks significant (joining wrapped lines could let a span straddle two items); the skill says to copy spans from `cr brief`'s printed issue text. Release note: `issue_hash` changes once for affected issues. Measure first whether `jira --plain` wraps to a width when piped; if so, pin the width in the command's environment. | Only if the default tracker command changes (§3.1.2). |
| 1.3 | The skill says to batch k prompts per sub-agent (ids are per prompt, so batching is safe) and gives a size guide. The claim list in every first-pass prompt is §4.6.5, not a defect. | Moving the §6.1 schema out of each prompt (§4.6.2); `--units` or `--shard`. |
| 1.4 | Measure the rename similarity threshold on #6233's file set (50/40/30%) and pick the lowest that pairs the moves without pairing unrelated files; §3.4 does not fix the threshold. No `na` defaults (§4.5.6); the skill says a moved, unchanged unit is a `pass` cell. | — |
| 1.5 | Report, never refuse. `cr review` writes one line per emission (round, head, pass, unit, role, time, the note ids carried) to a new `emissions.ndjson`; `cr note` says which emitted passes it postdates; `cr record` reports how many records came from a prompt older than a note on their claim or unit (computed from the file, not stored as a record field; see the checks above); `cr draft` renders a cr-owned line naming those notes in the block (to check: §7.1 fixes what a block carries); `cr status` counts them. | — |
| 1.6 | `expected_cells` entries carry `recorded: true`; the skill's sequence says which prompts to run after `cr map record`. | Omitting recorded prompts, or a flag for it (§4.6.1, §11). |
| 1.7 | When a probe establishes nothing because its baseline failed, `honesty` says so and names the baseline and its failed count; the skill says the first probe at a head runs the whole suite once. | A baseline scoped to the probe's filter (§5.2.2, §5.5), which evaluates §5.2.5 over the same tests the probe speaks about; a profile field for a test path (§2.4). |

### Batch 2, early items (2026-09-15, during the probe stage)

| # | Observation | Verification | Trust cost |
|---|-------------|--------------|------------|
| 2.1 | **Data safety.** The laravel-pest sandbox ran the test suite against the developer's local application database (`tarfin`, a local copy of production data) instead of the test database. A mutation `cr probe run --filter …` with no prior baseline ran the unfiltered suite there for ten minutes before it was killed; a filtered `cr test` failed 3 of 25 tests that pass 250/250 in the clone, on real rows ("208 is identical to 2"). Rows were rolled back only because the repository's `TestCase` uses `DatabaseTransactions`; a `RefreshDatabase` repository would have run `migrate:fresh` on the development database. | Confirmed on the mechanism: `internal/profile/builtin/laravel-pest.json:21` copies only `.env` and `vendor`; the repository's `.env.testing` exists and is gitignored (`.gitignore:10`), so the git-worktree sandbox lacks it; `phpunit.xml:35-36` sets `APP_ENV=testing` and `DB_CONNECTION=testing`; `config/database.php:70-74`'s `testing` connection reads `env('DB_DATABASE', 'tarfin-testing')`. The value `.env` supplies was not read by cr's session (it holds secrets); the reporter measured it, and the failing counts corroborate it. The unfiltered baseline is §5.2.2 as written (batch 1, 1.7). | Critical: an experiment that is not isolated and can mutate real data. |
| 2.2 | The first-pass intent prompts were emitted before `cr claims record`, so all 106 said "The round recorded no claim"; cr neither refused nor warned. | Confirmed (batch 1 checks); the reporter's agents read the claims from their own brief, so no cell was judged claimless on this run. | High: without a side channel a whole axis is judged against no claims. |
| 2.3 | Two cross-unit duplicates of one real bug (f12101, f13901) were not merged, because dedup groups by anchored line. | Not yet verified; repro requested. | To assess. |
