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
| Probe | A reproducible experiment run in a sandbox, with a recorded result |
| Grade | The strength of a finding's evidence: probed, cited, or argued |
| Draft | A human-editable rendering of a round's queued output |
| Thread | A posted GitHub review comment and its replies |
| Round | One review pass, bound to a single head SHA |
| Waiver | A recorded decision to never raise a given finding again |
| Note | An out-of-band fact recorded against an issue key |

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

## 2. Architecture

### 2.1 Division of labour

`cr` is deterministic. It fetches, clusters, executes, validates, records, and
reports. It never calls a language model and never decides whether code is
correct.

The agent judges. It reads code, writes findings, composes prose, and decides
dispositions. It reaches `cr` through the commands in §11 and the skill in §13.

1. Every `cr` command MUST be reproducible given the same state directory,
   the same head SHA, and the same inputs.
2. `cr` MUST NOT perform any network write except the GitHub calls in §8,
   and those only behind the confirmation gate of §8.5.

### 2.2 State layout

All state lives under `~/.cr/`. `cr` MUST NOT write inside the repository under
review.

| Path | Contents |
|------|----------|
| `~/.cr/config.json` | Global defaults |
| `~/.cr/profiles/<id>.json` | Mechanical profiles (§2.4) |
| `~/.cr/roles/<id>.json` | Judgement roles (§2.5) |
| `~/.cr/repos/<owner>/<repo>/config.json` | Per-repository overrides |
| `~/.cr/repos/<owner>/<repo>/roles/<id>.json` | Per-repository role overrides |
| `~/.cr/state/<owner>/<repo>/pr-<n>/` | Per-PR state (§2.3) |
| `~/.cr/context/<ISSUE-KEY>.md` | Out-of-band context store (§3.6) |
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
| `rounds/<n>/draft.md` | The editable draft for round n (§7.1) |
| `rounds/<n>/posted.json` | The exact payload posted for round n |

1. All writes to per-PR state MUST take an exclusive advisory file lock.
2. Reads MUST be lock-free.
3. Every record MUST carry the head SHA it was produced against.

### 2.4 Profiles

A profile is mechanical, language-specific configuration. It is data, never
prompt text.

| Field | Type | Meaning |
|-------|------|---------|
| `id` | string | Profile id, equal to the file stem |
| `match.files` | array | Marker files that select this profile |
| `match.globs` | array | Source globs this profile owns |
| `axes` | object | Default enabled state per axis id |
| `sandbox.copy` | array | Paths copied from the main worktree into the sandbox |
| `sandbox.setup` | array | Commands run once after sandbox creation |
| `tests.cmd` | array | Test runner argv |
| `tests.filter_flag` | string | Flag used to narrow the run to a subset |
| `tests.timeout_seconds` | integer | Per-run timeout |
| `symbols.lang` | string | Language hint for reinvention search (§4.3) |

1. Profile selection MUST be automatic via `match.files`, and overridable by
   `profile` in the per-repository config.
2. If no profile matches, `cr` MUST report the situation and disable every axis
   that requires one, rather than guessing.
3. v0.1 MUST ship two profiles: `laravel-pest` and `generic`.

### 2.5 Roles

A role customises the prompt for one axis. `cr` owns the output contract; a role
only supplies persona and focus.

| Field | Type | Meaning |
|-------|------|---------|
| `id` | string | Role id, equal to the file stem, kebab-case |
| `title` | string | Human label shown in prompts and progress output |
| `axis` | string | The axis this role serves |
| `instructions` | string | The role's framing text |
| `focus` | array | Focus questions appended to the prompt |
| `profiles` | array | Profiles this role applies to; empty means all |

1. v0.1 MUST ship four default roles, one per axis: `intent-coverage`,
   `correctness`, `convention`, `test-adequacy`.
2. `cr init --eject-roles` MUST write the defaults as editable files that are
   byte-identical to the built-ins.
3. A malformed role or profile file MUST abort the command with exit code 3.
4. Role resolution order MUST be per-repository, then global, then built-in.

### 2.6 Rules

A rule is one normative statement about how code in a repository must be
written. Rules are data and are kept separate from roles: a role is a lens, a
rule is a specific standard that lens enforces.

| Field | Type | Meaning |
|-------|------|---------|
| `id` | string | Rule id, equal to the file stem, kebab-case |
| `title` | string | One-line statement of the standard |
| `rationale` | string | Why the standard exists, quotable to the author |
| `axis` | string | The axis that enforces it, defaulting to `convention` |
| `class` | string | Defect class assigned to records from this rule |
| `severity` | string | Default severity for records from this rule |
| `kind` | string | Default `finding` or `question` |
| `detect` | object | Optional mechanical detector per §2.6.1 |
| `fix` | object | Optional suggestion template per §2.6.2 |
| `globs` | array | Paths the rule applies to; empty means all source |
| `exempt` | array | Paths excluded, for legacy areas |
| `profiles` | array | Profiles the rule applies to; empty means all |

1. Rules MUST resolve in layers, highest first: per-repository, then global,
   then the profile's built-in rules.
2. A rule id MUST be unique after resolution; a lower layer carrying the same id
   is overridden whole, never merged field by field.
3. Every record produced by a rule MUST carry the rule id.
4. A record produced by a rule MUST be able to quote the rule's `rationale`, so
   the author learns the standard and not only the violation.
5. A malformed rule file MUST abort the command with exit code 3.

#### 2.6.1 Detection

1. A rule carrying a `detect` block MUST be evaluated mechanically by `cr` over
   the changed lines of the diff only, never over the whole repository.
2. `detect.pattern` MUST be a regular expression and `detect.mode` MUST be
   `regex` in v0.1.
3. A hit MUST grade the resulting record `cited`, because the rule together with
   the matched location is the citation.
4. A rule without a `detect` block MUST be injected into its axis role's prompt
   as text, and any record it produces is graded normally per §6.2.
5. Detection MUST report hits, never verdicts. The agent decides whether a hit
   is a real violation and MAY dismiss it with a recorded reason.
6. Dismissed hits MUST be counted per rule, so an imprecise pattern is visible
   in the statistics of §7.3.

#### 2.6.2 Fixes

1. A rule MAY carry `fix.replace` and `fix.with` as a regular expression and its
   replacement, applied to the matched line to produce a suggestion.
2. A generated suggestion MUST pass the validation of §8.2 before drafting; a
   suggestion that fails validation MUST be dropped while its record survives.
3. `cr` MUST NOT apply a fix to any file. A fix only ever produces suggestion
   text for the author to accept.

#### 2.6.3 Harvesting

1. `cr rules suggest` MUST scan the bodies of comments posted from recorded
   rounds and group them by class and by normalised body.
2. A group reaching `rules.harvest_min` occurrences, default 3, MUST be reported
   as a candidate rule together with the comments that formed it.
3. Candidates MUST be reported only. `cr` MUST NOT write a rule file by itself.
4. `cr rules list --dead` MUST report rules that produced no hit and no record
   across the last `rules.dead_after` rounds, default 20, so the corpus can be
   pruned rather than growing without limit.

### 2.7 Configuration layers

Effective configuration MUST resolve at read time in this order, highest first:

1. Command-line flags.
2. Environment variables prefixed `CR_`.
3. Per-repository config at `~/.cr/repos/<owner>/<repo>/config.json`.
4. Global config at `~/.cr/config.json`.
5. Built-in defaults.

`cr config` MUST print the effective configuration, and `cr config --resolved`
MUST annotate every setting with the layer that supplied it.

## 3. Inputs

### 3.1 Intent source

The tracker is read through a configured command, so `cr` carries no tracker
authentication code.

1. `intent.cmd` MUST be an argv array with a `{key}` placeholder.
2. The default MUST be `["jira", "issue", "view", "{key}", "--plain"]`.
3. A non-zero exit MUST fail with exit code 3 and surface the command's stderr.
4. `--intent-file <path>` MUST bypass the command and read the issue text from
   a file, so the loop works with no tracker access at all.

### 3.2 Issue key resolution

The issue key MUST be resolved from the first source that yields a match:

1. The `--issue <KEY>` flag.
2. The PR branch name.
3. The PR title.
4. The PR body.

The pattern MUST default to `[A-Z][A-Z0-9]+-[0-9]+` and be overridable by
`intent.key_pattern`. If no key is found, `cr` MUST continue with an empty
intent and mark the intent axis as unavailable per §4.5.

### 3.3 Claim extraction

Claims are produced by the agent from the issue text and recorded by `cr`.

| Field | Meaning |
|-------|---------|
| `id` | `<ISSUE-KEY>#c<n>` |
| `text` | The claim, verbatim or minimally normalised |
| `source` | `description`, `acceptance`, `comment`, or `note` |
| `hash` | Content hash used to detect issue drift |

1. `cr claims record <pr> <file.ndjson>` MUST validate and store claims.
2. Claims sourced from the context store (§3.6) MUST carry `source: note` and
   MUST be included in coverage exactly like tracker claims.
3. If the issue text changes between rounds, `cr` MUST report which claim hashes
   no longer match, and MUST NOT silently re-extract.

### 3.4 Diff ingestion and unit clustering

1. The diff MUST be taken against the PR merge base at the current head.
2. Files matching `ignore.globs` MUST be excluded and counted as excluded.
3. Hunks MUST be clustered into units by, in order: same file and same enclosing
   symbol when detectable; otherwise same file and adjacency within
   `cluster.gap_lines` (default 12); otherwise one unit per hunk.
4. A unit MUST NOT exceed `cluster.max_lines` changed lines (default 80); a
   larger cluster MUST be split.
5. Every unit MUST record its file paths, hunk ranges, and changed line count.
6. Binary and generated files MUST be listed but not clustered.

### 3.5 Existing threads

1. All existing review threads on the PR MUST be ingested, including resolved
   ones, with author, body, anchor, resolution state, and replies.
2. Each thread MUST be tagged with an author type of `human` or `bot`.
3. A finding whose anchor and class match an existing human thread MUST be
   suppressed and recorded as `suppressed_by_thread`.
4. Author replies inside ingested threads MUST be offered as candidate context
   notes per §3.6.

### 3.6 Context store

The context store holds facts that are true about the issue but absent from the
tracker.

1. `cr note <ISSUE-KEY> "<text>" --source <source>` MUST append a note with a
   timestamp, the PR it came from, and the source.
2. `cr answer <question-id> "<text>" --source <source>` MUST close the question
   and append the same note in one call.
3. Valid sources MUST be `chat`, `jira`, `thread`, `meeting`, and `other`.
4. Notes for the issue key MUST be loaded automatically on every subsequent
   round and every subsequent PR that resolves to the same key.
5. `cr context <ISSUE-KEY>` MUST print the accumulated notes with provenance.

## 4. Review axes

### 4.1 Intent coverage

1. Every unit MUST be mapped to zero or more claims.
2. A unit mapped to zero claims MUST raise an unmapped-unit item.
3. A claim mapped to zero units MUST raise an unimplemented-claim item.
4. An unmapped unit MUST default to a question, never a finding, because the
   most common cause is intent that never reached the tracker.
5. If the context store already explains an unmapped unit, the item MUST NOT be
   raised and the explaining note MUST be cited in the coverage cell.

### 4.2 Correctness against claims

1. Each unit MUST be evaluated against the claims it is mapped to.
2. A correctness finding MUST cite the claim id it violates.
3. A unit mapped to zero claims MUST still be evaluated for internal defects.

### 4.3 Convention and reinvention

1. For every function, method, or class added by the diff, `cr` MUST search the
   repository for candidate pre-existing symbols and attach the candidates to
   the unit.
2. Candidate ranking MUST use name similarity and signature shape; semantic
   equivalence is the agent's judgement, not `cr`'s.
3. A reinvention item MUST cite the candidate symbol as `path:line`.
4. Reinvention items MUST default to `kind: question`, because the tool cannot
   know whether the existing symbol was rejected for a reason.
5. Project conventions beyond reinvention MUST come from the rule corpus of
   §2.6, never from hard-coded logic and never from prose buried inside role
   instructions.
6. A mechanical rule hit MUST be attached to the unit that contains it, so the
   agent judges an already-located candidate instead of searching for one.

### 4.4 Test adequacy

1. Every unit MUST be classified as covered, partially covered, or uncovered by
   tests changed or added in the PR.
2. For every claim, the expected edge cases MUST be enumerated and each one
   marked as asserted, missing, or not applicable with a reason.
3. A missing edge case SHOULD be escalated to a probe per §5.
4. A test-adequacy finding without a probe MUST be graded `argued` and therefore
   posted as a question per §6.3.

### 4.5 Axis activation and honest reporting

1. An axis is active when it is enabled by the resolved configuration and its
   prerequisites are met.
2. The test axis MUST be disabled automatically when the profile declares no
   test command.
3. The intent axis MUST be marked unavailable when no issue key resolves.
4. A disabled or unavailable axis MUST appear in the coverage report with its
   reason. `cr` MUST NOT report a review as complete without stating which axes
   did not run.
5. Every cell MUST carry one of `pass`, `finding`, `question`, or `na`, and a
   cell with `na` MUST carry a reason.

## 5. Probes

### 5.1 Sandbox

1. `cr sandbox create <pr>` MUST create a git worktree at the PR head under
   `~/.cr/state/<owner>/<repo>/pr-<n>/sandbox/`.
2. Paths listed in `sandbox.copy` MUST be copied from the main checkout into the
   sandbox after creation.
3. Commands in `sandbox.setup` MUST run once, in order, in the sandbox root.
4. `cr` MUST NOT modify the user's main worktree, index, or current branch.
5. `cr sandbox destroy <pr>` MUST remove the worktree and its registration.
6. A sandbox whose head no longer matches the PR head MUST be recreated.

### 5.2 Test runs

1. `cr test <pr> [--filter <expr>]` MUST run the profile's test command inside
   the sandbox and record exit code, duration, and a truncated tail of output.
2. A baseline run with no filter MUST be recorded once per head, so that
   pre-existing failures are never attributed to a probe.
3. A run exceeding `tests.timeout_seconds` MUST be killed and recorded as
   `timeout`.

### 5.3 Mutation probe

A mutation probe proves a test gap by breaking production code.

1. The agent supplies a mutation as a unified diff against a sandbox file.
2. `cr probe run --kind mutation --patch <file> --filter <expr>` MUST apply the
   mutation, run the tests, revert the mutation, and record the result.
3. The mutation MUST be reverted even when the run fails or times out.
4. A result of `no-test-failed` proves the gap and grades the finding `probed`.
5. A result of `failed` disproves the gap; `cr` MUST record it and the agent
   MUST NOT raise the finding.

### 5.4 Gap probe

A gap probe distinguishes a missing behaviour from a missing test.

1. The agent supplies a new test file targeting the suspected edge case.
2. `cr probe run --kind gap --test <file> --filter <expr>` MUST place the test
   in the sandbox, run it, remove it, and record the result.
3. A failing gap probe means the behaviour is wrong; severity MUST be at least
   `high` and the probe output MUST be offered as a reproduction.
4. A passing gap probe means only the test is missing; severity MUST be at most
   `medium`.

### 5.5 Probe record

| Field | Meaning |
|-------|---------|
| `id` | `p<n>` |
| `kind` | `mutation` or `gap` |
| `head` | Head SHA the probe ran against |
| `target` | `path:line` the probe addresses |
| `input` | The patch or test file content |
| `filter` | The test filter used |
| `result` | `no-test-failed`, `failed`, `passed`, `error`, or `timeout` |
| `duration_ms` | Wall-clock duration |
| `output_tail` | Truncated runner output |

1. Probe records MUST be immutable once written.
2. A finding MUST reference at most one probe, by id.
3. Probe records whose head differs from the current head MUST NOT be used to
   grade a finding in the current round.

### 5.6 Isolation and budget

1. Probe and test runs MUST take an advisory lock named after the profile id, so
   two `cr` runs never share a test database.
2. When the lock is held, `cr` MUST wait up to `probe.lock_timeout_seconds`
   (default 300) and then fail with exit code 4.
3. `cr` MUST warn that an unrelated local test run can still collide, because
   the lock only covers `cr`'s own runs.
4. `probe.max_per_round` (default 10) MUST cap probe executions per round; the
   cap being hit MUST be reported, never silently applied.

## 6. Findings

### 6.1 Record schema

| Field | Meaning |
|-------|---------|
| `id` | `f<n>`, stable for the life of the PR |
| `kind` | `finding` or `question` |
| `axis` | The axis that produced it |
| `role` | The role that produced it |
| `class` | Kebab-case defect class, used for dedup and statistics |
| `rule` | The rule id, when a rule produced the record |
| `severity` | `critical`, `high`, `medium`, or `low` |
| `grade` | `probed`, `cited`, or `argued` |
| `unit` | The unit id |
| `claim` | The claim id, when the axis produces one |
| `anchor` | Anchor object per §9.2 |
| `summary` | One sentence, English, structured for dedup |
| `evidence` | What supports it, including cited `path:line` |
| `probe` | Probe id, when graded `probed` |
| `suggestion` | Exact replacement lines, optional |
| `state` | State per §9.1 |
| `thread_id` | GitHub thread id once posted |
| `round` | The round that produced it |

1. `summary` and `evidence` MUST be English. Reader-facing prose is produced at
   draft time per §8.1.
2. A finding MUST carry an anchor that resolves to a line in the current head.

### 6.2 Evidence grades

| Grade | Requirement |
|-------|-------------|
| `probed` | References a probe with a result that supports the claim |
| `cited` | References at least one concrete `path:line` outside the diff |
| `argued` | Neither of the above |

1. The grade MUST be computed by `cr` from the record, never asserted by the
   agent.
2. A record claiming `probed` without a valid same-head probe MUST be rejected
   with exit code 1.

### 6.3 The argued rule

1. A record with grade `argued` MUST be forced to `kind: question` at record
   time.
2. The forcing MUST be visible in the round summary, with a count per class.
3. `--allow-argued-findings` MAY override the rule for a single command, and its
   use MUST be recorded in the round.

### 6.4 Dedup

1. Findings MUST be deduplicated by `(anchor.path, anchor.line, class)`.
2. Within a duplicate group the representative MUST be the highest grade, then
   the highest severity, then the earliest role in corpus order.
3. Suppressed duplicates MUST be retained with a pointer to the representative,
   and the contributing roles MUST be reported as an overlap summary.
4. Findings matching an active waiver MUST be dropped and counted per §7.4.

### 6.5 Merge

1. `cr merge <files...> -o <out>` MUST merge per-role NDJSON files, apply §6.4,
   and report counts by role, axis, severity, and grade.
2. Missing required fields MUST fail with exit code 1 and name the offending
   line.

## 7. Draft and triage

### 7.1 Draft format

`cr draft <pr>` MUST render every queued record into a single Markdown file at
`rounds/<n>/draft.md`.

1. Each record MUST be rendered as a block introduced by an HTML comment marker
   carrying `id`, `kind`, `path`, `line`, `severity`, and `grade`.
2. The block body MUST be free-form Markdown that the user may rewrite entirely.
3. A suggestion MUST be rendered as a fenced `suggestion` block inside the body.
4. The file MUST open with a summary header listing counts and the coverage
   state, as comments that are not posted.
5. The draft MUST be regenerable; regenerating MUST preserve existing bodies for
   records the user already edited.

### 7.2 Triage verbs

Triage happens by editing the file. `cr post` interprets the result.

| User action in `draft.md` | Effect |
|---------------------------|--------|
| Leaves a block unchanged | Posted as rendered |
| Edits the body prose | Posted as edited |
| Changes `kind=finding` to `kind=question` | Softened, and recorded as a triage event |
| Deletes the block entirely | Discarded, and a waiver is written per §7.4 |
| Adds a block with `id=new` | Posted as a manual comment owned by the user |

1. `cr post` MUST refuse to run when a marker is malformed, naming the line.
2. Changing `path` or `line` in a marker MUST re-validate the anchor and fail if
   it no longer resolves.

### 7.3 Triage statistics

1. Every triage event MUST be recorded with class, axis, role, grade, and the
   action taken.
2. `cr stats` MUST report per class and per rule: raised, kept, softened,
   discarded.
3. A class whose discard-plus-soften rate exceeds `stats.demote_threshold`
   (default 0.6) over at least `stats.min_samples` (default 8) events MUST be
   listed as a demotion candidate, with the recommendation to default it to
   `question`.

### 7.4 Waivers

1. A waiver MUST be keyed by `(class, anchor content hash, normalised summary)`
   and scoped to the repository, not the PR.
2. A waiver for a record produced by a rule MUST also store the rule id, and MAY
   be widened to a path prefix so a rule can be exempted for a legacy area
   without disabling it repository-wide.
2. Waived findings MUST be dropped before drafting and counted in the round
   summary.
3. `cr waivers list` MUST print active waivers, and `cr waivers remove <id>`
   MUST delete one.
4. A waiver MUST record the round, the PR, and the reason when one was given.

## 8. Posting

### 8.1 Render contract

1. Comment bodies MUST be written in the language configured by `render.lang`,
   defaulting to `tr`.
2. `cr` MUST NOT translate. The agent composes the body; `cr` validates and
   stores it.
3. A body MUST be non-empty and MUST NOT contain the marker comment.
4. A question body SHOULD end with an explicit question, and `cr` MUST warn when
   a `kind=question` body contains none.

### 8.2 Suggestion validation

1. A suggestion MUST map to a contiguous line range that exists in the PR diff
   on the `RIGHT` side.
2. The range MUST be within one hunk.
3. Leading whitespace of the first replaced line MUST be preserved unless the
   suggestion explicitly changes indentation.
4. A suggestion failing validation MUST block posting with exit code 1 and name
   the record id.

### 8.3 Batching

1. All comments in a round MUST be posted as one GitHub review, so the author
   receives a single notification.
2. The review event MUST be `COMMENT` in v0.1. `cr` MUST NOT emit `APPROVE` or
   `REQUEST_CHANGES`.
3. The exact posted payload MUST be written to `rounds/<n>/posted.json` before
   the network call, and updated with returned thread ids after it.

### 8.4 Partial failure

1. If the review call fails, no state MUST be marked posted.
2. If individual comments are rejected by GitHub, the successful ones MUST be
   recorded and the failures reported with their reasons and exit code 4.

### 8.5 Confirmation gate

1. `cr post` without `--confirm` MUST print the full payload and exit 0 without
   posting.
2. `--confirm` MUST be required for every network write, on every round.
3. There MUST be no configuration setting that removes the gate.

## 9. Re-review

### 9.1 Finding states

| State | Meaning |
|-------|---------|
| `draft` | Recorded, not yet queued for a draft |
| `queued` | Rendered into the current draft |
| `discarded` | Deleted during triage, waiver written |
| `posted` | Sent to GitHub, thread created |
| `awaiting-author` | Posted, no author response yet |
| `answered` | Author replied without changing code |
| `verifying` | Author pushed code touching the anchor |
| `fixed` | Verified as addressed, thread resolvable |
| `accepted` | Reviewer waived it after discussion |
| `regressed` | The fix introduced a new problem, linked to a new record |
| `stale-anchor` | The anchor no longer resolves after a force-push |

1. Transitions MUST be recorded with a timestamp, the head SHA, and the actor.
2. Only `fixed` and `accepted` are terminal.

### 9.2 Anchors and migration

An anchor MUST carry `path`, `side`, `start_line`, `line`, a normalised content
hash of the anchored lines, and up to three lines of context on each side.

1. On a head change, every non-terminal anchor MUST be re-resolved by searching
   the new file for the content hash, then for the context window.
2. A re-resolved anchor MUST update its line numbers and record the migration.
3. An anchor that cannot be re-resolved MUST move to `stale-anchor` and be
   surfaced for a decision; `cr` MUST NOT silently drop it.

### 9.3 Recheck

`cr recheck <pr>` MUST, for the new head:

1. Migrate anchors per §9.2.
2. Classify each open thread as untouched, touched, or stale.
3. Move touched threads to `verifying` and emit verification prompts.
4. Compute the delta diff since the last reviewed head and cluster new units.
5. Open a new round for the delta, reusing waivers and the context store.
6. Report which prior claims changed, if the issue text drifted.

### 9.4 Verification outcomes

For each `verifying` record the agent MUST record exactly one outcome:

1. `fixed` — the concern is addressed; `cr` MUST offer to resolve the thread.
2. `partial` — a reply is queued in the next draft and the state returns to
   `awaiting-author`.
3. `regressed` — a new record is created and linked to the original.

### 9.5 Thread resolution

1. `cr resolve <id> --confirm` MUST resolve the GitHub thread and set the record
   to `fixed`.
2. Resolution MUST require evidence text, stored on the record.
3. Resolving MUST be subject to the same confirmation gate as posting.

## 10. Reporting and convergence

### 10.1 Coverage report

`cr status <pr>` MUST report:

1. Units total, units with a complete row of cells, and units with gaps.
2. Claims total, claims mapped to units, and claims with no implementation.
3. Active axes, disabled axes with reasons, and unavailable axes with reasons.
4. Findings and questions by state, severity, and grade.
5. Probes run, and probes that changed a finding's grade.
6. Waivers applied and duplicates suppressed in the current round.

### 10.2 Convergence

A PR is converged when all of the following hold:

1. The current head equals the head of the last recorded round.
2. Every unit has a complete row of cells for every active role.
3. Every claim is either mapped to a unit or has an open unimplemented-claim
   record.
4. No record is in `draft`, `queued`, `posted`, `awaiting-author`, `answered`,
   `verifying`, or `stale-anchor` with severity `critical` or `high`.

`cr status` MUST print the convergence verdict and, when it is false, the exact
reason. `cr` MUST NOT approve the PR; the verdict is advice to the reviewer.

### 10.3 Round summary

Every round MUST record counts for raised, deduplicated, waived, forced to
question, drafted, posted, and discarded records, so that the history of a
review is reconstructable from state alone.

## 11. Command surface

| Command | Purpose |
|---------|---------|
| `cr init` | Create `~/.cr`, write default profiles and roles |
| `cr init --eject-roles` | Write built-in roles as editable files |
| `cr brief <pr>` | Read-only orientation payload for a PR |
| `cr review <pr>` | Emit per-role, per-unit prompts and output paths |
| `cr claims record <pr> <file>` | Store extracted claims |
| `cr merge <files...> -o <out>` | Merge and deduplicate role findings |
| `cr record <pr> <file>` | Record a round's merged findings |
| `cr sandbox create\|destroy <pr>` | Manage the probe worktree |
| `cr test <pr> [--filter]` | Run the test suite in the sandbox |
| `cr probe run <pr> ...` | Execute and record a probe |
| `cr draft <pr>` | Render the editable draft |
| `cr post <pr> [--confirm]` | Validate and post the review |
| `cr recheck <pr>` | Re-review after a head change |
| `cr resolve <id> --confirm` | Resolve a thread with evidence |
| `cr answer <question-id> <text>` | Close a question and store a note |
| `cr note <ISSUE-KEY> <text>` | Store an out-of-band note |
| `cr context <ISSUE-KEY>` | Print accumulated notes |
| `cr waivers list\|remove` | Inspect and edit waivers |
| `cr stats` | Triage statistics and demotion candidates |
| `cr rules list` | Effective rules and the layer each came from |
| `cr rules check <pr>` | Run mechanical rule detection over the diff |
| `cr rules suggest` | Propose rules from recurring comment history |
| `cr status <pr>` | Coverage, states, and convergence |
| `cr config [--resolved]` | Effective configuration |

### 11.1 Global flags

| Flag | Effect |
|------|--------|
| `--json` | Force JSON output |
| `--compact` | Minimal JSON |
| `--quiet` | Suppress informational messages |
| `--no-color` | Disable colour |
| `--repo <owner/repo>` | Override repository detection |

### 11.2 Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Validation failure |
| 2 | Usage error |
| 3 | File, configuration, or external command failure |
| 4 | State conflict, including lock timeout and partial post |

## 12. Output contract

1. Output MUST be JSON when stdout is not a terminal or `--json` is given, and
   human-readable text otherwise.
2. JSON MUST be pretty-printed with two-space indentation.
3. Slice fields MUST serialise as `[]` and never as `null`.
4. Every error MUST carry a `hint` field naming the next actionable step.
5. `--compact` MUST omit prose fields that an agent does not need to act:
   `evidence`, `output_tail`, `input`, and ingested thread bodies.

## 13. Skill

`skills/cr/SKILL.md` MUST ship with the release and MUST document:

1. The full loop: brief, review fan-out, merge, record, probe, draft, human
   read, post, recheck.
2. The argued rule and when to escalate to a probe instead of asserting.
3. The draft triage verbs and that deletion writes a waiver.
4. That every network write needs `--confirm` and that the human reads first.
5. The role, profile, and rule file formats, and how to add each.
6. That a comment written by hand more than twice belongs in the rule corpus,
   with a `detect` block when the violation is mechanically matchable.

## 14. Distribution

1. `cr` MUST build as a single binary whose only runtime dependencies are
   `git`, `gh`, and the configured tracker command.
2. Releases MUST be produced by GoReleaser on a `v*` tag, publishing archives
   for darwin and linux on both amd64 and arm64.
3. A release MUST update the Homebrew formula in the `deligoez/homebrew-tap`
   repository so that `brew install deligoez/tap/cr` installs the binary as
   `cr`.
4. `go install github.com/deligoez/cr/cmd/cr@<tag>` MUST install the same
   version as the tag.
5. The skill MUST ship inside this repository at `skills/cr/SKILL.md`, and
   `.claude-plugin/marketplace.json` MUST expose it as an installable plugin.
6. `cr --version` MUST print the tagged version, injected at build time.
