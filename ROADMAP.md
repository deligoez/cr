# cr — Roadmap

`VISION.md` says why cr exists; each `spec/<version>.md` is the normative contract of one release. This
file is neither. It is the working list of what cr lacks, what is sequenced next, and what has to be
measured before it is decided.

**An item leaves this file three ways and no other: it becomes a spec, it is dropped with its reason, or it
is written into Not planned.** Nothing stays by default, and nothing accretes.

The lesson that orders the list: **what cr has not been run against, it has not been shown to do.** Three
measurements now stand behind the ordering, all under `spec/measurements/`; the version history the first
paragraphs of this file used to carry is the Shipped table below.

## Shipped

| Version | What it delivered |
|---------|-------------------|
| v0.1.0 | One-head reviewer loop: intent, four axes, probes, draft, human triage, one posted review |
| v0.2.0 | The loop repaired against real pull requests; marker `side`, context-window waiver keys, `commit_id`-pinned reviews, exit-code-aware probe ladders, closed/merged disclosure |
| v0.2.1 | Known low defects and two limitations of v0.2.0 (per-clone probe lock, runner start window), no contract change |
| v0.2.2 | Test-environment safety after a field trial ran the sandbox against a developer's application database: `.env.testing` copied, every gitignored env file the sandbox lacks reported, an experiment header before every run, a stale sandbox rebuilt |
| v0.2.3 | What the field trial's operator asked for, within the v0.2 contract: notes that postdate a prompt reported, recorded cells marked, duplicate candidates listed, issue text cleaned and its links and uncovered paragraphs disclosed |
| v0.3.0 | The contract caught up with the trial: a probe baseline scoped to its own filter and paths, `sandbox.require`, `--intent-extra` issue files, a `cr review` that emits only what the round still owes (`--units`, `--shard`, `--all`) over one per-round contract file, role class vocabularies, `cr triage`, §2.3's state-file fence, and `cr init` refreshing ejected roles |
| v0.3.1 | The six defects cr found reviewing its own v0.1.0 packages (`spec/measurements/2026-09-18-m1-…`): the reserved marker sequence refused where a record enters, a rewrite refusal offering only the remedy that works, a corrupt `rendered.json` exiting 3, a sandbox directory that is not a readable worktree rebuilt, a copy that no longer writes through a symbolic link the head checked out, a bounded wait after the timeout kill |

## What has been measured

| | Instrument | Result |
|---|---|---|
| M1, 2026-09-18 | cr's reading roles, then its probes, over its own v0.1.0 packages, against 24 defects a 478-case QA pass had found | Reading **0 of 24**, probes **0 of 24**, false assertions **0**; the probes proved 21 real test gaps and decided two suspicions by experiment. QA 24, reading 0, probes 0 — the instrument matches the defect |
| M3, 2026-09-18 | cr against a careful human review on a real pull request (tarfin-labs/backend#3757), threads shimmed out of its ingestion | **8 of 16** of the human's comments recovered, 43 cr-only records, false assertions **0 of 9** verified by hand; seven of the eight misses are house style |
| AACR-Bench, external | Alibaba's 2145 expert-labelled comments over 200 pull requests | Evidence distance predicts correctness (0.741 / 0.696 / 0.607, non-overlapping CIs); comment *wording* predicts nothing. See Questions, settled |

## Next

### 0. Defects (before any feature)

- **A credential-shaped file in the diff reaches every prompt.** Measured end to end, twice: a tracked
  `.env`, `id_rsa` or `.netrc` is not matched by `ignore.globs` (which defaults to empty,
  `internal/config/config.go:56`), is not binary and is not generated, so `internal/unit/files.go`'s
  four-way sort clusters it, and `internal/review/text.go:118` writes its added lines into every role's
  prompt as a fenced diff block. §3.4 has no fence and §5.1.2's `.env*` report is about a different
  question — what the *sandbox* lacks. The fix is a built-in path fence applied before `ignore.globs`, and
  it **discloses rather than silently drops**: §4.5.4's honesty obligation means the brief says a file was
  fenced, or a reviewer cannot tell a fence from an oversight. No configuration key turns it off in the
  first release.
- **A runner's survivor outside the process group is neither killed nor reported.** v0.3.1 bounded the wait
  (`cmd.WaitDelay`), so a descendant that left the group no longer blocks the run for ever; cr cannot signal
  it, so the honest remedy is the disclosure that does not exist yet — the shape `StoppedRunner.Lingering`
  already uses. `spec/field-feedback.md` M-1.6's tail.

### 1. The basics cr lacks

Each of these is measured rather than wished for: the evidence is a run that had to work around its absence.

- **A role cannot propose an experiment.** §5's probes are cr's differentiator and the fan-out gives no way
  to reach them: a role holds the suspicion, and §6.1 gives it only `probe`, a field for the *id* of an
  already-executed probe. Measured twice — 15 records in one run and 10 in another wrote prose into that
  field ("Take an application for which shouldInitializeMachine() is false…"), and both measurements needed
  a bespoke side-channel before any probe could run. Widening `probe` to carry prose is ruled out on cr's
  own grounds: §6.2.1 closes the grade's inputs and `internal/finding/grade.go` makes "evidence prose is
  never parsed" structural rather than a promise, so a prose proposal is written into the one field cr has
  decided never to read. **The shape is a per-round proposals file** the roles write and `cr probe run`
  consumes — exactly `cr claims record`, `cr map record` and `cr cells record`: role writes a file, cr
  validates and ingests. A proposal carries kind, target, the mutation or command, and the outcome that
  would settle it, so it is machine-checkable the way a claim is; `probe` stays an id, and a proposal
  becomes evidence only by being run. **Not tool-shaped**, and that is decided rather than open: cr is a CLI
  an orchestrator drives, not a server a model calls mid-reasoning, and P5 keeps it that way. The opposite
  answer exists to look at — Alibaba's reviewer gives its agent six tools and no execution at all, and its
  published ceiling is 37.8% precision at 28.9% recall.
- **Nowhere to put house style.** M3's miss set is the evidence: seven of the eight comments cr missed
  against a careful reviewer are shaping comments — name this test that way, extract this helper, call this
  variable `$specification`. House style is not in the issue and not in the diff. §2.6's rule corpus is the
  right home and cr ships **no rules at all**, while `cr rules suggest` harvests only from *posted* rounds,
  so a team's first review — where style matters most — has nothing. Ship a starter corpus and a way to seed
  one; Alibaba's `rule_docs` (52 per-language guides, Apache-2.0, headed by "Favor precision over recall… a
  false positive costs reviewer trust") is the first source, as a corpus intake with attribution, not as an
  adoption of their selector.
- **Two profiles is not a tool.** Only `laravel-pest` and `generic` ship, so on any other stack the
  reinvention lens is off, the test axis needs a hand-written `tests.cmd`, and probes have no template. The
  Go profile is written and measured already (`spec/measurements/m1/go-profile.json`: runner, count
  patterns, filter and path flags, probe template), so it is a commit rather than a project; TypeScript and
  Python follow.
- **Every non-Jira team writes configuration before its first review.** `intent.cmd` defaults to a `jira`
  binary (§3.1.2), and the intent axis is the one P1 calls the authority, so a team on GitHub Issues or
  Linear cannot run a first round as shipped: it configures a tracker command or passes `--intent-file` by
  hand. Ship the two commands. This is a tracker, not a profile, and it is listed on its own because the
  file's opening rule forbids riders.
- **A driver for the mechanical steps.** A round is ten commands of which three need the agent's judgement.
  Measured: the orchestration was written three separate times for the three measurement passes, and every
  time the plumbing — which prompts to run, where their output goes, collecting the cells — was the
  operator's to build rather than cr's. §7.3's statistics, the probe cap's report, and the round's
  completeness all exist; what is missing is the one command that walks the mechanical half.

### 2. Measure what is still unmeasured

M1 and M3 answered two of this item's three slices. What is left has no data at all:

- **Real use.** Review real pull requests with cr, posting only what the reviewer would have posted anyway,
  and record per round: comments kept, softened, deleted, marked `wrong`; author replies; wall clock and
  token cost. §7.3's triage statistics exist for exactly this and hold one round's data from one field
  trial. It is the same question the volume question below asks, from the other end. **Blocked on a decision,
  not on work:** which real pull requests cr is pointed at, and whether its output is posted under the
  reviewer's name. Both are the user's to make, and nothing here proceeds until they are made.
- **AACR-Bench as an instrument.** Its 640 negatives are a straight binary test of whether cr's grade ladder
  and §6.3's question-forcing separate wrong from right, with no matcher needed and no scoring script (which
  is unpublished). cr's own false-assertion denominators are 6 and 9; this one is 640. Recall over its 245
  Go rows is a second, larger slice, conditional on the first and on the Go profile.

### 3. Scenario coverage

M1 gave all three instruments a number against the same 24 defects: a QA pass that drove commands found 24,
reading roles 0, probes proposed by those roles 0. A probe decides a suspicion; nothing in the loop
*produces* the suspicion a scenario produces. Candidates, cheapest first: a role whose prompt carries the
command surface and asks what a user would do with this unit and what would then go wrong; a fixture
repository the round drives commands against, which is what `deligoez/cr-qa` already is for cr itself;
`cr probe run --kind gap` used the way the QA used its cases, one command sequence per probe rather than one
unit test. Decide after a cheap trial: one scenario-shaped role over the same subject, counted against the
same 24.

### 4. The conversation (v0.4)

The re-review half the first spec promised:

- anchor migration across a push, using the context window and content hash v0.1 onward already record.
  Alibaba's `internal/diff/resolver.go` is the working reference for the shape: the agent supplies a
  verbatim excerpt rather than a line number, and the tool finds it by normalised sliding-window match over
  the hunk's new side, then the old side, then the file, declining on zero or multiple hits;
- recheck and verification: after the author pushes, is each posted concern addressed;
- resolving and withdrawing threads, still behind `--confirm`;
- reading author replies from GitHub instead of storing them by hand with `cr answer`;
- a question closed by an answer is neutral, an open question is not — the rule a convergence criterion
  needs.

### 5. Team use

- **A second reviewer starts from nothing.** State, waivers and triage statistics live in one reviewer's
  `~/.cr`, so "this class is wrong" and the context store are per-person: the two a second reviewer needs on
  their first round are shared repository-wide waivers and a shared context store. Opens when a second
  reviewer uses cr; cr has one today, so there is no measurement that a team wants any of this, and the
  trigger is what discharges the item rather than somebody's spare afternoon.

## Questions to settle by measurement

- **Does the question channel converge?** If the question-to-finding ratio on a real spec is high, a
  convergence rule that counts open questions never reaches clean, and one that does not count them loses
  recall. First data points: M1, 21 questions to 2 findings with the spec's items as claims, 6 to 4 without;
  M3, 42 questions to 9 findings against a human review. Every finding in both was true. The ratio is high
  exactly where the claims are.
- **Is 43 records a good round or a bad one?** M3 produced 43 cr-only records beside a careful human's 25
  comments, and all nine of its assertions were true — but the question is about the whole set reaching an
  author, not the assertions. AACR-Bench's sharpest structural result is that precision collapses on volume
  alone and on nothing else: 465 comments buys 37.8%, 5980 comments buys 7.2%, same model family. Nobody
  knows which side of that curve 43 sits on. §7.3's triage statistics are the instrument and have one
  round's data; this is the same question "real use" above ends on.
- **Does evidence distance predict correctness, and does register?** *Settled, both halves*, against
  AACR-Bench (Apache-2.0: 2145 expert-labelled review comments over 200 real pull requests, 50 repositories,
  10 languages, 1505 correct and 640 incorrect). Comments the benchmark's experts annotated as needing only
  the diff are right 74.1% of the time (n=1017, 95% CI [0.714, 0.768]); file-level 69.6% (n=744);
  repository-level 60.7% (n=384, CI [0.558, 0.656]). The outer intervals do not overlap, and the ordering
  survives every available control: within each category (Code Defect 0.738/0.685/0.626, Maintainability
  0.733/0.686/0.545, Security 0.757/0.708/0.615), within each author including the 548 human-written
  comments (0.763/0.712/0.590), and the context mix is near-identical across all seven authors, so it is not
  a proxy for who wrote the comment. That is §6.2's axis, measured from outside cr: the further the evidence
  sits from the diff, the likelier the comment is wrong. The negative half is as load-bearing. No property
  of a comment's *wording* predicts anything: hedging present versus absent is −0.023, assertive wording
  −0.013, and the sharpest cut — two or more hedges with no assertive verb (0.668) against assertive verbs
  with no hedge (0.687) — runs the wrong way and lies inside noise, as do backtick density, explicit line
  references, note length and anchor span. §6.2.1's "`evidence` prose is never parsed" is therefore not
  fastidiousness: a grader or filter keyed on how confident a finding sounds would be keyed on noise. What
  is *not* settled is whether cr's own graders can recover the distance from what they see. A blind pilot
  (58 comments, balanced, stratified on context, real diffs, graded before labels were joined) returned
  cited 0.433 against argued 0.571, z = −1.05 — but at n=58 it could only have detected a gap of about 0.30
  where the real gap is 0.134, so the null is uninformative and refutes nothing. Its one usable output is a
  rubric lesson: "settleable by pointing" collects comments the diff *refutes* as well as ones it supports,
  and a post-hoc grader conflates the two. If that generalises, the grade can only be assigned honestly at
  production, by the role holding the evidence with its citation attached — where §6.2 already puts it, and
  an argument against any late grading or filter pass.

## Not planned

An item here has been considered and declined. It returns only with a measurement that overturns the reason.

- **A whole-change pass for cross-cutting requirements.** A unit is built from hunks, so a requirement that
  spans the change ("every write takes the lock") is seen in pieces. The item's own condition was to decide
  it after M1 showed whether the misses clustered there; M1's misses were case-shaped and M3's were house
  style, so the condition is discharged negative, twice. And there is now a number for why it is the
  expensive direction: 18% of AACR-Bench's comments (384 of 2145) need repository-level context, and that
  band has the worst precision of the three at 0.607. A whole-change pass buys the least reliable comments
  at the highest cost.
- **The author side** — ingesting incoming review comments on the user's own pull requests as a work list.
  It is a second product, not a later item of this one, and leaving it in Next is what turns a plan into a
  wish list.
- **GitLab and Bitbucket.** Not until a user needs them; cr's single network-write door keeps that change
  contained when one does.
- **A post-hoc filter or fact-check pass over merged findings.** Measured and declined: a diff-only grader
  could not recover evidence distance (above), and removing exactly the class it can identify would have
  dropped 5 wrong comments and 3 correct ones — a 1.67:1 trade cr's asymmetry cannot buy at a prompt per
  finding.
- **A risk plan before the fan-out.** Alibaba's reviewer builds one and then strips it from round 2 onward
  because it caps recall. cr already has a pre-pass with provenance: the intent axis maps claims to units,
  and §6.2 can grade what rests on it. A second, unsourced plan would give the roles a ceiling and cr
  nothing it could grade.

## Known limits kept by design

- **cr runs the pull request's `tests.cmd` with the reviewer's permissions**, in a git worktree on the
  reviewer's own machine. It is not to be pointed at an outside contributor's pull request until that run
  happens in a container with no network by default and an explicit profile opt-in for anything else. This
  is a gate rather than a task: it binds today, and the work behind it opens the day the first
  outside-contributor pull request is reviewed.
- `COMMENT` is the only review event; cr never approves or requests changes.
- An interrupted runner exits 4; the next command recreates the sandbox.
- The out-of-block record-id refusal applies once `cr review` has emitted the round.
- `cr status` on a moved head omits the halves computed for a head that is no longer current.
- Windows is not supported: probes rely on process groups and `flock`.
