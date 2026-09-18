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
| 1.4 | Measure the rename similarity threshold on #6233's file set (50/40/30%) and pick the lowest that pairs the moves without pairing unrelated files; §3.4 does not fix the threshold. No `na` defaults (§4.5.6); the skill says a moved, unchanged unit is a `pass` cell. **Outcome (2026-09-16): the threshold stays at 50%.** The condition failed when measured: unrelated files sharing boilerplate pair below 50% (a deleted Go test and an unrelated added one at 31%; an Invoice model deleted and an unrelated Customer model added at 46%), the same range as #6233's real moves (32–49%), and a false pair moves a deleted file's removed lines into a unit on another path where no LEFT anchor can land, which raises the cost of a wrong comment; an existing status test fails at 30% for exactly that reason. | Rename-aware pairing that does not rest on similarity alone (e.g. same file name, namespace-only difference), if a later field trial shows the cells cost justifies it. |
| 1.5 | Report, never refuse. `cr review` writes one line per emission (round, head, pass, unit, role, time, the note ids carried) to a new `emissions.ndjson`; `cr note` says which emitted passes it postdates; `cr record` reports how many records came from a prompt older than a note on their claim or unit (computed from the file, not stored as a record field; see the checks above); `cr draft` renders a cr-owned line naming those notes in the block (to check: §7.1 fixes what a block carries); `cr status` counts them. | — |
| 1.6 | `expected_cells` entries carry `recorded: true`; the skill's sequence says which prompts to run after `cr map record`. | Omitting recorded prompts, or a flag for it (§4.6.1, §11). |
| 1.7 | When a probe establishes nothing because its baseline failed, `honesty` says so and names the baseline and its failed count; the skill says the first probe at a head runs the whole suite once. | A baseline scoped to the probe's filter (§5.2.2, §5.5), which evaluates §5.2.5 over the same tests the probe speaks about; a profile field for a test path (§2.4). |

### Batch 2, early items (2026-09-15, during the probe stage)

| # | Observation | Verification | Trust cost |
|---|-------------|--------------|------------|
| 2.1 | **Data safety.** The laravel-pest sandbox ran the test suite against the developer's local application database (`tarfin`, a local copy of production data) instead of the test database. A mutation `cr probe run --filter …` with no prior baseline ran the unfiltered suite there for ten minutes before it was killed; a filtered `cr test` failed 3 of 25 tests that pass 250/250 in the clone, on real rows ("208 is identical to 2"). Rows were rolled back only because the repository's `TestCase` uses `DatabaseTransactions`; a `RefreshDatabase` repository would have run `migrate:fresh` on the development database. | Confirmed on the mechanism: `internal/profile/builtin/laravel-pest.json:21` copies only `.env` and `vendor`; the repository's `.env.testing` exists and is gitignored (`.gitignore:10`), so the git-worktree sandbox lacks it; `phpunit.xml:35-36` sets `APP_ENV=testing` and `DB_CONNECTION=testing`; `config/database.php:70-74`'s `testing` connection reads `env('DB_DATABASE', 'tarfin-testing')`. The value `.env` supplies was not read by cr's session (it holds secrets); the reporter measured it, and the failing counts corroborate it. The unfiltered baseline is §5.2.2 as written (batch 1, 1.7). | Critical: an experiment that is not isolated and can mutate real data. |
| 2.2 | The first-pass intent prompts were emitted before `cr claims record`, so all 106 said "The round recorded no claim"; cr neither refused nor warned. | Confirmed (batch 1 checks); the reporter's agents read the claims from their own brief, so no cell was judged claimless on this run. | High: without a side channel a whole axis is judged against no claims. |
| 2.3 | Two cross-unit duplicates of one real bug (f12101, f13901) were not merged, because dedup groups by anchored line. | Superseded by 2.4 of the full batch below. | See 2.4. |

### Batch 2 (2026-09-15, merge, record, probes, draft, dry-run post; nothing posted)

The reporter tags each item WRONG (a false comment would have gone out), UNHELPFUL (true, but noise or
unreadable) or FRICTION (operator cost only). All items are unverified until the verification section
below says otherwise.

Counts as reported: 106 units (6 LEFT), 25 claims, 7 notes; intent 106 cells (95 pass, 6 na, 5 question),
57 pairs, 19 of 25 claims mapped (c19–c22 frontend outside the repository; c24, c25 draw-time rules with no
code); rest 318 cells and 37 records; 42 records graded cited 24, argued 18; forced to question at draft 0
(every argued record was already a question); dropped by waiver 0, by dedup 0; 19 probes proposed, 12 with a
target inside the record's anchor, 10 chosen, 0 run (blocked by 2.1); draft 42 queued, 20 kept with every
body rewritten, 20 discarded not-here, 2 marked wrong; dry-run post 20 comments, event COMMENT, commit
`073b6f4e40`, no refusal. Wall clock: intent fan-out 8 agents, 1.09M tokens, 4m20s; rest fan-out 8 agents,
1.95M tokens, 17m12s; merge and record 3s; sandbox create 26s; filtered baseline 60s; draft 1s; dry-run
post under 1s; the reporter's own orchestration and triage about 40 minutes.

The reporter credits three real production defects to cr, verified by hand in the code (f12101 a
cancellation after the freeze changes eligibility; f13301 a job without `$timeout` killed at 300s drops the
next rounds; f13401 an export that answers 202 and fails silently in the queue), none caught by earlier
spec-driven audits of the branch, and attributes them to the cell bookkeeping forcing a look at every unit.

| # | Observation (reporter's tag) | Reporter's suggestion |
|---|------------------------------|-----------------------|
| 2.3 | Probe support needs the anchor on the mutated line: 7 of 19 probes support nothing because test-adequacy anchors on the test while the mutation targets production code (FRICTION; real findings stay unproven). | Tell test-adequacy to anchor on the production line under test, or let a probe support a record when its target equals one of the record's citations. |
| 2.4 | Cross-unit duplicates escape dedup: four pairs (f12101/f13901, f35101/f40001, f38401/f40601, f34702/f40501) with different classes and anchors but the same root cause or shared citations (UNHELPFUL: four duplicate comments). | Flag possible duplicates that share a citation or name each other's anchor, listed in `cr merge` output for the human, not dropped. |
| 2.5 | 36 new classes for 42 records; six records on one issue used five class names, so a `wrong` demotes a class nobody reuses and `cr rules suggest` never reaches `harvest_min` (UNHELPFUL, indirect). | A per-axis class vocabulary in the role file, or normalisation at record time. |
| 2.6 | Question bodies without "?" pass `cr record` and `cr draft` and are refused only at `cr post` (FRICTION; safe). | Refuse or warn at record time; state the rule in every prompt's output contract. |
| 2.7 | Labels render in Turkish per `render.lang`, bodies stay English; no prompt names the language to write in (UNHELPFUL). | The prompt states `render.lang` for summary and evidence. |
| 2.8 | Bodies are the agent's evidence prose verbatim, typically 900–1,500 characters (UNHELPFUL). | Post `summary` and keep `evidence` in the draft or a collapsed block; or warn above a length. |
| 2.9 | Two wrong intent records and one settled: f30901 says the changelog does not declare a gate that `changelogs/unreleased/deligoez/hotfix/WB-3155-kampanya-ureticilerine-is-active-gate.md:7` declares (the agent read one of three changelog files); f24601 says no claim states the counting windows, which the issue text does (claim extraction skipped them); f27201 asks what note n4 settles (batch 1, 1.5) (WRONG ×2 if posted). | List issue sentences with dates or amounts that no claim spans ("unclaimed spans") in brief or status. |
| 2.10 | Triage by hand-editing a 62k-character draft: deleting 20 blocks and replacing 20 bodies needed line arithmetic, and one range overshot (FRICTION, corruption risk). | `cr triage <pr> <id>` with `not-here`, `wrong`, `soften` or `--body-file f`. |
| 2.11 | A `wrong` disposition becomes a repository-wide waiver when `cr draft` runs, before any post (needs a decision). | Say so in the draft header, or apply dispositions at post or an explicit apply step. |
| 2.12 | Worked well: id blocks kept 16 parallel writers collision-free; cells guaranteed 424 of 424; no argued record escaped as a finding; dry-run validation and commit pinning were clean; waiver scopes match how reviewers think. | — |

### Verification of batch 2 (2026-09-15, read-only over spec/0.2.0.md, HEAD, the stored #6233 state and the backend repository at `073b6f4e`)

The reported counts match stored state: records cited 24 and argued 18 (31 questions, 11 findings); draft
20 queued, 20 not-here, 2 wrong, 20 pull-request waivers; 12 of 19 probes supportable, 10 chosen, none run;
20 comments on the dry run, `posted.json` empty.

| # | Verdict | Tag holds | Evidence |
|---|---------|-----------|----------|
| 2.3 | Confirmed; follows §6.2.2 | FRICTION | `internal/probe/span.go:33-41`: support needs the target inside the record's anchor on the RIGHT side. All 7 unsupportable probes sit on records anchored in `tests/…` units (f40601, f40001 ×2, f40101, f41901, f39701, f40501), each targeting production code that is also one of the record's citations. No role instruction says where to anchor. **The approved alternative does not work as worded:** §6.1.3 requires the anchor to lie inside the record's own unit, so a record raised from a test-file unit cannot anchor on the production line; it can reach `probed` only if raised from the production unit's cell. |
| 2.4 | Confirmed; by design (§6.4.1) | UNHELPFUL, as triage cost only | Dedup keys on path, side, line and class (`internal/finding/dedup.go:57-64`). Each pair's records are in different files; f35101/f40001 share a class, contrary to the report. Links: f12101/f13901 share two citations and one cites the other's anchor line; f35101/f40001 share a citation and each cites a line inside the other's anchor; f38401/f40601 and f34702/f40501 each cite a line inside the other's anchor. In three pairs both records' probes targeted the same line. The human deleted one of each pair, so none would have posted twice. |
| 2.5 | Confirmed; by design (§6.1, §7.3.3) | UNHELPFUL, indirect | 42 records carry 36 distinct classes (`summary.json` `new_classes` 36); six @see-link records use five names. §6.1 requires only kebab-case; §2.6.3.1 groups harvest candidates by class; no builtin role declares classes. |
| 2.6 | By design (§8.1.5, applied at post) | FRICTION, mild | Enforced when post bodies are built (`internal/draft/post.go:29-33,43`); the output contract never mentions "?". **A warning at record time would misfire:** §6.3.1 forces argued records to questions at record time with English statement prose, and 17 of the 31 stored questions have no "?" in summary or evidence. A warning at `cr draft` does not clash. |
| 2.7 | Not reproduced as stated; by design (§6.1.1, §8.1.2) | Does not hold against cr | Every prompt says to write summary and evidence in English and that reader-facing prose is produced at draft time (`internal/review/contract.go:75`). The agent rewrites bodies in `render.lang` inside draft.md; the skill's Draft section never says so (`skills/cr/SKILL.md:506-507` only defines the setting). A skill gap. |
| 2.8 | Partly; by design (§8.1.2) | UNHELPFUL for the first draft only | `rendered.json` bodies equal summary, a blank line and evidence, byte for byte; no length rule exists. Initial bodies: 440 minimum, 858 median, 1,646 maximum; 15 of 42 in 900–1,500, 3 above. The posted body is the agent's rewrite. |
| 2.9 | f30901 agent-side; f24601 agent-side, catchable mechanically | WRONG holds | f30901: the branch adds three changelog files; `…is-active-gate.md:7` at the head declares the move into `shouldQueue()`; the record cites only another file, and record f16401 (kept) is anchored on that same line. Absence of text is a judgement cr cannot make. f24601: no claim span covers the two counting windows the issue text states; §3.3 checks that spans occur, never that the issue is covered, so a coverage report is text arithmetic. |
| 2.10 | Confirmed; by design (§7.2) | FRICTION | No triage command exists; §7.2: "Triage happens by editing the file." |
| 2.11 | Confirmed; by design (§7.1.6) | The undisclosed part holds | The repository waivers were written at 22:45:34.2678, the draft files a millisecond later, with `posted.json` empty. The draft header (`internal/draft/header.go:66-79`) does not mention waivers, and `skills/cr/SKILL.md:513` suggests they are written at post. |

### Decisions (2026-09-15, the user with the second reviewing session)

- **Releases.** v0.2.2 ships the test-environment safety fix first (2.1). v0.3.0 takes every other verified
  item, including those that need spec text; the re-review half of the loop moves to v0.4. The v0.2.1
  GitHub release page is not edited; the warning goes in the v0.2.2 notes and README.
- **Suggestions declined as proposed, with the alternative taken** (each would raise the cost of a wrong
  assertion or make cr form a judgement):
  - 2.3, a probe supporting a record through its citations: declined, because §6.2's anchor-range rule is
    what stops one mutation from grading records elsewhere; no restricted form keeps "one experiment, one
    location, one assertion" except the rule as written. **Corrected after verification** (the first
    alternative, anchoring on the production line, is impossible from a test-file unit under §6.1.3): per
    §4.4.1 test adequacy is a property of the code under test, so the test-adequacy role raises "production
    line X has no test" from the production unit's cell, anchored on X and citing the test file (§4.4.2 keeps
    it argued until a probe); the test-file unit's cell judges the test itself. The role instructions and the
    skill say so; a production line outside the diff has no unit and is not a record (§1.6.1, §4.1.3).
  - 2.5, normalising classes at record time: declined (cr would decide what a text means). Instead a role
    file may declare a class vocabulary, prompts print it, and `cr record` reports classes outside it.
  - 2.8, a posted body assembled by cr from `summary`: declined (§8.1.2: the agent composes every body).
    Instead the draft header reports bodies above a length.
  - 2.9, flagging issue sentences with dates or amounts: declined (which sentence matters is a judgement).
    Instead brief and status list issue paragraphs no claim span covers, without ranking them.
  - 2.11, moving the `wrong` waiver to post time: declined (§7.1.6 fixes the moment, and "this is false"
    does not depend on posting). Instead the draft header says `wrong` writes a repository-wide waiver when
    the draft regenerates.
  - 2.6: **corrected after verification** — not at `cr record`, where §6.3.1's forcing produces statement
    prose as questions by design (17 of 31 stored questions lack "?"), but at `cr draft`: a warning over
    rendered question bodies lacking "?", naming record ids, with the count in the draft header; `cr post`
    keeps the refusal; the rule is stated in the prompt's output contract.
- **QA.** No test or probe runs against tarfin-labs/backend again; v0.3.0 QA uses a local Laravel-shaped
  fixture, and the backend repository is read only.
- **Placement conditions from the second review.** 1.5's `emissions.ndjson` carries `head` and `round`, is
  written under the per-PR lock, and gets a §2.3 table row in 0.3.0 together with `intake.json`. 2.4's
  possible-duplicate pairs live in `cr merge`'s report, never as a field on a merged record (§6.5.1, §6.1.4).
  2.2's refusal leaves an unavailable intent axis (`prompts:[]`, exit 0) alone and counts claims carried into
  a new round (§9.3.4) as recorded.
- **Process.** From v0.2.3 releases are built without tp; this file's disposition tables are the record of
  what was decided and why, and each commit names the item it closes.

### Batch 3 (2026-09-15, outcome after the author acted on the 20 kept comments; nothing posted)

Reported by the field-trial session: the author fixed the pull request locally in 20 commits. Of the 20
kept comments, 7 were real production defects now fixed (f12101 and f13901 on one root cause, f13301,
f13401, f2801, f13201, f13101, f16401 with f5901); 9 were real test gaps closed with new tests, each checked
by applying the agent's proposed mutation before and after the fix (9 of 9 caught after, none before, so the
argued test-adequacy questions blocked from probing were correct 9 of 9); 2 questions were answered by
existing documented decisions (f29501, f23601); 1 was left open (f1901, a bidirectional @see convention the
author is reconsidering). Precision of the kept comments as reported: 19 of 20 actionable; of the 42 raw
records, 2 were wrong (both intent, 2.9).

| # | Observation | Verification | Triage |
|---|-------------|--------------|--------|
| 3.1 | The fix for f12101/f13901 needed the same status predicate changed in a third place (`CalculatePerformanceTicketsAction::completedChannelOneSaleBy()`) that no record named: a finding about a shared predicate should ask where else it appears. | Not verifiable by cr's session without judging the backend code; the fix commits exist on the branch (20 commits after `073b6f4e`). | Candidate for the correctness role's instructions (non-normative, v0.2.3): when a finding rests on a predicate or rule used in several places, search for its other occurrences and cite them. |
| 3.2 | Six of 42 raw records complained of one-way @see links; the author paused the convention. Suggestion: cr should not ship a built-in role or rule flagging missing @see backlinks; a repository that wants it registers a mechanical rule with `detect`. | cr ships no such role or rule: `git grep -i '@see\|backlink'` over `internal/role`, `internal/rule` and `skills` finds nothing, so the records came from the convention role judging the repository's own conventions. The five class names are 2.5. | Already satisfied; no change. |
| 3.3 | The author's counts: 20 fix commits; module test directories green (266, 75, 16, 31 tests); hand verification of the 3 production findings about 10 minutes. | The 20 commits on `deligoez/hotfix/WB-3155-araba-kampanyalari` after the reviewed head are present; test results were not re-run (no tests run against the backend repository). | Recorded as reported. |

### Measurement: the 3.1 correctness instruction (2026-09-16)

The first 3.1 wording (commit 85d1aba) told the correctness role to cite a shared predicate's other
occurrences. On the correctness axis a resolved citation outside the unit grades a record `cited` (§6.2),
and no mechanical check tells a premise citation from a copy of the same code, so that wording would have
moved reading-only findings into the assertion register. The sharpened wording (7fce22e) names the other
occurrences in the evidence and defines a citation as a location showing the defect's premise; the role's
closing paragraph was reworded to match. Six correctness prompts of #6233 (u16, u18, u26, u29, u34, u59), one
run each, under the pre-85d1aba and the sharpened text:

| | Pre-85d1aba text | Sharpened text |
|---|---|---|
| Records | 5 (u29 none) | 5 (u59 none) |
| Graded `cited` (a premise citation outside the unit) | 5 of 5 | 5 of 5 |
| Citations that are copies of the same code | 3 | 0 |
| Kind chosen `finding` | 1 of 5 | 4 of 5 |
| Real defects per batch 3 | status bypass (u16, u34), NULL type (u26), changelog (u59), u18 unverified | status bypass (u16, u34), the third occurrence the field trial missed (u18), NULL type (u26), export failure (u29) |

The grade did not move and copy citations disappeared. The shift in the agent's chosen kind stays inside a
register cr already permits, every finding in the sample is a real defect, and with six units and one run each
it is within noise; this sample cannot show the risky direction, a wrong finding carrying a premise citation.
The instrument for that is §7.3's triage statistics over real rounds. Baseline recorded before 7fce22e, from
`~/.cr/repos/tarfin-labs/backend/triage.ndjson` (the field trial, one round): correctness 7 raised, 0
`discarded-wrong`, 1 `discarded-not-here`; across all axes 42 raised, 2 `discarded-wrong` (both intent). A rise
in correctness `discarded-wrong` after 7fce22e is the signal to revisit the wording.

## M-1 · measurement 1, cr's own v0.1.0 packages as a pull request (deligoez/cr#1), cr v0.3.0, Opus roles

Not a field trial: a measurement of the reading roles' recall against 24 known defects, recorded whole in
`spec/measurements/2026-09-18-m1-recall-against-known-defects.md`. Recall was 0 of 24 with and without the
intent axis, false assertions 0 of 2 and 0 of 4 records. The run found the following about cr itself; each is
verified by reading the tree named, none by execution yet.

| # | Observation | Verification | Triage |
|---|-------------|--------------|--------|
| M-1.1 **(fixed, v0.3.1: e468fba, b65b8be, f4b7fd9, 602c59b, b213260)** | `cr draft` exits 1 refusing a record whose stored evidence quotes `<!-- cr:` (a convention role reviewing draft/marker.go quoted the marker), and the hint says to edit the body in the draft, which does not exist yet; `cr triage` needs a draft and findings.ndjson is not hand-editable, so the record can never be drafted. | Measured on the measurement's state root: `cr draft 1` exit 1 on f13901 with that message, v0.3.0. §8.1.3 refuses the sequence in a *body*; at first render the body is the evidence verbatim. | Defect. Either escape or reject the sequence at `cr record` (loud, with the record id), or render the block and refuse only at post; the hint must name a step that exists. |
| M-1.2 **(fixed, v0.3.1: 3e9938b, fc11a2d, cc3dd77, 2cdcbe6, 622c8a1)** | `KeyRewriteError`'s first remedy, `cr brief <pr> --issue <recorded>`, cannot succeed in the case the error is written for (a changed `intent.key_pattern` that no longer matches the recorded key), because `ResolveKey` runs the flag through the pattern like every other source (v0.1.0 key.go:98-105, by design), so the refusal recurs; only the second remedy (remove the state directory) works. | Read at v0.1.0 (brief/rekey.go:36-38, intent/key.go:125-141) and unchanged in kind on main. **The 14 recorded audit rounds under `spec/.tp-review/0.1.0` each carry one row with `evidence_file` internal/brief/rekey.go, and all 14 read status PASS, evidence_lines 14-40, item task-issue-key-rewrite-refusal** — the lines the record flags. Raised as a finding in both rounds of the measurement (f10601). | Defect. Drop the first remedy when the pattern no longer matches the recorded key, or make `--issue` exempt from the pattern when it equals the recorded key. |
| M-1.3 **(fixed, v0.3.1: 07022c4, 11c62b4; measured exit 2 before, 3 after)** | A corrupt `rendered.json` is refused by `DecodeRendered` with a bare `fmt.Errorf`, no exit.go row claims a json error, so the command exits 2 with the usage hint instead of 3 with `state.UnusableHint` — the class D-S11-1 found for meta.json (fixed there by 98df0c8). | Read on main: rendered_decode.go:19, regenerate.go:63, exit.go rows. Raised by two roles under two classes without the intent axis (f5301, f14901) and by one with it (f5301). Not yet measured by corrupting a rendered.json. | Defect. Wrap with `state.FileFailure(..., state.UnusableHint, err)` as meta.json's reader does. |
| M-1.4 **(fixed, v0.3.1: b67157e, 2ae3615; two defects: the check, and `git worktree remove` refusing the broken tree)** | When the sandbox directory exists but is no longer a readable worktree, `Unclean` returns `git.Head`'s error before any unclean reason, so `Ensure` never reaches `recreate` and the sandbox is never rebuilt. | Read at v0.1.0 clean.go:98-101; the S10 QA note ("missing but already registered worktree" until `git worktree prune`) is the same state seen from `cr sandbox create`. Raised as a question with claims (f18401) and as a finding without. Check whether v0.2.2's recreation conditions changed it. | Defect candidate; verify on main by breaking a sandbox's `.git` file and running `cr test`. |
| M-1.5 **(real; fixed, v0.3.1: 231fca9, 51346b5)** | `copyPath` checks `sandbox.copy` entries lexically and the copy step writes through symlinks the head checks out, so a copy could land outside the sandbox. | Measured 2026-09-18: with the head tracking `.env -> <file outside both trees>`, `cr sandbox create` wrote the checkout's `.env` into the outside file; the same through a linked directory in the path. Fixed by copying through an `os.Root` on the sandbox and replacing a link at the target. | Verify with a fixture whose head holds a symlink named in `sandbox.copy`. |
| M-1.6 **(real; fixed, v0.3.1: 56208b8, f9bb9db; tail closed in v0.3.2: the survivor is disclosed by `cr test` and `cr probe run` from the runner lock it still holds, after a 2s grace)** | After the timeout kills the process group, `Run` waits on `<-finished` with no bound; `cmd.Wait` waits for the stdout copy, so a descendant that left the group holding the pipe blocks `cr test`/`cr probe run` for ever. | Measured 2026-09-18: a runner leaving `set -m; sleep 20 &` in its own group under a 500 ms budget did not return in 15 s; with `cmd.WaitDelay` (2 s) it returns in 2.64 s reporting the runner's own status as timed out. The survivor is neither killed (cr cannot signal outside the group) nor yet reported: a disclosure candidate. | Verify with a runner that spawns a `setsid` child holding stdout; bound the wait after the kill. |
| M-1.7 | Worked as designed: 0 false assertions over 6 finding records; per-prompt id blocks collision-free over 205 parallel writers; every transcript's file path under the clone; §6.4.1 did not fold two roles' records of one defect under different classes (M-1.3), which the merge's possible-duplicate list (v0.2.3, 2.4) did not list either since they share neither citation nor anchor-inside-citation. | Measured. | The last is a dedup observation for later: same anchor, different class, two roles. |
