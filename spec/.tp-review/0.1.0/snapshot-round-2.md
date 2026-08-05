# cr v0.1.0 — Reviewer Loop

First release. Delivers the complete reviewer-side loop for a single pull
request: orient, review across four axes, prove findings with experiments,
draft, human triage, post, and re-review until every thread is closed.

Design rationale and non-normative background live in `VISION.md`. This document
is normative.

## 1. Scope

### 1.1 Terms

| Term | Definition |
|------|------------|
| PR | A GitHub pull request identified by `owner/repo#number` |
| Head | The commit SHA at the tip of the PR branch at the time of a round |
| Issue | The tracker item that states the intent, identified by an issue key |
| Claim | One atomic requirement extracted from the issue |
| Hunk | One contiguous changed range in the PR diff |
| Unit | A cluster of related hunks reviewed as one piece of work |
| Role | A judgement lens with an id, a title, and prompt instructions |
| Axis | One of the four review dimensions defined in §4 |
| Cell | The intersection of one unit and one active role |
| Finding | An assertion that something is wrong, with evidence |
| Question | A request for information the reviewer cannot answer alone |
| Citation | A `path:line` reference that `cr` resolved against the current head |
| Probe | A reproducible experiment run in a sandbox, with a recorded result |
| Grade | The strength of a finding's evidence: probed, cited, or argued |
| Draft | A human-editable rendering of a round's queued output |
| Thread | A posted GitHub review comment and its replies |
| Round | One review pass, bound to a single head SHA |
| Waiver | A recorded decision to never raise a given finding again |
| Disposition | Why a record was discarded: `wrong` or `not-here` (§7.2) |
| Note | An out-of-band fact recorded against an issue key |
| Open | Any record in a non-terminal state per §9.1 |

### 1.2 In scope

1. Reviewing a pull request the user did not author.
2. Reading intent from a tracker through an existing CLI command.
3. Ingesting existing review threads from other reviewers.
4. Running experiments against the PR head in an isolated worktree.
5. Producing, triaging, and posting review comments as a single GitHub review.
6. Re-reviewing after the author pushes, and resolving threads.

### 1.3 Out of scope

1. Writing to the tracker.
2. Author-side workflow: consuming review comments on the user's own PRs.
3. Approving or requesting changes automatically.
4. Any call to a language model from within `cr`.
5. Chat platform integrations.

### 1.4 Normalisation

Several requirements hash or compare text. Wherever this document says
**normalised**, it means exactly this transformation and no other:

1. Decode as UTF-8; invalid input MUST fail with exit code 1.
2. Replace CRLF and lone CR with LF.
3. Strip trailing whitespace from every line.
4. Replace every run of horizontal whitespace inside a line with one space.
5. Drop leading and trailing blank lines, and collapse interior runs of blank
   lines to one.

A **normalised hash** is SHA-256 over the normalised text, lowercase hex,
truncated to the first 16 characters. Normalisation MUST NOT case-fold, MUST NOT
strip punctuation, and MUST NOT reorder lines.

### 1.5 Axis identifiers

The axis set is closed in v0.1. Valid axis ids are exactly `intent`,
`correctness`, `convention`, and `test`, defined in §4.1 through §4.4. Any
`axis` field in a profile, role, or rule MUST name one of these; any other value
MUST abort the command with exit code 3.

### 1.6 Comment economy

A review comment spends the reviewer's standing with the author. Volume is
therefore a cost in itself, independent of correctness.

1. `cr` MUST prefer a line-anchored comment. A top-level review comment
   (`side: NONE` per §9.2) is permitted only where no code location exists,
   which in v0.1 is the unimplemented-claim item of §4.1.3.
2. `post.max_comments` (default 20) MUST cap the comments in one round. When the
   cap is exceeded `cr` MUST block posting with exit code 1 and name the count,
   so the user triages further. `cr` MUST NOT silently drop comments to fit.
3. The count of top-level comments MUST be reported in the round summary, so the
   exception of §1.6.1 stays visible and rare.

## 2. Architecture

### 2.1 Division of labour

`cr` is deterministic. It fetches, clusters, executes, validates, records, and
reports. It never calls a language model and never decides whether code is
correct.

The agent judges. It reads code, writes findings, composes prose, and decides
dispositions. It reaches `cr` through the commands in §11 and the skill in §13.

1. Every `cr` command MUST be reproducible given the same state directory, the
   same head SHA, and the same inputs.
2. `cr` MUST NOT perform any network write except the GitHub calls in §8, and
   those only behind the confirmation gate of §8.5.
3. The following judgements belong to the agent alone, and `cr` MUST NOT make
   them: whether a rule hit is a real violation, whether a candidate symbol is
   semantic reinvention, whether a note explains an unmapped unit, whether an
   existing human thread already covers a finding, whether a unit is covered by
   tests, and whether a posted concern has been addressed. For each of these `cr`
   locates the candidate, supplies it, and records the decision.

### 2.2 State layout

All state lives under `~/.cr/`. `cr` MUST NOT write inside the repository under
review, with the single exception of git worktree registration metadata under
`.git/worktrees/` created by §5.1. `cr` MUST NOT modify the repository's tracked
files, index, HEAD, stash, or any branch.

| Path | Contents |
|------|----------|
| `~/.cr/config.json` | Global defaults |
| `~/.cr/profiles/<id>.json` | Mechanical profiles (§2.4) |
| `~/.cr/roles/<id>.json` | Judgement roles (§2.5) |
| `~/.cr/rules/<id>.json` | Global rules (§2.6) |
| `~/.cr/repos/<owner>/<repo>/config.json` | Per-repository overrides |
| `~/.cr/repos/<owner>/<repo>/roles/<id>.json` | Per-repository role overrides |
| `~/.cr/repos/<owner>/<repo>/rules/<id>.json` | Per-repository rule overrides |
| `~/.cr/repos/<owner>/<repo>/triage.ndjson` | Triage events across PRs (§7.3) |
| `~/.cr/repos/<owner>/<repo>/rule-stats.ndjson` | Per-rule hits, records, dismissals (§2.6.1) |
| `~/.cr/state/<owner>/<repo>/pr-<n>/` | Per-PR state (§2.3) |
| `~/.cr/context/<ISSUE-KEY>.ndjson` | Out-of-band context store (§3.6) |
| `~/.cr/waivers/<owner>/<repo>.ndjson` | Repository-wide waivers (§7.4) |
| `~/.cr/locks/` | Advisory locks (§5.6) |

### 2.3 Per-PR state

| File | Contents |
|------|----------|
| `meta.json` | PR identity, issue key, profile id, active roles, round index |
| `claims.ndjson` | Extracted claims (§3.3) |
| `units.ndjson` | Clustered units (§3.4) |
| `threads.ndjson` | Ingested and created threads (§3.5) |
| `findings.ndjson` | All findings and questions, all states (§6.1) |
| `probes.ndjson` | Probe records (§5.5) |
| `coverage.ndjson` | Coverage cells (§4.5) |
| `transitions.ndjson` | State transitions (§9.1) |
| `rounds/<n>/draft.md` | The editable draft for round n (§7.1) |
| `rounds/<n>/rendered.json` | Bodies exactly as `cr` rendered them (§7.1) |
| `rounds/<n>/posted.json` | The exact payload posted for round n |
| `rounds/<n>/summary.json` | Round summary counts (§10.3) |

1. All writes to per-PR state MUST take an exclusive advisory file lock.
2. Reads MUST be lock-free.
3. Every record in `claims.ndjson`, `units.ndjson`, `findings.ndjson`,
   `probes.ndjson`, and `coverage.ndjson` MUST carry a `head` field naming the
   head SHA it was produced against.

### 2.4 Profiles

A profile is mechanical, language-specific configuration. It is data, never
prompt text.

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `id` | string | yes | Profile id, equal to the file stem |
| `match.files` | array | yes | Marker files selecting this profile; empty means never auto-selected |
| `match.globs` | array | yes | Source globs this profile owns |
| `axes` | object | yes | Default enabled state per axis id of §1.5 |
| `sandbox.copy` | array | no | Paths copied from the main worktree into the sandbox |
| `sandbox.setup` | array | no | Commands run once after sandbox creation |
| `tests.cmd` | array | no | Test runner argv; absent disables the test axis |
| `tests.filter_flag` | string | no | Flag used to narrow the run to a subset |
| `tests.timeout_seconds` | integer | no | Per-run timeout, default 900 |
| `tests.output_tail_bytes` | integer | no | Bytes of runner output retained, default 4096 |
| `tests.count_pattern` | string | no | Regex with one capture group yielding the executed test count |
| `symbols.lang` | string | no | Language hint for the reinvention search of §4.3 |

1. Profile selection MUST be automatic via `match.files`, and overridable by
   `profile` in the per-repository config.
2. When more than one profile matches, `cr` MUST select the one with the greatest
   number of matched marker files; on a tie it MUST abort with exit code 3 and
   name the tied profiles rather than picking one.
3. The `generic` profile MUST declare an empty `match.files` and therefore MUST
   NOT be selected automatically. It applies only when named by configuration.
4. If no profile matches, `cr` MUST report the situation and disable every axis
   that requires one, rather than guessing.
5. v0.1 MUST ship two profiles: `laravel-pest` and `generic`.

### 2.5 Roles

A role customises the prompt for one axis. `cr` owns the output contract; a role
only supplies persona and focus.

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `id` | string | yes | Role id, equal to the file stem, kebab-case |
| `title` | string | yes | Human label shown in prompts and progress output |
| `axis` | string | yes | The axis this role serves, from §1.5 |
| `instructions` | string | yes | The role's framing text |
| `focus` | array | no | Focus questions appended to the prompt |
| `profiles` | array | no | Profiles this role applies to; empty means all |

1. v0.1 MUST ship four default roles, one per axis: `intent-coverage` on
   `intent`, `correctness` on `correctness`, `convention` on `convention`, and
   `test-adequacy` on `test`.
2. `cr init --eject-roles` MUST write the defaults as editable files that are
   byte-identical to the built-ins.
3. A malformed role or profile file MUST abort the command with exit code 3,
   naming the file and the offending field.
4. Role resolution order MUST be per-repository, then global, then built-in.
5. More than one role MAY serve the same axis. **Corpus order**, used by §6.4.2,
   is resolution layer first (per-repository before global before built-in), then
   ascending lexicographic role id.

### 2.6 Rules

A rule is one normative statement about how code in a repository must be
written. Rules are data and are kept separate from roles: a role is a lens, a
rule is a specific standard that lens enforces.

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `id` | string | yes | Rule id, equal to the file stem, kebab-case |
| `title` | string | yes | One-line statement of the standard |
| `rationale` | string | yes | Why the standard exists, quotable to the author |
| `axis` | string | no | The axis that enforces it, default `convention` |
| `class` | string | yes | Defect class assigned to records from this rule |
| `severity` | string | no | Default severity for records from this rule, default `medium` |
| `kind` | string | no | Default `finding` or `question`, default `question` |
| `detect` | object | no | Optional mechanical detector per §2.6.1 |
| `fix` | object | no | Optional suggestion template per §2.6.2 |
| `globs` | array | no | Paths the rule applies to; empty means all source |
| `exempt` | array | no | Paths excluded, for legacy areas |
| `profiles` | array | no | Profiles the rule applies to; empty means all |

1. Rules MUST resolve in layers, highest first: per-repository, then global, then
   the profile's built-in rules.
2. A rule id MUST be unique after resolution; a lower layer carrying the same id
   is overridden whole, never merged field by field.
3. Every record produced by a rule MUST carry the rule id.
4. A record produced by a rule MUST be able to quote the rule's `rationale`, so
   the author learns the standard and not only the violation.
5. A malformed rule file MUST abort the command with exit code 3.

#### 2.6.1 Detection

1. A rule carrying a `detect` block MUST be evaluated mechanically by `cr` over
   the added and modified `RIGHT`-side lines of the diff only — never over
   removed lines, and never over the whole repository.
2. `detect.pattern` MUST be a regular expression in Go `regexp` syntax and
   `detect.mode` MUST be `regex` in v0.1. A pattern that fails to compile MUST
   abort with exit code 3, naming the rule.
3. A hit MUST grade the resulting record `cited` per §6.2, with the rule id and
   the matched `path:line` stored as the citation.
4. A rule without a `detect` block MUST be injected into its axis role's prompt as
   text, and any record it produces is graded normally per §6.2.
5. Detection MUST report hits, never verdicts. The agent decides whether a hit is
   a real violation and MAY dismiss it with a recorded reason.
6. Every hit, record, and dismissal MUST be appended to `rule-stats.ndjson` with
   the rule id, the round, and the head, so an imprecise pattern is visible in the
   statistics of §7.3.

#### 2.6.2 Fixes

1. A rule MAY carry `fix.replace` and `fix.with` as a regular expression and its
   replacement, applied to the matched line to produce a suggestion.
2. A generated suggestion MUST pass the validation of §8.2 before drafting; a
   suggestion that fails validation MUST be dropped while its record survives.
3. `cr` MUST NOT apply a fix to any file. A fix only ever produces suggestion text
   for the author to accept.
4. `cr` cannot establish that a generated replacement compiles, parses, or
   preserves behaviour. A suggestion produced by `fix` MUST carry
   `suggestion_origin: rule`, MUST be labelled as machine generated in the draft,
   and MUST be dropped when the agent does not confirm it.

#### 2.6.3 Harvesting

1. `cr rules suggest --repo <owner/repo>` MUST scan the bodies of comments posted
   from recorded rounds and group them by class and by normalised body per §1.4.
2. A group reaching `rules.harvest_min` occurrences, default 3, MUST be reported
   as a candidate rule together with the comments that formed it.
3. Candidates MUST be reported only. `cr` MUST NOT write a rule file by itself.
4. `cr rules list --dead --repo <owner/repo>` MUST report rules that produced no
   hit and no record across the last `rules.dead_after` rounds, default 20, so the
   corpus can be pruned rather than growing without limit.

### 2.7 Configuration layers

Effective configuration MUST resolve at read time in this order, highest first:

1. Command-line flags.
2. Environment variables prefixed `CR_`.
3. Per-repository config at `~/.cr/repos/<owner>/<repo>/config.json`.
4. Global config at `~/.cr/config.json`.
5. Built-in defaults.

`cr config` MUST print the effective configuration, and `cr config --resolved`
MUST annotate every setting with the layer that supplied it.

The confirmation gate of §8.5 and the argued forcing of §6.3 are not settings.
They MUST NOT be readable from any layer, and `cr` MUST reject and report any
`CR_`-prefixed variable or config key whose name would address either.

## 3. Inputs

### 3.1 Intent source

The tracker is read through a configured command, so `cr` carries no tracker
authentication code.

1. `intent.cmd` MUST be an argv array with a `{key}` placeholder.
2. The default MUST be `["jira", "issue", "view", "{key}", "--plain"]`.
3. A non-zero exit MUST fail with exit code 3 and surface the command's stderr.
4. `--intent-file <path>` MUST bypass the command and read the issue text from a
   file, so the loop works with no tracker access at all.

### 3.2 Issue key resolution

The issue key MUST be resolved from the first source that yields a match:

1. The `--issue <KEY>` flag.
2. The PR branch name.
3. The PR title.
4. The PR body.

The pattern MUST default to `[A-Z][A-Z0-9]+-[0-9]+` and be overridable by
`intent.key_pattern`. If no key is found, `cr` MUST continue with an empty intent
and mark the intent axis unavailable per §4.5.

### 3.3 Claim extraction

Claims are produced by the agent from the issue text and recorded by `cr`.

| Field | Required | Meaning |
|-------|----------|---------|
| `id` | yes | `<ISSUE-KEY>#c<n>` |
| `text` | yes | The claim, verbatim or minimally normalised |
| `source` | yes | `description`, `acceptance`, `comment`, or `note` |
| `span` | yes | The verbatim substring of the issue text the claim was drawn from |
| `span_hash` | computed | Normalised hash of `span`, written by `cr` |
| `issue_hash` | computed | Normalised hash of the whole issue text at extraction, written by `cr` |
| `head` | yes | Head SHA the extraction ran against |

1. `cr claims record <pr> <file.ndjson>` MUST validate and store claims. `cr` MUST
   compute `span_hash` and `issue_hash` itself, and MUST reject with exit code 1
   any claim whose `span` does not occur in the issue text.
2. Claims sourced from the context store (§3.6) MUST carry `source: note`, MUST
   set `span` to the note body, and MUST be included in coverage exactly like
   tracker claims.
3. Drift MUST be detected by re-reading the issue text at the start of every round
   and comparing its normalised hash to the stored `issue_hash`. When they differ,
   `cr` MUST report for each claim whether its `span` still occurs in the new issue
   text. `cr` MUST NOT re-extract; re-extraction happens only through a new
   `cr claims record`.

### 3.4 Diff ingestion and unit clustering

1. The diff MUST be taken against the PR merge base at the current head.
2. Files matching `ignore.globs` MUST be excluded and counted as excluded.
3. An enclosing symbol is **detectable** only when the profile declares
   `symbols.lang` and `cr` can build a symbol index for the file. When it cannot,
   clustering MUST fall through to adjacency without reporting an error.
4. Hunks MUST be clustered into units by, in order: same file and same enclosing
   symbol when detectable; otherwise same file and **adjacency**, where two hunks
   are adjacent when the gap between the last changed line of one and the first
   changed line of the next is at most `cluster.gap_lines` (default 12); otherwise
   one unit per hunk.
5. A cluster exceeding `cluster.max_lines` changed lines (default 80) MUST be
   split at hunk boundaries into the fewest units that each stay within the limit.
   A single hunk that alone exceeds the limit MUST become one unit, MUST be
   flagged `oversized`, and MUST appear as such in the coverage report; it is the
   one case where the limit does not bind.
6. Every unit MUST record its file paths, hunk ranges, changed line count, and
   whether it was formed by symbol, by adjacency, or by fallback.
7. Binary and generated files MUST be listed but not clustered.

### 3.5 Existing threads

1. All existing review threads on the PR MUST be ingested, including resolved
   ones, with author, body, anchor, resolution state, and replies.
2. Each thread MUST be tagged with an author type of `human` or `bot`.
3. `cr` MUST attach to every unit the ingested human threads whose anchor falls
   inside the unit's hunks or within `threads.proximity_lines` (default 10) of
   them. `cr` MUST NOT assign a class to an ingested thread and MUST NOT decide
   suppression itself.
4. The agent decides whether an attached thread already covers a finding. When it
   does, the finding MUST be recorded in state `suppressed` carrying
   `suppressed_by` with the thread id, and MUST NOT be drafted.
5. Author replies inside ingested threads MUST be offered as candidate context
   notes per §3.6.

### 3.6 Context store

The context store holds facts that are true about the issue but absent from the
tracker.

1. `cr note <ISSUE-KEY> "<text>" --source <source>` MUST append a note with an id
   of the form `<ISSUE-KEY>#n<n>`, a timestamp, the PR it came from, and the
   source.
2. `cr answer <pr> <question-id> "<text>" --source <source>` MUST set the
   question's state to `answered` and append the same note in one call.
3. Valid sources MUST be `chat`, `jira`, `thread`, `meeting`, and `other`.
4. Notes for the issue key MUST be loaded automatically on every subsequent round
   and every subsequent PR that resolves to the same key.
5. `cr context <ISSUE-KEY>` MUST print the accumulated notes with provenance.
6. A note is unverified hearsay and MUST stay revocable.
   `cr note --remove <note-id>` MUST retract one, and any coverage cell or record
   citing a retracted note MUST be reported as needing re-evaluation in the next
   round rather than silently retained.

## 4. Review axes

### 4.1 Intent coverage

1. Every unit MUST be mapped to zero or more claims.
2. A unit mapped to zero claims MUST raise an unmapped-unit item.
3. A claim mapped to zero units MUST raise an unimplemented-claim item. Such an
   item has no code location by construction, MUST carry an anchor with
   `side: NONE` per §9.2, and posts as a top-level comment — the only such case in
   v0.1 per §1.6.1.
4. An unmapped unit MUST default to a question, never a finding, because the most
   common cause is intent that never reached the tracker.
5. `cr` MUST attach the issue key's notes to every unmapped unit. The agent
   decides whether a note explains the unit; when it does, the item MUST NOT be
   raised and the coverage cell MUST cite the note id. `cr` MUST NOT decide this
   by matching text.

### 4.2 Correctness against claims

1. Each unit MUST be evaluated against the claims it is mapped to.
2. A correctness finding MUST cite the claim id it violates.
3. A unit mapped to zero claims MUST still be evaluated for internal defects.

### 4.3 Convention and reinvention

1. For every function, method, or class added by the diff, `cr` MUST search the
   repository for candidate pre-existing symbols and attach the candidates to the
   unit. This needs a symbol index built from the profile's `symbols.lang`; when
   none can be built, the reinvention half of this axis MUST be marked unavailable
   per §4.5 rather than skipped silently.
2. Candidate ranking MUST use a normalised-name similarity — case-folded, with `_`
   and `-` removed, scored by Levenshtein ratio — combined with equality of
   declared parameter count. `cr` MUST attach at most
   `reinvention.max_candidates` (default 5) candidates scoring at least
   `reinvention.min_similarity` (default 0.6). Semantic equivalence is the agent's
   judgement, not `cr`'s.
3. A reinvention item MUST cite the candidate symbol as `path:line`.
4. Reinvention items MUST default to `kind: question`, because the tool cannot
   know whether the existing symbol was rejected for a reason.
5. Project conventions beyond reinvention MUST come from the rule corpus of §2.6,
   never from hard-coded logic and never from prose buried inside role
   instructions.
6. A mechanical rule hit MUST be attached to the unit that contains it, so the
   agent judges an already-located candidate instead of searching for one.

### 4.4 Test adequacy

1. `cr` MUST attach to every unit the test files changed or added by the PR and
   the symbols they reference. The agent MUST then classify the unit as covered,
   partially covered, or uncovered, and the classification MUST be recorded in the
   coverage cell together with the test paths it rested on.
2. For every claim the agent MUST enumerate the expected edge cases it can support
   from the claim text, and mark each asserted, missing, or not applicable with a
   reason. `cr` MUST cap the enumeration at `test.max_edge_cases` per claim
   (default 10) and MUST report the cap when it binds.
3. A missing edge case SHOULD be escalated to a probe per §5.
4. A test-adequacy finding without a probe MUST be graded `argued` and therefore
   posted as a question per §6.3.

### 4.5 Axis activation and honest reporting

1. An axis is active when it is enabled by the resolved configuration and its
   prerequisites are met.
2. The test axis MUST be disabled automatically when the profile declares no
   `tests.cmd`.
3. The intent axis MUST be marked unavailable when no issue key resolves.
4. A disabled or unavailable axis MUST appear in the coverage report with its
   reason. `cr` MUST NOT report a review as complete or converged without stating
   which axes did not run.
5. Every cell MUST carry one of `pass`, `finding`, `question`, or `na`, the head it
   was filled against, and a reason when it is `na`.

## 5. Probes

### 5.1 Sandbox

1. `cr sandbox create <pr>` MUST create a git worktree at the PR head under
   `~/.cr/state/<owner>/<repo>/pr-<n>/sandbox/`. The worktree registration under
   the repository's `.git/worktrees/` is the sole write permitted by §2.2.
2. Paths listed in `sandbox.copy` MUST be copied from the main checkout into the
   sandbox after creation.
3. Commands in `sandbox.setup` MUST run once, in order, in the sandbox root.
4. `cr` MUST NOT modify the user's main worktree, index, or current branch.
5. `cr sandbox destroy <pr>` MUST remove the worktree and its registration.
6. Before every probe or test run, `cr` MUST verify that the sandbox HEAD equals
   the PR head and that the sandbox working tree is clean. A sandbox failing
   either check MUST be recreated, and the recreation MUST be reported.
7. A probe whose post-run cleanliness check fails MUST be recorded with result
   `error`, MUST NOT grade any finding, and MUST force recreation before the next
   run.

### 5.2 Test runs

1. `cr test <pr> [--filter <expr>]` MUST run the profile's test command inside the
   sandbox and record exit code, duration, the executed test count when
   `tests.count_pattern` is configured, and the last `tests.output_tail_bytes` of
   output.
2. A baseline run with no filter MUST be recorded once per head, so pre-existing
   failures are never attributed to a probe. A filtered probe MUST additionally
   record a filtered baseline for the same filter on unmutated code.
3. A run exceeding `tests.timeout_seconds` MUST be killed and recorded as
   `timeout`.

### 5.3 Mutation probe

A mutation probe proves a test gap by breaking production code.

1. The agent supplies a mutation as a unified diff against a sandbox file.
2. `cr probe run <pr> --kind mutation --patch <file> --filter <expr>` MUST apply
   the mutation, run the tests, revert the mutation, and record the result.
3. The mutation MUST be reverted even when the run fails, times out, or `cr` is
   killed. On the next invocation the check of §5.1.6 MUST detect an unclean
   sandbox and recreate it before running anything.
4. A result of `no-test-failed` proves the gap and grades the finding `probed`,
   but only when both hold: the executed test count is greater than zero, and the
   filtered baseline of §5.2.2 passed. When the count cannot be determined because
   the profile declares no `tests.count_pattern`, the result MUST be recorded
   `inconclusive` and MUST NOT grade a finding `probed`.
5. A run that selected no tests MUST be recorded `no-tests-selected`, never
   `no-test-failed`, so an over-narrow filter cannot manufacture evidence.
6. A result of `failed` disproves the gap; `cr` MUST record it and the agent MUST
   NOT raise the finding.

### 5.4 Gap probe

A gap probe distinguishes a missing behaviour from a missing test.

1. The agent supplies a new test file targeting the suspected edge case.
2. `cr probe run <pr> --kind gap --test <file> --filter <expr>` MUST place the test
   in the sandbox under `probe.test_dir` (default the profile's first test glob
   root) with a name derived from the probe id, run it, remove it, and record the
   result. An existing file at that path MUST abort with exit code 4. The removal
   MUST happen even when the run fails or times out, and §5.1.6 catches the case
   where it did not.
3. A failing gap probe means either the behaviour is wrong or the supplied test is
   wrong, and `cr` cannot distinguish the two. The result MUST be recorded and the
   probe output offered as a reproduction. Severity MUST be at least `high` only
   when the agent records that the test asserts a specific claim id; otherwise the
   finding MUST be graded `argued` and posted as a question.
4. A passing gap probe means only the test is missing; severity MUST be at most
   `medium`.

### 5.5 Probe record

| Field | Required | Meaning |
|-------|----------|---------|
| `id` | yes | `p<n>` |
| `kind` | yes | `mutation` or `gap` |
| `head` | yes | Head SHA the probe ran against |
| `target` | yes | `path:line` the probe addresses |
| `input` | yes | The patch or test file content |
| `filter` | no | The test filter used |
| `result` | yes | A value from the per-kind table below |
| `tests_run` | no | Executed test count when derivable |
| `baseline` | no | Id of the filtered baseline it was compared against |
| `duration_ms` | yes | Wall-clock duration |
| `output_tail` | yes | Runner output truncated to `tests.output_tail_bytes` |

The `result` vocabulary is per kind and MUST NOT be shared:

| Kind | Valid results |
|------|---------------|
| `mutation` | `no-test-failed`, `failed`, `no-tests-selected`, `inconclusive`, `error`, `timeout` |
| `gap` | `passed`, `failed`, `no-tests-selected`, `error`, `timeout` |

1. Probe records MUST be immutable once written.
2. A finding MUST reference at most one probe, by id.
3. Probe records whose head differs from the current head MUST NOT be used to grade
   a finding in the current round.

### 5.6 Isolation and budget

1. Probe and test runs MUST take an advisory lock named after the absolute path of
   the repository under review together with the profile id, so two `cr` runs never
   share a test database and two unrelated repositories never block each other.
2. When the lock is held, `cr` MUST wait up to `probe.lock_timeout_seconds`
   (default 300) and then fail with exit code 4.
3. `cr` MUST warn that an unrelated local test run can still collide, because the
   lock only covers `cr`'s own runs.
4. `probe.max_per_round` (default 10) MUST cap probe executions per round; the cap
   being hit MUST be reported, never silently applied.

## 6. Findings

### 6.1 Record schema

| Field | Required | Meaning |
|-------|----------|---------|
| `id` | yes | `f<n>`, stable for the life of the PR |
| `kind` | yes | `finding` or `question` |
| `axis` | yes | The axis that produced it |
| `role` | yes | The role that produced it |
| `class` | yes | Kebab-case defect class, used for dedup and statistics |
| `rule` | no | The rule id, when a rule produced the record |
| `severity` | yes | `critical`, `high`, `medium`, or `low` |
| `grade` | computed | `probed`, `cited`, or `argued`; written by `cr` per §6.2 |
| `unit` | yes | The unit id |
| `claim` | no | The claim id, when the axis produces one |
| `anchor` | yes | Anchor object per §9.2 |
| `summary` | yes | One sentence, English, structured for dedup |
| `evidence` | yes | Prose explaining what supports it |
| `citations` | no | Array of `{path, line, content_hash}`; the machine-readable input to grading |
| `probe` | no | Probe id, when graded `probed` |
| `suggestion` | no | Exact replacement lines |
| `suggestion_origin` | no | `agent` or `rule` |
| `state` | yes | State per §9.1 |
| `disposition` | no | `wrong` or `not-here` once discarded (§7.2) |
| `duplicate_of` | no | Representative record id when suppressed as a duplicate |
| `suppressed_by` | no | Thread id when suppressed per §3.5.4 |
| `regression_of` | no | Original record id when this records a regression |
| `thread_id` | no | GitHub thread id once posted |
| `round` | yes | The round that produced it |
| `head` | yes | Head SHA the record was produced against |

1. `summary` and `evidence` MUST be English. Reader-facing prose is produced at
   draft time per §8.1.
2. A finding MUST carry an anchor that resolves to a line in the current head,
   unless its anchor carries `side: NONE` per §9.2.
3. `cr merge` and `cr record` MUST reject a record missing any field marked
   required, with exit code 1, naming the line and the field.
4. The agent MUST NOT write `grade`, `span_hash`, `issue_hash`, or any other field
   marked computed. A record arriving with one MUST be rejected with exit code 1.

### 6.2 Evidence grades

| Grade | Requirement |
|-------|-------------|
| `probed` | References a probe whose head matches and whose result supports the claim per §5.3 and §5.4 |
| `cited` | Carries at least one entry in `citations` that `cr` resolved against the current head and that lies outside the record's own unit, or carries a `rule` id whose detector produced the match |
| `argued` | Neither of the above |

1. The grade MUST be computed by `cr` from the record, never asserted by the agent.
   `citations`, `probe`, and `rule` are the only inputs; `evidence` prose is never
   parsed.
2. A record claiming `probed` without a valid same-head probe MUST be rejected with
   exit code 1.
3. `cr` MUST resolve every entry in `citations` against the current head. An entry
   whose path does not exist, whose line is out of range, or whose recomputed
   normalised line hash does not match the stored `content_hash` MUST cause
   rejection with exit code 1. `cr` computes and stores `content_hash` itself.
4. `cr` cannot judge whether a citation supports the summary, and MUST NOT claim
   to. Every citation of a `cited` record MUST be rendered verbatim into the
   draft block, so the human makes the judgement `cr` cannot.

### 6.3 The argued rule

1. A record with grade `argued` MUST be forced to `kind: question`. The forcing
   MUST be applied at record time, again at draft time, and again at post time
   immediately before the payload is built.
2. The forcing MUST be visible in the round summary, with a count per class.
3. There MUST be no flag, configuration setting, environment variable, profile
   field, or role instruction that disables or overrides the forcing. `cr` MUST
   reject any attempt to post a record graded `argued` with `kind: finding`, with
   exit code 1, naming the record id.

### 6.4 Dedup

1. Findings MUST be deduplicated by `(anchor.path, anchor.line, class)`.
2. Within a duplicate group the representative MUST be the highest grade, then the
   highest severity, then the earliest role in corpus order per §2.5.5.
3. Suppressed duplicates MUST be retained in state `duplicate` with `duplicate_of`
   naming the representative, and the contributing roles MUST be reported as an
   overlap summary.
4. Findings matching an active waiver MUST be dropped and counted per §7.4.

### 6.5 Merge

1. `cr merge <files...> -o <out> --repo <owner/repo>` MUST merge per-role NDJSON
   files, apply §6.4, and report counts by role, axis, severity, and grade. The
   repository is required because §6.4.4 needs its waivers and §6.4.2 needs its
   resolved role corpus.
2. Missing required fields MUST fail with exit code 1 and name the offending line.

## 7. Draft and triage

### 7.1 Draft format

`cr draft <pr>` MUST render every queued record into a single Markdown file at
`rounds/<n>/draft.md`.

1. Each record MUST be rendered as a block introduced by an HTML comment marker
   carrying `id`, `kind`, `path`, `line`, `severity`, `grade`, and `disposition`.
2. The block body MUST be free-form Markdown that the user may rewrite entirely.
3. A suggestion MUST be rendered as a fenced `suggestion` block inside the body,
   and a suggestion with `suggestion_origin: rule` MUST be labelled as machine
   generated.
4. The file MUST open with a summary header listing counts, the coverage state,
   and the comment count against `post.max_comments`, as comments that are not
   posted.
5. `cr` MUST write every body exactly as rendered to `rounds/<n>/rendered.json`,
   together with the normalised hash of the whole draft file. This is the only
   observable basis for detecting a user edit.
6. The draft MUST be regenerable. Regeneration MUST first ingest the current
   draft's triage state per §7.2 — including deletions, which become discards and
   waivers at that moment — and MUST then preserve bodies for records whose body
   differs from `rendered.json`. Regeneration MUST NOT resurrect a deleted block.

### 7.2 Triage verbs

Triage happens by editing the file. `cr draft` and `cr post` interpret the result.

| User action in `draft.md` | Effect |
|---------------------------|--------|
| Leaves a block unchanged | Posted as rendered |
| Edits the body prose | Posted as edited |
| Changes `kind=finding` to `kind=question` | Softened, and recorded as a triage event |
| Deletes the block entirely | Discarded with disposition `not-here`, waiver written per §7.4 |
| Sets `disposition=wrong`, then deletes the block | Discarded as a false positive, waiver written, counted against the class per §7.3 |
| Adds a block with `id=new` | Posted as a manual comment owned by the user |

The two discard dispositions are deliberately distinct. `not-here` means the
finding is true but not worth a comment on this pull request — the ordinary case
under §1.6 — and MUST NOT count against the class's precision. `wrong` means the
finding is false, and is the only signal that should demote a class. Collapsing
them would demote classes that are always right and merely never worth saying.

Marker fields have these edit semantics; any other edit MUST abort with exit
code 1, naming the record id:

| Marker field | Edit semantics |
|--------------|----------------|
| `id` | Immutable; a changed or unknown id aborts |
| `kind` | `finding`→`question` softens; `question`→`finding` is accepted only when the recomputed grade is `probed` or `cited`, and aborts otherwise per §6.3.3 |
| `path`, `line` | Re-validated against the current head; aborts when the anchor no longer resolves |
| `severity` | Freely editable; recorded as a triage event |
| `disposition` | `wrong` or `not-here`; meaningful only on a deleted block |
| `grade` | Immutable; `cr` recomputes it and aborts on a mismatch |

1. `cr post` MUST refuse to run when a marker is malformed, naming the line.
2. `cr post` MUST recompute every record's grade and re-apply §6.3 before building
   the payload, so no draft edit can turn an `argued` record into a posted
   assertion.
3. A block with `id=new` MUST carry `path` and `line`. `cr` MUST record it with
   `role: human`, `axis: manual`, `class: manual`, and no grade. It is exempt from
   §6.3 because a human authored it, and MUST be excluded from the statistics of
   §7.3 and from waiver generation.

### 7.3 Triage statistics

1. Every triage event MUST be appended to the repository's `triage.ndjson` with
   class, axis, role, grade, rule id when present, disposition when present, the
   PR, the round, and the action taken.
2. `cr stats --repo <owner/repo>` MUST report per class and per rule: raised, kept,
   softened, discarded as `not-here`, and discarded as `wrong`.
3. The demotion rate for a class MUST be computed as
   `(discarded-wrong + softened) / raised` over that class's events in the
   repository's `triage.ndjson`. Discards dispositioned `not-here` MUST be excluded
   from the numerator, because they carry no evidence that the class is imprecise.
   A class whose rate exceeds `stats.demote_threshold` (default 0.6) over at least
   `stats.min_samples` (default 8) raised events MUST be listed as a demotion
   candidate.
4. A class whose `not-here` rate exceeds `stats.demote_threshold` over the same
   sample MUST be reported separately as a **volume candidate**: it is accurate but
   rarely worth posting, and the remedy is to stop raising it, not to soften it.
5. Both candidacies are reports to the user, not automatic changes. `cr` MUST NOT
   alter a rule's `kind` by itself.

### 7.4 Waivers

1. A waiver MUST be keyed by `(class, rule id when present, normalised hash of the
   anchored lines)` and scoped to the repository, not the PR. The summary is
   deliberately excluded from the key: it is agent-composed prose that differs
   between rounds, and including it would let a waived finding resurface under a
   reworded summary.
2. The key is narrow on purpose. It suppresses the same class at the same unchanged
   code, and stops suppressing once that code changes — which is exactly when the
   judgement behind the waiver should be revisited.
3. A waiver MUST record its `disposition` per §7.2, so §7.3 can tell a false
   positive from a deliberate silence.
4. A waiver for a record produced by a rule MAY be widened to a path prefix, so a
   rule can be exempted for a legacy area without disabling it repository-wide.
5. Waived findings MUST be dropped before drafting and counted in the round summary.
6. `cr waivers list --repo <owner/repo>` MUST print active waivers, and
   `cr waivers remove <id> --repo <owner/repo>` MUST delete one.
7. A waiver MUST record the round, the PR, the head, and the reason when one was
   given.

## 8. Posting

### 8.1 Render contract

1. Comment bodies MUST be written in the language configured by `render.lang`,
   defaulting to `tr`.
2. `cr` MUST NOT translate or compose. The agent composes the body; `cr` validates,
   frames, and stores it.
3. A body MUST be non-empty and MUST NOT contain the marker comment.
4. `cr` MUST prepend a fixed, `cr`-owned label line to every `kind=question`
   comment, drawn from `render.question_label` — a configured template string,
   never model output — so the reader sees the register §6.3 assigned. The grade
   MUST be stated in the same line.
5. `cr` MUST refuse to post a `kind=question` body containing no `?` character,
   with exit code 1 naming the record id. The register of a question is not
   cosmetic: without it, §6.3's forcing changes only a field the reader never sees.

### 8.2 Suggestion validation

1. A suggestion MUST map to a contiguous line range that exists in the PR diff on
   the `RIGHT` side.
2. The range MUST be within one hunk.
3. `cr` MUST NOT infer intent about indentation. When the leading whitespace of the
   suggestion's first line differs from that of the first replaced line, `cr` MUST
   warn and show both, and MUST still post when the user leaves the block in place.
4. A suggestion failing validation MUST block posting with exit code 1 and name the
   record id.

### 8.3 Batching

1. All comments in a round MUST be posted as one GitHub review, so the author
   receives a single notification.
2. The review event MUST be `COMMENT` in v0.1. `cr` MUST NOT emit `APPROVE` or
   `REQUEST_CHANGES`.
3. The exact posted payload and its normalised hash MUST be written to
   `rounds/<n>/posted.json` before the network call, and updated with returned
   thread ids after it.

### 8.4 Partial failure

1. The GitHub review-creation call is atomic: it either creates the review with all
   its comments or creates nothing. `cr` MUST therefore pre-validate every comment
   position per §8.2 before the call.
2. If the call is rejected, no state MUST be marked posted. `cr` MUST report every
   position GitHub named invalid, with the record id it belongs to, and exit 4.
3. If the outcome is unknown — a timeout, a dropped connection, or a response `cr`
   cannot parse — `cr` MUST mark the round `post-unresolved` and MUST NOT retry
   automatically. `cr post <pr> --reconcile` MUST query the PR for a review matching
   the stored payload hash and either adopt it as posted or clear the state for a
   retry. Posting twice is a worse failure than posting late.

### 8.5 Confirmation gate

1. `cr post` without `--confirm` MUST validate, print the full payload, report
   `"posted": false`, and exit 0 without any network call.
2. `--confirm` MUST be required for every network write, on every round.
3. There MUST be no configuration setting, environment variable, profile field, or
   alias that removes the gate or supplies `--confirm` implicitly.
4. `cr` MUST NOT claim that a human read the draft. It can establish only that
   `--confirm` was given, and the round summary MUST record exactly that much, plus
   the payload hash and whether `draft.md` changed between render and confirmation
   per §7.1.5. Any output describing the gate MUST use that wording and MUST NOT
   assert that human review occurred.

## 9. Re-review

### 9.1 Finding states

| State | Meaning |
|-------|---------|
| `draft` | Recorded, not yet queued for a draft |
| `queued` | Rendered into the current draft |
| `duplicate` | Suppressed as a duplicate of another record (§6.4.3) |
| `suppressed` | Suppressed by an existing human thread (§3.5.4) |
| `discarded` | Deleted during triage, waiver written, `disposition` set |
| `posted` | Sent to GitHub, thread created |
| `awaiting-author` | Posted, no author response yet |
| `answered` | Author replied without changing code |
| `verifying` | Author pushed code touching the anchor |
| `fixed` | Verified as addressed, thread resolvable |
| `accepted` | Reviewer waived it after discussion |
| `regressed` | The fix introduced a new problem, linked to a new record |
| `stale-anchor` | The anchor no longer resolves after a force-push |

Every state MUST be produced by exactly one command:

| To state | Produced by |
|----------|-------------|
| `draft` | `cr record` |
| `queued` | `cr draft` |
| `duplicate`, `suppressed` | `cr merge`, `cr record` |
| `discarded` | `cr draft` or `cr post` reading a deleted block |
| `posted`, `awaiting-author` | `cr post --confirm` |
| `answered` | `cr answer` |
| `verifying`, `stale-anchor` | `cr recheck` |
| `fixed`, `regressed` | `cr verify` |
| `accepted` | `cr accept` |

1. Transitions MUST be recorded in `transitions.ndjson` with a timestamp, the head
   SHA, and the actor.
2. Terminal states are `fixed`, `accepted`, `discarded`, `duplicate`, and
   `suppressed`. Every other state is open per §1.1.

### 9.2 Anchors and migration

An anchor MUST carry `path`, `side`, `start_line`, `line`, a normalised content
hash of the anchored lines per §1.4, and up to three lines of context on each
side. Valid `side` values are `RIGHT`, `LEFT`, and `NONE`.

1. `RIGHT` anchors a line in the head. `LEFT` anchors a removed line and MUST be
   used only for records about deletions. `NONE` carries no location and posts as a
   top-level comment per §4.1.3.
2. On a head change, every anchor of an open record MUST be re-resolved by
   searching the new file for the content hash, then for the context window.
   `NONE` anchors are exempt.
3. A re-resolved anchor MUST update its line numbers and record the migration.
4. An anchor that cannot be re-resolved MUST move to `stale-anchor` and be surfaced
   for a decision; `cr` MUST NOT silently drop it.
5. Every entry in `citations` MUST be re-resolved by the same procedure. A record
   whose citation no longer resolves MUST be re-graded per §6.2, which may demote
   it to `argued` and therefore to a question. Evidence that moved out from under a
   finding is no longer evidence.

### 9.3 Recheck

`cr recheck <pr>` MUST, for the new head:

1. Migrate anchors and citations per §9.2.
2. Classify each open thread as untouched, touched, or stale.
3. Move touched threads to `verifying` and emit verification prompts.
4. Compute the delta diff since the last reviewed head. When the previous head is
   not an ancestor of the new head — the force-push case — `cr` MUST compute the
   delta against the merge base of the two heads and MUST report that it did so,
   because the delta is then wider than a push would suggest.
5. Open a new round for the delta, reusing waivers and the context store.
6. Report which prior claims changed, per §3.3.3, if the issue text drifted.

### 9.4 Verification outcomes

For each `verifying` record the agent MUST record exactly one outcome through
`cr verify <pr> <id> --outcome <outcome> --evidence <text>`:

1. `fixed` — the concern is addressed; `cr` MUST offer to resolve the thread. The
   record MUST carry a citation resolving against the new head, or a same-head
   probe; without one the outcome MUST be recorded as `partial` instead. Closing a
   concern is an assertion too, and is graded like one.
2. `partial` — a reply is queued in the next draft and the state returns to
   `awaiting-author`.
3. `regressed` — a new record is created with `regression_of` naming the original,
   and the original moves to `regressed`.

### 9.5 Thread resolution

1. `cr resolve <pr> <id> --confirm` MUST resolve the GitHub thread and set the
   record to `fixed`.
2. Resolution MUST require evidence text, stored on the record.
3. Resolving MUST be subject to the same confirmation gate as posting.

## 10. Reporting and convergence

### 10.1 Coverage report

`cr status <pr>` MUST report:

1. Units total, units with a complete row of cells, units with gaps, and units
   flagged `oversized` per §3.4.5.
2. Claims total, claims mapped to units, and claims with no implementation.
3. Active axes, disabled axes with reasons, and unavailable axes with reasons.
4. Findings and questions by state, severity, and grade.
5. Probes run, and probes that changed a finding's grade.
6. Waivers applied and duplicates suppressed in the current round.

### 10.2 Convergence

A PR is converged when all of the following hold:

1. The current head equals the head of the last recorded round.
2. Every unit has a complete row of cells for every active role, and every one of
   those cells was filled against the current head.
3. Every claim is either mapped to a unit or has an open unimplemented-claim
   record.
4. No open record per §1.1 carries severity `critical` or `high`. This includes
   records in `regressed` and `stale-anchor`.

`cr status` MUST print the convergence verdict and, when it is false, the exact
reason. A verdict of converged MUST be printed together with the disabled and
unavailable axes of §4.5.4, because convergence across three axes is not
convergence across four. `cr` MUST NOT approve the PR; the verdict is advice to
the reviewer.

### 10.3 Round summary

Every round MUST record to `rounds/<n>/summary.json` counts for raised,
deduplicated, suppressed by thread, waived, forced to question, drafted, posted,
discarded as `not-here`, and discarded as `wrong`, plus the top-level comment
count of §1.6.3, the probe cap state, the payload hash, and the draft-changed
signal of §8.5.4 — so the history of a review is reconstructable from state alone.

## 11. Command surface

| Command | Purpose |
|---------|---------|
| `cr init` | Create `~/.cr`, write default profiles and roles |
| `cr init --eject-roles` | Write built-in roles as editable files |
| `cr brief <pr>` | Read-only orientation payload for a PR |
| `cr review <pr>` | Emit per-role, per-unit prompts and output paths |
| `cr claims record <pr> <file>` | Store extracted claims |
| `cr merge <files...> -o <out> --repo <owner/repo>` | Merge and deduplicate role findings |
| `cr record <pr> <file>` | Record a round's merged findings |
| `cr sandbox create\|destroy <pr>` | Manage the probe worktree |
| `cr test <pr> [--filter]` | Run the test suite in the sandbox |
| `cr probe run <pr> --kind <kind> ...` | Execute and record a probe |
| `cr draft <pr>` | Render the editable draft |
| `cr post <pr> [--confirm] [--reconcile]` | Validate and post the review |
| `cr recheck <pr>` | Re-review after a head change |
| `cr verify <pr> <id> --outcome <o> --evidence <text>` | Record a verification outcome |
| `cr accept <pr> <id> --evidence <text>` | Waive a posted record after discussion |
| `cr resolve <pr> <id> --confirm` | Resolve a thread with evidence |
| `cr answer <pr> <question-id> <text>` | Close a question and store a note |
| `cr note <ISSUE-KEY> <text>` | Store an out-of-band note |
| `cr note --remove <note-id>` | Retract a note |
| `cr context <ISSUE-KEY>` | Print accumulated notes |
| `cr waivers list\|remove --repo <owner/repo>` | Inspect and edit waivers |
| `cr stats --repo <owner/repo>` | Triage statistics, demotion and volume candidates |
| `cr rules list [--dead] --repo <owner/repo>` | Effective rules and the layer each came from |
| `cr rules check <pr>` | Run mechanical rule detection over the diff |
| `cr rules suggest --repo <owner/repo>` | Propose rules from recurring comment history |
| `cr status <pr>` | Coverage, states, and convergence |
| `cr config [--resolved]` | Effective configuration |

Record ids are scoped to a PR, so every command naming one takes the PR.

### 11.1 Global flags

| Flag | Effect |
|------|--------|
| `--json` | Force JSON output |
| `--compact` | Minimal JSON |
| `--quiet` | Suppress informational messages |
| `--no-color` | Disable colour |
| `--repo <owner/repo>` | Override repository detection |

`--quiet` MUST NOT suppress any honesty disclosure. The disabled and unavailable
axes of §4.5.4, the probe cap of §5.6.4, the forcing counts of §6.3.2, the waiver
and duplicate counts of §10.1.6, the sandbox recreation notice of §5.1.6, the
force-push recompute of §9.3.4, and the comment cap of §1.6.2 MUST always be
printed.

### 11.2 Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Validation failure |
| 2 | Usage error |
| 3 | File, configuration, or external command failure |
| 4 | State conflict, including lock timeout and partial post |

Validation runs before the confirmation gate. A `cr post` whose payload fails
validation MUST exit 1 whether or not `--confirm` was given; only a payload that
passes validation reaches §8.5.1's exit 0.

## 12. Output contract

1. Output MUST be JSON when stdout is not a terminal or `--json` is given, and
   human-readable text otherwise.
2. JSON MUST be pretty-printed with two-space indentation.
3. Slice fields MUST serialise as `[]` and never as `null`.
4. Every error MUST carry a `hint` field naming the next actionable step.
5. `--compact` MUST omit `output_tail`, `input`, and ingested thread bodies. It
   MUST NOT omit `evidence` or `citations`: the agent composes every posted body
   from them, and a compacted payload would leave the composition unfounded.
6. Every command that could have performed a network write MUST report a `posted`
   boolean, so a dry run is distinguishable from a successful post in JSON as well
   as in a terminal.

## 13. Skill

`skills/cr/SKILL.md` MUST ship with the release and MUST document:

1. The full loop: brief, review fan-out, merge, record, probe, draft, human read,
   post, recheck.
2. The argued rule, that it cannot be overridden, and when to escalate to a probe
   instead of asserting.
3. The draft triage verbs, the marker edit semantics of §7.2, and the difference
   between discarding as `wrong` and as `not-here`.
4. That every network write needs `--confirm`, that `cr` cannot prove the human
   read the draft, and that the human is therefore the one who must.
5. The role, profile, and rule file formats, and how to add each.
6. That a comment written by hand more than twice belongs in the rule corpus, with
   a `detect` block when the violation is mechanically matchable.
7. That `citations` is the machine-readable evidence field and `evidence` is prose,
   so a finding without citations is graded `argued` and posted as a question.
8. The comment economy of §1.6: fewer comments, each anchored to the line it
   concerns.

## 14. Distribution

1. `cr` MUST build as a single binary whose only runtime dependencies are `git`,
   `gh`, and the configured tracker command.
2. Releases MUST be produced by GoReleaser on a `v*` tag, publishing archives for
   darwin and linux on both amd64 and arm64.
3. A release MUST update the Homebrew formula in the `deligoez/homebrew-tap`
   repository so that `brew install deligoez/tap/cr` installs the binary as `cr`.
4. `go install github.com/deligoez/cr/cmd/cr@<tag>` MUST install the same version as
   the tag.
5. The skill MUST ship inside this repository at `skills/cr/SKILL.md`, and
   `.claude-plugin/marketplace.json` MUST expose it as an installable plugin.
6. `cr --version` MUST print the tagged version, injected at build time.
