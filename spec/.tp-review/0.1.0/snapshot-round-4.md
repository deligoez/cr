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
3. Strip trailing whitespace from every line, where whitespace means U+0020
   SPACE and U+0009 TAB and nothing else.
4. Replace every run of one or more U+0020 or U+0009 anywhere in a line —
   including leading indentation — with a single U+0020.
5. Drop leading and trailing blank lines, and collapse interior runs of blank
   lines to one.
6. Rejoin the lines with a single LF between them. The result MUST NOT end with a
   trailing LF, so a one-line input normalises to that line with no terminator.

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

1. Every posted comment MUST be anchored to a line of the diff. v0.1 has no
   unanchored comment channel: an item with no code location is reported through
   `cr status` and never posted, per §4.1.3.
2. `post.max_comments` (default 20) MUST cap the comments in one round. When the
   cap is exceeded `cr` MUST block posting with exit code 1 and name the count,
   so the user triages further. `cr` MUST NOT silently drop comments to fit.
3. Triaging to fit under the cap MUST NOT be destructive. A discard dispositioned
   `not-here` is scoped to the pull request per §7.4.1, so setting a true finding
   aside for volume reasons never silences it repository-wide.

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
| `waivers.ndjson` | Pull-request-scoped `not-here` waivers (§7.4) |
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
3. A hit MUST be recorded as a `citations` entry per §6.1 carrying the matched
   `path`, `line`, and `origin: rule`, which grades the record `cited` per §6.2.
   The rule id is carried separately on the record per §2.6.3.
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

The confirmation gate of §8.5, the argued forcing of §6.3, and the question label
of §8.1.4 are not settings. They MUST NOT be readable from any layer, and `cr`
MUST reject and report any `CR_`-prefixed variable or config key whose name would
address any of them.

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
| `span` | yes | The verbatim substring of the source text the claim was drawn from |
| `note_id` | no | The note the claim came from; required when `source` is `note` |
| `span_hash` | computed | Normalised hash of `span`, written by `cr` |
| `issue_hash` | computed | Normalised hash of the whole issue text at extraction, written by `cr` |
| `head` | yes | Head SHA the extraction ran against |

1. `cr claims record <pr> <file.ndjson>` MUST validate and store claims. `cr` MUST
   compute `span_hash` and `issue_hash` itself. For a claim whose `source` is not
   `note`, `cr` MUST reject with exit code 1 any claim whose `span` does not occur
   in the issue text.
2. A claim sourced from the context store (§3.6) MUST carry `source: note` and a
   `note_id`, MUST set `span` to that note's body, and MUST be validated against
   the named note rather than the issue text — the two rules never collide because
   each claim is checked against exactly one source. Such a claim MUST be included
   in coverage exactly like a tracker claim, but it rests on weaker provenance, and
   §8.1.6 requires that provenance to reach the reader.
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
6. Every unit MUST record an `id` of the form `u<n>`, stable for the life of the
   PR and reused across rounds for a unit whose file paths and hunk ranges are
   unchanged; its file paths; its hunk ranges; its changed line count; a
   normalised hash of its changed lines called the **unit hash**; and whether it
   was formed by symbol, by adjacency, or by fallback. The `id` names the unit and
   the unit hash says whether its content moved — §10.2.2 needs both.
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

### 3.7 Orientation

`cr brief <pr>` is read-only with respect to judgement: it performs no network
write and creates no finding, coverage cell, probe, or thread. It MAY compute and
persist the derived inputs of §3.3 through §3.6 — claims already recorded, units,
threads, notes — because those are facts about the PR rather than opinions about
it, and §4.6 needs the same unit set. It is safe to run at any point in a round.
It MUST assemble and print, without judgement:

1. PR identity, head SHA, merge base, and the resolved profile together with the
   layer that selected it per §2.4.
2. The issue key and the source it was resolved from per §3.2, with the issue
   text, or the reason no key was found.
3. The claims of §3.3, and whether the issue text has drifted per §3.3.3.
4. The units of §3.4 with their file paths, hunk ranges, and unit hashes.
5. The ingested threads of §3.5 and the notes of §3.6 for the issue key.
6. The active, disabled, and unavailable axes with their reasons per §4.5.

## 4. Review axes

### 4.1 Intent coverage

1. Every unit MUST be mapped to zero or more claims.
2. A unit mapped to zero claims MUST raise an unmapped-unit item.
3. A claim mapped to zero units MUST raise an unimplemented-claim entry. Such an
   entry has no code location by construction, so it MUST be recorded in
   `coverage.ndjson` against the intent axis, MUST be reported by `cr status` per
   §10.1.2, and MUST block convergence per §10.2.3. It MUST NOT become a record in
   `findings.ndjson`, and MUST NOT be drafted or posted: v0.1 has no unanchored
   comment channel per §1.6.1. The reviewer raises it with the author out of band
   and may record the answer as a note per §3.6.
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
2. Candidate ranking MUST work on the **comparison name**: the symbol name
   case-folded to lowercase with every `_` and `-` removed. Similarity MUST be
   `1 - levenshtein(a, b) / max(len(a), len(b))` over comparison names, counted in
   Unicode code points, defined as 1.0 when both names are empty. A candidate
   qualifies only when its similarity is at least `reinvention.min_similarity`
   (default 0.6) **and** its declared parameter count equals that of the added
   symbol. The declared parameter count of a function or method is the number of
   parameters in its signature; for a class it is the parameter count of its
   constructor, or zero when it declares none. Qualifying candidates MUST be
   ordered by similarity descending, then path ascending, then line ascending, and
   `cr` MUST attach at most `reinvention.max_candidates` (default 5). Semantic
   equivalence is the agent's judgement, not `cr`'s.
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
5. Every cell MUST carry the unit id and role id it sits at, one of `pass`,
   `finding`, `question`, or `na`, the head it was filled against, the unit hash of
   §3.4.6 it was filled for, and a reason when it is `na`.
6. Cells are the agent's judgement and reach `cr` the same way findings do, through
   `cr cells record <pr> <file.ndjson>`, which validates and stores them. `cr` MUST
   reject with exit code 1 a cell naming an unknown unit id or an inactive role,
   and MUST NOT invent a cell for a unit no role reported on — an unfilled cell is
   a coverage gap per §10.1.1, not something for `cr` to complete.

### 4.6 Review fan-out

`cr review <pr>` emits the prompts an agent runs. It performs no judgement of its
own and calls no model.

1. For every active role of §2.5 and every unit of §3.4, `cr` MUST emit one prompt
   carrying the role's `instructions` and `focus`, the unit's hunks, the claims
   mapped to it, the candidate symbols of §4.3.1, the rule hits of §4.3.6, the test
   files of §4.4.1, and the threads and notes of §3.5.3 and §4.1.5.
2. Every prompt MUST name the NDJSON path the role writes to, and MUST state the
   record schema of §6.1 together with the fields the agent may not write per
   §6.1.4.
3. `cr review` MUST NOT write to `findings.ndjson`. It MUST report the set of cells
   it expects to be filled, so §10.2.2 can be checked once the roles return.
4. A role whose prerequisites are unmet MUST be reported as skipped with its
   reason, never omitted silently.

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
   the PR head and that the sandbox is clean. **Clean** means that no tracked file
   differs from HEAD, and that no leftover probe artefact exists — no file under
   `probe.test_dir` whose name matches the probe naming scheme of §5.4.2. Other
   untracked files are ignored, because `sandbox.copy` and `sandbox.setup` create
   them by design. A sandbox failing any of these checks MUST be recreated, and the
   recreation MUST be reported.
7. A probe whose post-run cleanliness check fails MUST be recorded with result
   `error`, MUST NOT grade any finding, and MUST force recreation before the next
   run.

### 5.2 Test runs

1. `cr test <pr> [--filter <expr>]` MUST run the profile's test command inside the
   sandbox and record exit code, duration, the executed and failed test counts
   when `tests.count_pattern` is configured and matches, and the last
   `tests.output_tail_bytes` of output. `tests.count_pattern` MUST yield two
   capture groups, the executed count and the failed count; a pattern that does not
   match a run leaves both counts undetermined.
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
4. The result MUST be determined by the first matching rung of this ladder, which
   is total over every run — no run can match two rungs, and none can match none:
   1. the run was killed for exceeding `tests.timeout_seconds` → `timeout`;
   2. the runner could not be started, or exited on a signal → `error`;
   3. the executed test count is known and equals zero → `no-tests-selected`;
   4. the executed or failed count is undetermined, whether because no
      `tests.count_pattern` is configured or because the configured pattern did
      not match the output → `inconclusive`;
   5. the failed count is zero → `no-test-failed`;
   6. otherwise → `failed`.
5. Only `no-test-failed` proves the gap, and only when the filtered baseline of
   §5.2.2 passed. `timeout`, `error`, `no-tests-selected`, and `inconclusive` MUST
   NOT support a `probed` grade, so neither an over-narrow filter, nor a runner
   whose output `cr` cannot parse, nor a run that never finished can manufacture
   evidence.
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
3. The result MUST be determined by the first matching rung of this ladder, which
   is total over every run: the run was killed for exceeding
   `tests.timeout_seconds` → `timeout`; the runner could not be started, or the
   supplied test failed to compile or load → `error`; the executed test count is
   known and equals zero, so the filter excluded the supplied test →
   `no-tests-selected`; the failed count is zero → `passed`; otherwise → `failed`.
4. A `failed` gap probe means either the behaviour is wrong or the supplied test is
   wrong, and `cr` cannot distinguish the two. The result MUST be recorded and the
   probe output offered as a reproduction. The probe MUST be treated as supporting
   the finding only when the finding's `claim` field names the claim the test
   asserts; without it the probe MUST NOT support a `probed` grade, which leaves
   the record `argued` under §6.2 and therefore a question under §6.3. Severity
   MUST be at least `high` only when the probe supports the finding.
5. A `passed` gap probe means only the test is missing; severity MUST be at most
   `medium`. `timeout`, `error`, and `no-tests-selected` prove nothing and MUST NOT
   support a `probed` grade.

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
| `tests_failed` | no | Failed test count when derivable |
| `baseline` | required for a filtered `mutation` probe | Id of the filtered baseline it was compared against, without which §5.3.5 cannot be evaluated |
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
| `citations` | no | Array of `{path, line, content_hash, origin}`; `content_hash` and `origin` are computed by `cr`; the machine-readable input to grading |
| `probe` | no | Probe id, when graded `probed` |
| `suggestion` | no | Exact replacement lines |
| `suggestion_origin` | no | `agent` or `rule` |
| `state` | yes | State per §9.1 |
| `disposition` | no | `wrong` or `not-here` once discarded (§7.2) |
| `duplicate_of` | no | Representative record id when suppressed as a duplicate |
| `suppressed_by` | no | Thread id when suppressed per §3.5.4 |
| `regression_of` | no | Original record id when this records a regression |
| `reply_to` | no | Thread id this record replies to, set by `cr verify --outcome partial` (§9.4.2) |
| `thread_id` | no | GitHub thread id once posted |
| `round` | yes | The round that produced it |
| `head` | yes | Head SHA the record was produced against |

1. `summary` and `evidence` MUST be English. Reader-facing prose is produced at
   draft time per §8.1.
2. Every record MUST carry an anchor that resolves to a line in the current head.
   An item with no code location never becomes a record, per §4.1.3.
3. `cr merge` and `cr record` MUST reject a record missing any field marked
   required, with exit code 1, naming the line and the field.
4. The agent MUST NOT write `grade`, `span_hash`, `issue_hash`, a citation's
   `content_hash` or `origin`, or any other field marked computed. A record
   arriving with one MUST be rejected with exit code 1.

### 6.2 Evidence grades

| Grade | Requirement |
|-------|-------------|
| `probed` | References a probe whose head matches and whose result supports the claim per §5.3 and §5.4 |
| `cited` | Carries at least one entry in `citations` that `cr` resolved against the current head, and that entry either has `origin: rule` or lies outside the record's own unit |
| `argued` | Neither of the above |

1. The grade MUST be computed by `cr` from the record, never asserted by the agent.
   The only inputs are `citations` and the referenced probe record, whose `result`
   and supporting conditions are fixed by §5.3 and §5.4; `evidence` prose is never
   parsed.
2. A record claiming `probed` without a valid same-head probe that supports it MUST
   be rejected with exit code 1.
3. `cr` MUST resolve every entry in `citations` against the current head, and MUST
   reject with exit code 1 an entry whose path does not exist or whose line is out
   of range. `cr` computes and stores `content_hash` itself at record time, so that
   hash cannot fail then; it exists to detect drift at re-review per §9.2.5.
4. Record-time validation establishes that a citation **exists**, never that it
   **supports** the summary. `cr` MUST NOT describe a `cited` record as verified,
   confirmed, or proven in any output; `cited` means only that a human-checkable
   location was supplied.
5. `origin` MUST be computed, never supplied. `cr` MUST set `origin: rule` only on
   a citation it generated itself from its own detection pass of §2.6.1, matching
   the record's `rule` id against its own recorded hit in `rule-stats.ndjson`, and
   MUST set `origin: agent` on every citation the agent supplied. A record arriving
   with `origin: rule` MUST be rejected with exit code 1. Without this the
   in-unit exemption of the `cited` row would be self-granted, which is the same
   forgery the deleted `rule` branch allowed.
6. `cr` cannot judge whether a citation supports the summary, and MUST NOT claim
   to. Every citation of a `cited` record MUST be rendered verbatim into the draft
   block, so the human makes the judgement `cr` cannot.

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
4. Findings matching an active waiver MUST be dropped and counted per §7.4. A
   dropped finding MUST NOT be written to `findings.ndjson` at all, so no record
   ever exists in a state §9.1 does not define; only the count reaches the round
   summary.

### 6.5 Merge

1. `cr merge <files...> -o <out> --repo <owner/repo> --pr <number>` MUST merge
   per-role NDJSON files, apply §6.4, and report counts by role, axis, severity,
   and grade. The repository is required because §6.4.2 needs its resolved role
   corpus and §6.4.4 needs its repository-wide waivers; the pull request is
   required because §7.4.1 also scopes `not-here` waivers to it, and a merge that
   could not read them would resurface exactly what the reviewer set aside.
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
| Deletes the block entirely | Discarded with disposition `not-here`; a pull-request-scoped waiver is written per §7.4 |
| Sets `disposition=wrong` in the marker | Discarded as a false positive whether or not the body remains; a repository-wide waiver is written per §7.4 and it counts against the class per §7.3 |

Both discard verbs are observable in the file `cr` reads back: deletion removes
the marker, and `wrong` is a marker edit that survives precisely because the
marker stays. A record carrying `disposition=wrong` MUST be discarded even when
its body is untouched, so the reviewer never has to delete text to express it.

The two dispositions are deliberately distinct. `not-here` means the finding is
true but not worth a comment on this pull request — the ordinary case under §1.6
— and MUST NOT count against the class's precision. `wrong` means the finding is
false, and is the only signal that should demote a class. Collapsing them would
demote classes that are always right and merely never worth saying.

Marker fields have these edit semantics; any other edit MUST abort with exit
code 1, naming the record id:

| Marker field | Edit semantics |
|--------------|----------------|
| `id` | Immutable; a changed or unknown id aborts |
| `kind` | `finding`→`question` softens; `question`→`finding` is accepted only when the recomputed grade is `probed` or `cited`, and aborts otherwise per §6.3.3 |
| `path`, `line` | Re-validated against the current head; aborts when the anchor no longer resolves |
| `severity` | Freely editable; recorded as a triage event |
| `disposition` | Setting it to `wrong` discards the record as a false positive; `not-here` is written by `cr` when a block is deleted and MUST NOT be set by hand |
| `grade` | Informational. `cr` recomputes it per §6.2 and ignores whatever the marker holds; it aborts only when the marker's `kind` contradicts the recomputed grade per §6.3.3 |

1. `cr post` MUST refuse to run when a marker is malformed, naming the line.
2. `cr post` MUST recompute every record's grade and re-apply §6.3 before building
   the payload, so no draft edit can turn an `argued` record into a posted
   assertion.
3. v0.1 has no manual-comment channel in the draft. A block whose `id` is unknown
   MUST abort with exit code 1 rather than being adopted as a new record: every
   record in the corpus carries a role, an axis, and a grade `cr` computed, and a
   hand-written block can carry none of them. A comment the reviewer wants to make
   in their own name is written on GitHub directly, after posting.

### 7.3 Triage statistics

1. Triage events MUST be keyed by `(PR, round, record id, action)` and written
   idempotently: re-running a command MUST overwrite the event for that key rather
   than append a second one, so regenerating a draft per §7.1.6 or running
   `cr post` twice cannot inflate a count. `cr draft` MUST write one `raised` event
   per record it queues, and MUST write the outcome event directly for a record it
   discards at draft time. `cr post --confirm` MUST write exactly one outcome event
   per queued record, drawn from `kept`, `softened`, `discarded-not-here`, and
   `discarded-wrong`; `cr post` without `--confirm` MUST write none, since it
   changes nothing. Every event MUST carry class, axis, role, grade, rule id when
   present, the PR, the round, and the head. These five action names are the
   complete vocabulary the statistics below are computed from.
2. `cr stats --repo <owner/repo>` MUST report per class and per rule: raised, kept,
   softened, discarded as `not-here`, and discarded as `wrong`.
3. The demotion rate for a class MUST be computed as
   `(discarded-wrong + softened) / raised` over that class's events in the
   repository's `triage.ndjson`. Discards dispositioned `not-here` MUST be excluded
   from the numerator, because they carry no evidence that the class is imprecise.
   A class whose rate exceeds `stats.demote_threshold` (default 0.6) over at least
   `stats.min_samples` (default 8) raised events MUST be listed as a demotion
   candidate.
4. Marking a false positive `wrong` costs the reviewer an extra edit while plain
   deletion is free, so `discarded-wrong` is systematically under-counted and the
   demotion rate is a lower bound on imprecision. `cr` MUST report it as a lower
   bound and MUST NOT present it as a measured precision.
5. A class whose `not-here` rate exceeds `stats.demote_threshold` over the same
   sample MUST be reported separately as a **volume candidate**: it is accurate but
   rarely worth posting, and the remedy is to stop raising it, not to soften it.
6. Both candidacies are reports to the user, not automatic changes. `cr` MUST NOT
   alter a rule's `kind` by itself.

### 7.4 Waivers

1. A waiver MUST be keyed by `(anchor.path, class, rule id when present, normalised
   hash of the anchored lines)`. Its scope follows its disposition: a waiver
   written for `wrong` is scoped to the repository, and one written for `not-here`
   is scoped to the pull request. The summary is deliberately excluded from the
   key: it is agent-composed prose that differs between rounds, and including it
   would let a waived finding resurface under a reworded summary.
2. The key is narrow on purpose. It suppresses the same class at the same unchanged
   code in the same file, and stops suppressing once that code changes — which is
   exactly when the judgement behind the waiver should be revisited.
3. The two scopes follow the two meanings. "This is wrong" is a fact about the
   class and generalises across the repository; "not worth saying here" is a fact
   about this pull request and MUST NOT silence the finding anywhere else.
4. The two scopes live in two files. A repository-wide waiver is written to
   `~/.cr/waivers/<owner>/<repo>.ndjson`; a pull-request-scoped one is written to
   that PR's `waivers.ndjson` per §2.3. Both MUST be readable and removable, so
   neither disposition is a one-way door.
5. A waiver MUST record its `disposition` per §7.2, so §7.3 can tell a false
   positive from a deliberate silence.
6. A waiver for a record produced by a rule MAY be widened to a path prefix, so a
   rule can be exempted for a legacy area without disabling it repository-wide.
7. Waived findings MUST be dropped before drafting and counted in the round summary.
8. `cr waivers list --repo <owner/repo> [--pr <number>]` MUST print active waivers
   with their scope and disposition, covering both files of §7.4.4 when `--pr` is
   given and the repository-wide file alone when it is not.
   `cr waivers remove <id> --repo <owner/repo> [--pr <number>]` MUST delete one
   from either scope.
9. A waiver MUST record the round, the PR, the head, and the reason when one was
   given.

## 8. Posting

### 8.1 Render contract

1. Comment bodies MUST be written in the language configured by `render.lang`,
   defaulting to `tr`.
2. `cr` MUST render each block's initial body from the record's `summary` and
   `evidence`, in English. The agent then rewrites that body in `render.lang` by
   editing `draft.md` before the human reads it. Editing the draft is the only
   input path for reader-facing prose: `cr` MUST NOT translate or compose, and
   MUST NOT accept a body through any other channel.
3. A body MUST be non-empty and MUST NOT contain the marker comment.
4. `cr` MUST prepend a fixed, `cr`-owned label line to every `kind=question`
   comment, naming the register and the grade. The label text MUST be built in per
   `render.lang` and MUST NOT be configurable: §6.3's forcing reaches the reader
   through this line alone, so §2.7 protects it exactly as it protects the gate.
5. `cr` MUST refuse to post a `kind=question` body containing no `?` character,
   with exit code 1 naming the record id. The register of a question is not
   cosmetic: without it, §6.3's forcing changes only a field the reader never sees.
6. Weak provenance MUST be disclosed in the posted body, not only in the draft the
   author never sees. A suggestion carrying `suggestion_origin: rule` MUST be
   labelled machine generated, and a record resting on a claim with `source: note`
   MUST name the note and its source per §3.6.3. The author cannot weigh what they
   cannot see.
7. Every citation of a `cited` record MUST be rendered into the posted body as
   `path:line`, exactly as §6.2.6 renders it into the draft. A `cited` record is an
   assertion whose whole warrant is a location the reader can check; posting the
   assertion while withholding the location asks the author to take it on trust,
   which is the cost this spec exists to avoid.

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
3. The exact posted payload MUST be written to `rounds/<n>/posted.json` before the
   network call, and updated with returned thread ids after it. The **payload
   hash** MUST be the normalised hash of the comments array alone, excluding the
   review body, so that embedding it in that body per §8.4.3 cannot change the
   value being embedded.
4. A reply record per §9.4.2 MUST be posted into its existing thread rather than as
   a new review comment, in the same `cr post --confirm` invocation and after the
   review call. Replies MUST be listed in `posted.json` and counted in the round
   summary. §8.3.1's single-notification guarantee governs new comments; a reply
   belongs to the thread it answers and cannot be batched into a fresh review.

### 8.4 Partial failure

1. The GitHub review-creation call is atomic: it either creates the review with all
   its comments or creates nothing. `cr` MUST therefore pre-validate every comment
   position per §8.2 before the call.
2. If the call is rejected, no state MUST be marked posted. `cr` MUST report every
   position GitHub named invalid, with the record id it belongs to, and exit 4.
3. To make a round identifiable after the fact, `cr` MUST embed the payload hash as
   an HTML comment in the review's own body before posting. The review body is not
   a line comment and is unaffected by §1.6.1.
4. If the outcome is unknown — a timeout, a dropped connection, or a response `cr`
   cannot parse — `cr` MUST mark the round `post-unresolved` and MUST NOT retry
   automatically. `cr post <pr> --reconcile` MUST list the PR's reviews, match the
   embedded payload hash of §8.4.3, and either adopt that review as posted or clear
   the state for a retry. Posting twice is a worse failure than posting late.

### 8.5 Confirmation gate

1. `cr post` without `--confirm` MUST validate, print the full payload, report
   `"posted": false`, and exit 0 without any network call.
2. `--confirm` MUST be required for every network write, on every round.
3. There MUST be no configuration setting, environment variable, profile field, or
   alias that removes the gate or supplies `--confirm` implicitly.
4. `cr` MUST NOT claim that a human read the draft. The only fact it can establish
   is that `--confirm` was given, and the round summary MUST record exactly that
   and the payload hash, and nothing more. In particular `cr` MUST NOT report
   whether `draft.md` changed as evidence of human involvement: §8.1.2 requires the
   agent to rewrite every body before the human sees it, so the file has always
   changed and the signal measures nothing. Any output describing the gate MUST say
   that confirmation was given, never that the review was read.

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

Transitions are exhaustive. A transition not listed here MUST be rejected with
exit code 4, naming the record and its current state:

| From | To | Command |
|------|----|---------|
| — (new record) | `draft` | `cr record` |
| — (new linked record) | `draft` | `cr verify --outcome regressed` (§9.4.3) |
| — (new reply record) | `draft` | `cr verify --outcome partial` (§9.4.2), `cr accept` (§9.6.5) |
| `draft` | `duplicate`, `suppressed` | `cr record` |
| `draft` | `queued` | `cr draft` |
| `queued` | `discarded` | `cr draft`, `cr post --confirm` |
| `queued` | `posted`, then `awaiting-author` | `cr post --confirm` |
| `awaiting-author` | `answered` | `cr answer` |
| `awaiting-author`, `answered`, `regressed` | `verifying` | `cr recheck` |
| `draft`, `queued`, `awaiting-author`, `answered`, `verifying`, `regressed` | `stale-anchor` | `cr recheck` |
| `verifying` | `fixed`, `regressed` | `cr verify` |
| `verifying` | `awaiting-author` | `cr verify --outcome partial` |
| `awaiting-author`, `answered`, `regressed`, `stale-anchor` | `accepted` | `cr accept` |

`cr merge` is not a producer: it runs on role output files before any record
exists, and §6.4.3 marks duplicates as `cr record` writes them.

1. Transitions MUST be recorded in `transitions.ndjson` with a timestamp, the head
   SHA, and the actor.
2. Terminal states are `fixed`, `accepted`, `discarded`, `duplicate`, and
   `suppressed`. Every other state is open per §1.1.

### 9.2 Anchors and migration

An anchor MUST carry `path`, `side`, `start_line`, `line`, a normalised content
hash, and up to three lines of context on each side. The content hash is the
normalised hash per §1.4 of the lines from `start_line` to `line` inclusive,
treated as one text — so a single-line anchor and a multi-line anchor hash by the
same rule. Valid `side` values are `RIGHT` and `LEFT`.

1. `RIGHT` anchors a line in the head. `LEFT` anchors a removed line and MUST be
   used only for records about deletions. Every record in `findings.ndjson` MUST
   have an anchor; an item with no code location never becomes a record, per
   §4.1.3.
2. On a head change, every anchor of an open record MUST be re-resolved
   deterministically. Let `n` be `line - start_line + 1`. `cr` MUST scan the new
   file from line 1 for a window of `n` consecutive lines whose joined normalised
   hash equals the stored content hash; exactly one match resolves the anchor. On
   zero matches or more than one, `cr` MUST scan again for the stored context
   window, where again exactly one match resolves it.
3. A resolved anchor MUST take `start_line` from the first line of the matching
   window and `line` from its last — in the context-window case, from the lines
   between the leading and trailing context — MUST keep its content hash, and MUST
   record the migration.
4. An anchor matching zero times, or more than once at both stages, MUST move to
   `stale-anchor` and be surfaced for a decision. `cr` MUST NOT choose between
   candidates and MUST NOT silently drop it.
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

1. `fixed` — the concern is addressed; `cr` MUST offer to resolve the thread.
   Closing a concern is an assertion, and it MUST meet the same bar §6.2 sets for
   `cited`: `cr verify` MUST accept `--citation <path>:<line>`, repeatable, and
   `--probe <id>`, and the outcome may be `fixed` only when at least one citation
   resolves against the new head and lies outside the record's own unit, or a
   same-head probe supports it. `cr` MUST store the citations on the record with
   `origin: agent` and their computed hashes. Where the bar is not met the outcome
   MUST be recorded as `partial` instead.
2. `partial` — `cr` MUST create a reply record carrying `reply_to` with the
   original's `thread_id`, the original's `class`, `axis`, `unit`, and `anchor`,
   and `kind: question`. It is rendered into the next draft as an ordinary block
   per §7.1 and posted as a reply inside that thread rather than as a new comment,
   which is why §1.6.1 is not engaged. The original returns to `awaiting-author`.
3. `regressed` — a new record is created with `regression_of` naming the original,
   and the original moves to `regressed`.

### 9.5 Thread resolution

1. `cr resolve <pr> <id> --confirm` MUST resolve the GitHub thread of a record that
   is already in state `fixed`. A record in any other state MUST be rejected with
   exit code 4, so `cr verify` and the evidence bar of §9.4.1 remain the only path
   to `fixed`. Resolution changes no state; it closes the thread.
2. The evidence recorded by `cr verify` MUST be carried into the resolution
   comment, so the author reads why the thread was closed. `cr resolve` MUST NOT
   post agent-composed prose: it MUST post a built-in, `cr`-owned template for
   `render.lang`, naming the record id and substituting the stored evidence and
   citations. The template is protected exactly as the label of §8.1.4 is, and
   §8.1.2 is unaffected because `cr` substitutes rather than composes.
3. The resolution comment MUST be written to the round's `posted.json` and counted
   in the round summary, so no network write escapes the record.
4. Resolving MUST be subject to the same confirmation gate as posting.

### 9.6 Acceptance

`cr accept <pr> <id> --evidence <text>` records that the reviewer withdrew a
posted concern after discussion, rather than because it was addressed. The
distinction matters: `fixed` claims the code changed, `accepted` claims only that
the reviewer stopped asking.

1. It MUST apply only to a record in `awaiting-author`, `answered`, or
   `stale-anchor`, and MUST reject any other state with exit code 4.
2. It MUST require evidence text naming why the concern was withdrawn, store it on
   the record, and move the record to `accepted`.
3. `accepted` is terminal and MUST NOT write a waiver. The concern was real, and
   suppressing it on a future pull request is a separate decision the reviewer
   makes through §7.2.
4. Accepting MUST NOT resolve the GitHub thread; §9.5 stays the only resolution
   path, and it applies only to `fixed`.
5. Accepting MUST queue a withdrawal reply into the next draft, as a reply record
   per §9.4.2 carrying `reply_to` with the record's `thread_id`. A concern the
   reviewer has silently dropped is worse than one never raised: the author is
   still working against it, and the thread still reads as an open objection.

## 10. Reporting and convergence

### 10.1 Coverage report

`cr status <pr>` MUST report:

1. Units total, units with a complete row of cells, units with gaps, and units
   flagged `oversized` per §3.4.5.
2. Claims total, claims mapped to units, and the unimplemented-claim entries of
   §4.1.3, which appear here and nowhere else because they are never posted.
3. Active axes, disabled axes with reasons, and unavailable axes with reasons.
4. Findings and questions by state, severity, and grade.
5. Probes run, and probes that changed a finding's grade.
6. Waivers applied and duplicates suppressed in the current round.

### 10.2 Convergence

A PR is converged when all of the following hold:

1. The current head equals the head of the last recorded round.
2. Every unit in the current unit set has a complete row of cells for every active
   role, and every one of those cells was filled for that unit's current unit hash
   per §3.4.6. A unit unchanged since an earlier round keeps its cells, so a delta
   round per §9.3.5 does not invalidate coverage it did not touch; a unit whose
   content changed needs its row filled again.
3. Every claim is mapped to at least one unit. A claim carrying an
   unimplemented-claim entry per §4.1.3 MUST block convergence until it is either
   mapped, or set aside by a note per §3.6 naming it out of scope for this pull
   request. The entry is not a record, so §10.2.4 never reaches it; this condition
   is the only thing that makes an unimplemented claim block, which is what §4.1.3
   relies on.
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
discarded as `not-here`, and discarded as `wrong`, plus the comment count against
`post.max_comments` per §1.6.2, the replies posted per §8.3.4, the probe cap
state, and the payload hash of §8.3.3 — so the history of a review is
reconstructable from state alone.

## 11. Command surface

| Command | Purpose |
|---------|---------|
| `cr init` | Create `~/.cr`, write default profiles and roles |
| `cr init --eject-roles` | Write built-in roles as editable files |
| `cr brief <pr>` | Read-only orientation payload for a PR |
| `cr review <pr>` | Emit per-role, per-unit prompts and output paths |
| `cr claims record <pr> <file>` | Store extracted claims |
| `cr merge <files...> -o <out> --repo <owner/repo> --pr <n>` | Merge and deduplicate role findings |
| `cr record <pr> <file>` | Record a round's merged findings |
| `cr cells record <pr> <file>` | Store the coverage cells the roles filled (§4.5.6) |
| `cr sandbox create\|destroy <pr>` | Manage the probe worktree |
| `cr test <pr> [--filter]` | Run the test suite in the sandbox |
| `cr probe run <pr> --kind <kind> ...` | Execute and record a probe |
| `cr draft <pr>` | Render the editable draft |
| `cr post <pr> [--confirm] [--reconcile]` | Validate and post the review |
| `cr recheck <pr>` | Re-review after a head change |
| `cr verify <pr> <id> --outcome <o> --evidence <text> [--citation <path>:<line>] [--probe <id>]` | Record a verification outcome |
| `cr accept <pr> <id> --evidence <text>` | Waive a posted record after discussion |
| `cr resolve <pr> <id> --confirm` | Close the thread of a record already `fixed` |
| `cr answer <pr> <question-id> <text>` | Close a question and store a note |
| `cr note <ISSUE-KEY> <text>` | Store an out-of-band note |
| `cr note --remove <note-id>` | Retract a note |
| `cr context <ISSUE-KEY>` | Print accumulated notes |
| `cr waivers list\|remove --repo <owner/repo> [--pr <n>]` | Inspect and edit waivers in either scope |
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
   between discarding as `wrong` — a marker edit, repository-wide, counted against
   the class — and as `not-here`, which is plain deletion, scoped to the pull
   request, and counted separately.
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
