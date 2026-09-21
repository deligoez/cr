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
| v0.4.0 | §5.7, proposed experiments: a role that holds a suspicion it cannot establish writes it as a proposal, `cr proposals record` stores it, and `cr probe run --proposal` executes it and re-grades the record it names. The evidence was M3's grade distribution, 0 probed of 51 records |
| v0.4.1 | The three defects measurement 4 found, and nothing else: §3.4.7 names the credential kind (a debt owed since v0.3.2), a gap probe's test may be placed under its target's directory and its run is scoped to that directory rather than to the file, and the two proposal fields the prompt never explained — `filter` and `paths` — are explained, as is §8.1.3's reserved sequence |
| v0.3.2 (untagged, shipped inside v0.4.0) | The two defects listed before any feature: a credential-shaped file (`.env`, `id_rsa`, `*.pem` and the rest of a built-in fence read before `ignore.globs`) is listed by path and not clustered, so its content reaches no role's prompt; and a process of the test runner still holding the runner lock when the run ends is disclosed by `cr test` and `cr probe run` |

## What has been measured

| | Instrument | Result |
|---|---|---|
| M1, 2026-09-18 | cr's reading roles, then its probes, over its own v0.1.0 packages, against 24 defects a 478-case QA pass had found | Reading **0 of 24**, probes **0 of 24**, false assertions **0**; the probes proved 21 real test gaps and decided two suspicions by experiment. QA 24, reading 0, probes 0 — the instrument matches the defect |
| M3, 2026-09-18 | cr against a careful human review on a real pull request (tarfin-labs/backend#3757), threads shimmed out of its ingestion | **8 of 16** of the human's comments recovered, 43 cr-only records, false assertions **0 of 9** verified by hand; seven of the eight misses are house style |
| M4, 2026-09-19/21 | v0.4.0's §5.7 channel: part A, 76 role sessions on a real pull request (tarfin-labs/backend#3757) carrying no sandbox; part B, 22 sessions on cr's own v0.1.0 packages with a `go test` sandbox | A: the channel carries — well-formed proposals, none usable as evidence. B: 17 proposals accepted, 9 run, **7 records reached `probed`** against M3's 0, all 7 verified green by hand under `go test -overlay` with a control mutant red. **B4 false assertions 0, but vacuously** — §5.7.4 raises the *grade*, and the *kind* stays `question` until a human edits the draft. Of the three runs that produced nothing, one was the role misreading the unexplained `paths`, and **both gap probes were cr's**: §5.4.2's fixed `tests.probe_path_template` cannot place a Go test in the package it tests |
| AACR-Bench, external | Alibaba's 2145 expert-labelled comments over 200 pull requests | Evidence distance predicts correctness (0.741 / 0.696 / 0.607, non-overlapping CIs); comment *wording* predicts nothing. See Questions, settled |

## Next

### 0. Defects (before any feature)

- **A run that did not compile is disclosed as `no-tests-selected`, the same word as a filter that
  matched nothing.** §5.3.4's rung 4 and §5.4.3's rung 3 fire on `tests_run == 0` before any rung
  reads the exit code, and both carry an empty `reason`, so the operator sees one word for two
  situations that need different fixes. **The trust-economy half is already closed and was checked
  rather than assumed:** `run.Verdict()` conjoins four clauses — not contaminated, exit 0,
  `tests_run > 0`, `tests_failed == 0` — so a build-failed run is stored `passed: false` and can
  never support a `probed` grade. Measured on M4's stored runs: `r5` is a baseline carrying no probe,
  exit 1, 0 ran, `passed: false`. What is left is disclosure, and it is not cosmetic: reading
  `no-tests-selected` and stopping there is exactly what made M4's first write-up blame the wrong
  clause for two of its three lost runs. Discharged when a zero count from a non-zero exit carries a
  `reason` naming the exit status, as §5.3.4's `error` and `inconclusive` rungs already do.

*The three below were v0.4.1's whole content, and each is discharged with the evidence that closed
it:*

- ~~**§3.4.7 does not name the credential kind cr now lists.**~~ `spec/0.4.1.md` §3.4.7 names it a
  third listed kind beside `binary` and `generated`, with the match rule written out and the clause
  that no configuration key turns it off. The debt was two versions old and had never left
  `spec/0.3.2-release-notes.md`.
- ~~**§5.4.2's fixed `tests.probe_path_template` cannot place a Go gap probe's test.**~~ The
  template's directory may now be `<target-dir>`, resolved to the directory of `--target`, and the
  probe's own run is scoped to that directory rather than to the placed file. Discharged against the
  criterion this entry set — a Go gap probe reaching a verdict on this tree — with M4 part B's own
  failed proposal, `x36702`: the same `package probe` test, the same target, placed now at
  `internal/probe/cr_probe_p1_test.go` and run as `./internal/probe`. Before: `0 ran, 0 failed`,
  `undefined: Target`, `result: no-tests-selected`. After: `1 ran, 1 failed`, `result: failed`,
  over a baseline of 1453 passing. §5.1.6's leftover scan follows: an any-depth glob is walked
  rather than globbed, verified by planting an artefact two directories down and watching the
  sandbox be recreated for it.
- ~~**A proposal's `filter` and `paths` are the two fields the prompt never explains.**~~ The prompt
  now says they scope the run and not the code, that `filter` is a test name rather than a runner
  argument string, that `paths` are test targets and never the file the patch changes, and that
  leaving both out runs the whole suite. §8.1.3's reserved sequence, the other fence M4 found a role
  was judged by and never told, is now named in the round's contract file beside §8.1.5's.
  The reservation itself stands — it is what keeps a record's prose from splitting its own draft
  block — so what v0.4.1 changed is that a role is told about it and given a way through.

### 1. The basics cr lacks

Each of these is measured rather than wished for: the evidence is a run that had to work around its absence.

- ~~**A role cannot propose an experiment.**~~ *Shipped in v0.4.0 as `spec/0.4.0.md` §5.7, and measured
  in M4: roles do propose experiments worth running — 17 accepted over two parts, 9 run, 7 records
  moved `argued` → `probed`, every one of the seven verified by hand. What M4 did **not** settle is
  whether that changes anything a reviewer sends: the grade moves, the `kind` does not, and only a
  human's draft edit turns a probed question into a finding. The paragraphs below are kept as the
  record of why the shape is what it is.*
  §5's probes are cr's differentiator and the fan-out gives no way to reach them: a role holds the
  suspicion, and §6.1 gives it only `probe`, a field for the *id* of an already-executed probe.
  Measured twice: 15 records in one run and 10 in another wrote prose into that field ("Take an
  application for which shouldInitializeMachine() is false…"), and both measurements needed a bespoke
  side-channel before any probe could run. Widening `probe` to carry prose is ruled out on cr's
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
  so a team's first review — where style matters most — has nothing. **Half shipped in v0.3.2**: the
  `laravel-pest` profile now carries four rules drawn from M3's own miss set, in the profile layer §2.6
  already defines, each in the `question` register because they are style cr proposes rather than style the
  team declared. What is left is the seeding — no other profile has a corpus, and `cr rules suggest` still
  harvests only from posted rounds.

  **This entry used to name Alibaba's `rule_docs` as its first source, and that was the wrong pairing.**
  `cr-research` read the corpus: 52 language files, 2403 lines, 1428 prose bullets under per-file
  `#### Topic` headings that share no taxonomy — only "Obvious Typos or Spelling Errors" repeats, in 21
  files. Against §2.6's eight fields **one maps and seven do not**: `system_rules.json`'s `path_rule_map`
  gives glob → document (`**/*.go` → go.md), but at *file* granularity, so it is one glob for 69 lines of
  prose rather than a glob per rule. No ids, no classes, no severities; its bullets are instructions to a
  reviewing model where cr's `rationale` is a sentence quotable to the *author*; and 2 of the 52 files so
  much as mention a regex, so there are no `detect` patterns at all. cr does accept a prose rule — §2.6.1.4
  injects one with no `detect` block into its axis role's prompt — so a converted bullet is a first-class
  rule, but converting is authoring per rule and hopeless at 1428.
  The decisive objection is not the shape, it is the aim. M3's misses were shaping comments on a **PHP**
  pull request, and every one of `php.md`'s nine headings is defect-oriented; its single line touching
  naming reads *"Do not make formatting, naming, import ordering, modern-syntax preferences, or advice
  already enforced by deterministic PHP tooling into blocking findings."* The corpus is deliberately tuned
  **against** the register M3 found cr missing — that is how it buys precision, and why its published
  ceiling is 28.9% recall. Shipping it would have caught none of cr's eight misses and might have
  suppressed the two things cr did find in that register.
  So **house style has to come from the team**, by definition, and the remedy is seeding a corpus from a
  team's own code and review history. Discharged when a team can seed one without writing every file by
  hand, and `cr rules suggest` harvests from something other than posted rounds.
- **No per-language correctness corpus.** This is the half split out of the item above, and it has a
  source: Alibaba's `rule_docs` is a reasonable first intake for per-language *defect and security*
  rules, with attribution, as a corpus intake and not as an adoption of their selector. It pairs with
  profiles rather than with house style. Discharged when a shipped profile other than `laravel-pest`
  carries a corpus.
- **Two profiles is not a tool.** Only `laravel-pest` and `generic` ship, so on any other stack the
  reinvention lens is off, the test axis needs a hand-written `tests.cmd`, and probes have no template.
  TypeScript and Python follow the Go one.

  **This entry said the Go profile "is a commit rather than a project" and that was wrong.** Measured
  2026-09-21 by trying it: the profile in `spec/measurements/m1/go-profile.json` is not shippable,
  and a third profile is a normative change. Two things, both cheap to know and expensive to
  rediscover.

  1. **`go test` prints no count cr can read.** §5.2.1 takes a run's executed and failed counts from
     `tests.count_pattern`, and `internal/run/counts.go`'s `sum` adds the capture group of every
     match — `go test -v` emits `--- PASS: TestFoo (0.00s)` with no number on it and no total
     anywhere, so no regex can count. With no counts, §5.3.4's and §5.4.3's ladders can only answer
     `inconclusive`, and every probe on a Go repository establishes nothing. The m1 profile hid this
     behind a wrapper script at an absolute scratchpad path, which is why it read as finished.
     A first answer wrapped `go test` in a `sh -c` script that printed a recap line. **Take the other
     answer**: `cr-research` measured the same ground and located the mismatch in cr rather than in
     Go. `sum()` assumes a runner prints *a recap line containing numbers*; Pest does, `cargo test`
     does (`test result: FAILED. 1 passed; 1 failed; …`, measured), and `go test` prints *one line
     per test* instead. So the fix is **a second count mode that counts occurrences of the pattern**
     rather than summing its captures — and then Go needs no wrapper, no `sh -c`, no shipped shell.
     `tests.cmd` becomes `["go","test","-v","-count=1"]` with `^--- (PASS|FAIL|SKIP): ` and
     `^--- FAIL: `.
     Anchoring at `^` is load-bearing and measured twice, by me and by research, agreeing:
     `./internal/text` reads **12** anchored and **66** with leading whitespace allowed, because Go
     indents subtests — and `go test -json`'s top-level pass/fail events for the same package read
     **12**, which is the instrument check. A filter matching nothing reads 0, so the ladder still
     answers `no-tests-selected`.
     `probe_path_template` is `<target-dir>/cr_probe_<probe-id>_test.go`, which v0.4.1 made possible.
     `gotestsum` is ruled out — it would put a network fetch in `sandbox.setup` ahead of every probe
     run to buy a recap line the occurrence mode gets for free.

     **Go is not the only outlier, and the other runners were measured rather than remembered.**
     `cr-research` ran all of them against one fixture — 4 passing, 1 failing, 1 skipped, so the truth
     to hit is executed 5 and failed 1 — through a harness replicating `counts.go`'s `sum()` exactly.
     Versions, because a recap format is a version-dependent claim: pytest 9.1.1, jest 30.5.2,
     vitest 5.0.1, cargo 1.98.1, go1.27.1, darwin/arm64.

     | runner | `tests.cmd` | `count_pattern` | `failed_pattern` | reads |
     |---|---|---|---|---|
     | jest | `jest --json` | `"num(?:Passed\|Failed)Tests":(\d+)` | `"numFailedTests":(\d+)` | 5 / 1, zero case 0 / 0 |
     | vitest | `vitest run --reporter=json` | same fields | same | same |
     | pytest | `pytest -q` | `(\d+) (?:passed\|failed)\b` | `(\d+) failed\b` | 5 / 1, zero case **undetermined** |
     | cargo | `cargo test` | `(\d+) (?:passed\|failed)\b` | `(\d+) failed\b` | 2 / 1 |
     | go | `go test -v -count=1` | none exists | — | undetermined |

     Two things follow. **pytest is a second outlier, narrower than Go**: a filter matching nothing
     prints `6 deselected` and no `passed`/`failed` at all, so the pattern never matches, `Counts()`
     returns nil, and the ladder reaches `inconclusive` where Go reaches `no-tests-selected` — the
     weaker answer for the same situation. `pytest -v` with occurrence counting at
     `^[^ ]+::[^ ]+ (PASSED|FAILED|SKIPPED)` gives 6 / 1 normally and 0 on the zero case; the anchor
     excludes the short-summary line `FAILED test_sample.py::test_e`, which otherwise doubles the
     failed count. So the occurrence mode is not a Go special case — it is what a runner needs when
     the recap is absent (Go) or vanishes on the zero case (pytest). And **jest has no per-test list
     to count instead**: `jest --verbose` printed no per-test lines at all on 30.5.2, passing or
     failing, so its JSON reporter is the only route rather than a preference. vitest's
     `--reporter=verbose` does print them.

     The jest/vitest key spelling is load-bearing and invisible: `"numFailedTests":` does not match
     inside `"numFailedTestSuites":1` only because the suite key reads `TestSuites` where the test key
     reads `Tests"`. Any profile shipping it wants that comment beside the pattern.
  2. **§2.4.5 pins the shipped set at two**, by name, so a third profile cannot land without a spec
     version. `TestV01ShipsExactlyTheTwoProfilesOf245` and `TestBothShippedProfilesRequireNoSandboxPath`
     both fail on a third, correctly. This is why the Go profile belongs in v0.5's spec rather than in
     a patch release minted for it — and while that clause is being opened, **pin the property rather
     than the count**: every shipped profile carries a runner whose counts cr can read, a filter flag
     and a probe path template, with the set in a table a later version extends without amending the
     sentence. Then a fourth profile is a commit, which is what this entry wrongly claimed the third
     already was.

  **A constraint on `count_pattern` that belongs beside §5.2.1, not in any one profile.** A single
  capture group summed over non-overlapping matches can read a recap line **only when the words it
  keys on appear nowhere else in the output**. Go's regexp is RE2, so there is no lookbehind to
  exclude a line — `(?<!Suites: )(\d+) passed` does not compile, it is rejected as an invalid named
  capture — and anchoring does not rescue it either, because matches are non-overlapping and the
  anchored prefix is consumed by the first match, so only the first number on the line is ever read.
  Measured through cr's own `sum()` semantics against jest's real recap
  (`Test Suites: 1 failed, 1 total` above `Tests: 1 failed, 1 skipped, 4 passed, 6 total`), truth
  being 5 executed and 1 failed:

  | pattern | reads |
  |---|---|
  | `(\d+) (?:passed\|failed)\b` | ran 6, **failed 2** |
  | `(\d+) (?:passed\|failed\|total)\b` | ran 13, failed 2 |
  | `(?m)^Tests:.*?(\d+) (?:passed\|failed)` | **ran 1** |

  Three plausible patterns, three wrong answers, and the first is the dangerous shape because it
  **inflates the failed count** rather than obviously breaking — a baseline reading two failures where
  there is one still fails §5.2.5's verdict, so nothing is graded on it, but a `failed` probe result
  would be read as the code's. This is why a shipped profile picks a JSON reporter over a prettier
  one, and it is the scope of the occurrence mode: that mode exists for runners whose counts are
  per-line rather than per-recap. Writing the constraint down is cheap and it cost three wrong
  patterns to find.

  Which languages, if the order is ever questioned: AACR-Bench's 50 repositories give comment share
  C++ 508, TypeScript 422, Java 272, Go 245, C 206, Python 151, JavaScript 141, Rust 92, PHP 56,
  C# 52 — Go + TypeScript + Python is 38% of 2145, and Java would take it to 51%. Popularity is the
  weaker criterion, though: what decides is whether the probe machinery can work at all, which is a
  readable count, a filter flag and a placeable probe path. On that test the three already named are
  the right three and Java is fourth. Per-language precision from the same dataset says which will be
  hardest to make trustworthy: C# 0.87, JavaScript 0.79, Java 0.78, Python 0.75, PHP 0.73,
  TypeScript 0.72, Go 0.71, C 0.67, Rust 0.64, C++ 0.60.
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

### 4. The conversation (v0.5)

The re-review half the first spec promised:

- anchor migration across a push, using the context window and content hash v0.1 onward already record;
- recheck and verification: after the author pushes, is each posted concern addressed;
- resolving and withdrawing threads, still behind `--confirm`;
- reading author replies from GitHub instead of storing them by hand with `cr answer`;
- a question closed by an answer is neutral, an open question is not — the rule a convergence criterion
  needs.

**Migration is smaller than it looks, and `cr-research` measured why.** Four findings, the first three
checked against cr's own code here before being written down.

1. **GitHub migrates posted comments itself, and says honestly when it could not.** On
   `deligoez/cr-qa#13`, 14 comments written against `be7e2c7` all carry `commit_id` `49d8bca`; the four
   it could not place carry `line: null` while keeping `originalLine`, `originalStartLine` and
   `diff_hunk`. **cr already reads exactly that** — `internal/gh/threads.go:154` selects
   `isOutdated path line startLine originalLine originalStartLine diffSide`, and `Anchor.Line` is
   documented as zero when the head no longer carries the code while `OriginalLine` survives "so an
   outdated thread still names a place". So for a *posted* comment the work is not re-finding it. It is
   deciding what an outdated thread means — withdraw, re-ask, or carry forward — which is a policy
   question, not an algorithm.
2. **REST's `position` is a trap cr avoids by construction, and the clause should say so on purpose.**
   Measured on the same pull request: three comments originally at lines 9, 44 and 44 all report
   `position: 1` after the head moved, while `line` is null for all three. A reader trusting `position`
   would place three different comments on one line and believe it had succeeded. There is no defect —
   the GraphQL selection does not ask for `position` at all, verified here — but one sentence in §5
   naming `line`/`originalLine` as the read and `position` as never-read keeps somebody from
   "simplifying" it to a REST call later.
3. **What stays cr's own** is what nothing external tracks: records not yet posted, waivers, and any
   record being re-graded. Waivers matter most because their key moves with the code —
   `finding/waiver.go:91` keys on `ContextKeyHash(ContextBefore, anchored, ContextAfter)`, so the
   window moving changes the key.
4. **The migration must not need the old tree, and that is measured rather than assumed.** After a
   force-push the superseded commit is absent from a fresh clone and cannot be fetched by sha
   (`fatal: couldn't find remote ref c6878b3b`). The instrument check is worth repeating: a first run
   reported the old commit present, an artefact of `git clone /local/path` hardlinking the object store
   and carrying unreachable objects across; cloning over `file://` forces the pack protocol and gives
   the real answer. GitHub may itself retain a force-pushed commit, but that is a host's retention
   policy and not a git guarantee, and it was not tested. So the migration reads **only the new tree
   plus what cr stored** — `path`, `side`, `start_line`, `line`, `content_hash`, `context_before`,
   `context_after`. That is what §9.2.3 recorded the window for.

**The shape, and the rule that matters more than the shape.** Alibaba's `internal/diff/resolver.go`
generalises cleanly: normalise each line (trim, strip a leading `+`/`-`), sliding-window match the
stored text over the hunk's new side, then the old side, then the file. Its decision rule is worth
copying verbatim — **a unique hit relocates, and zero hits and multiple hits both decline**, on the
reasoning that the same boilerplate legitimately appears in several files and guessing trades one wrong
location for another. cr can do better in one way, because it stored more: the anchored lines are a
narrow key and the anchored lines plus the context window are a wider one, so try narrow, widen on
ambiguity, decline if still ambiguous.

**The failure to design against is a migration that looks like it worked.** Declining is cheap — the
record becomes a question or is withdrawn, and §7.2 still requires a human's edit before anything
posts. Placing a comment on a line that merely resembles the old one is a wrong assertion with a
confident location attached, which is the most expensive thing cr can produce. So: **a migration that
cannot be made unique is not a migration.** It withdraws or re-asks; it never places. That is §6.2's
own asymmetry applied to a location instead of to a claim.

Not researched, and not to be assumed: how Gerrit or GitLab handle any of this. All of the above is
GitHub, git, and Alibaba's reader.

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
- **Does a `probed` question ever become a finding?** M4 part B produced seven records graded `probed`
  by experiment, and all seven stayed `kind: question`, because §5.7.4 moves the grade and §7.2 leaves
  the kind to the human's edit in the draft. So the trust-economy number M4 pre-registered — false
  assertions among the probed records — came back 0 over an empty set: nothing in that set asserts.
  §5.7 today buys a question the human *may* promote (§6.3.3 admits `question` → `finding` only on
  `probed` or `cited`) with the probe's evidence region rendered under it. Whether a human promotes
  one, and whether the promoted one is true, cannot be measured without a human reading a draft, which
  is the one part of the loop no batch can stand in for.
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
