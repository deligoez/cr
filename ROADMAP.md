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
| v0.7.0 | A push carries the draft: `cr brief` migrates every unsent record to the new head, searching its own file and every file the new diff touches, and carries each one that places inside a new unit — same id, new line and round, back to `draft`, with the body the reviewer edited — while one whose code is gone goes stale. A carried record re-reads its evidence (its probe cleared, a citation whose line changed unstamped), is raised once per pull request, and absorbs its own re-raise as a duplicate. `cr recheck` reports the round's migrations, or previews them while the head has moved |
| v0.6.2 | The rest of the v0.6.0 QA pass: `cr test` prints the counts and verdict it stored, a brief discloses a head GitHub still reports after a push against the remote's own `refs/pull/<n>/head`, and three refusals that named no step now name one — a base with no shared history, the intent settings the GitHub tracker leaves inert, and a pull request closing two issues. `cr post --reconcile` was measured against a real lost answer for the first time |
| v0.6.1 | Two defects the v0.6.0 QA pass found on real pull requests: `cr recheck` migrated an anchor against the head the record was made at rather than the one the push moved to, and so reported code that had moved as unmoved; and a gap probe whose template places its test at the repository root ran on no directory and came back `inconclusive` |
| v0.6.0 | A `go` profile, with an occurrence count mode for a runner that prints one line per test and a zero read only from a clean exit, and §2.4.5 pinning what a shipped profile carries rather than how many; GitHub Issues as a tracker, keyed `owner.repo#n` from the pull request's closing link and read through cr's own gh door; `cr withdraw … wrong\|not-here` writing the waiver its disposition scopes and a `withdrawn-*` outcome that replaces the posting's `kept`, with the waiver key's hash stamped at record time; `cr status` listing posted concerns from every round; and `cr resolve`/`cr withdraw --confirm` fixed, which GitHub had refused since v0.5.0 |
| v0.5.1 | The case v0.5 exists for: after a push opened a new round, `cr recheck`, `cr verify`, `cr resolve` and `cr withdraw` could not see a posted record, because it stays in the round that posted it and they read only the current one. They now find it by id where it lives and change its state there. `cr rules suggest` also harvests a comment after `cr verify` or `cr withdraw` has moved its record |
| v0.5.0 | The loop closes: `posted` stops being terminal and gains four exits, `cr recheck` reports thread state, replies and the anchors cr migrated, `cr verify` records the agent's judgement about each posted concern, and `cr resolve` and `cr withdraw` close a thread behind `--confirm`. cr reaches no verdict of its own: §9.5.6 keeps "was this addressed?" a judgement |
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

*Empty.*

- ~~**§9.4 migrates exactly the records §9.3.4 stales. Two readings; the user decides.**~~ *Shipped in
  v0.7.0 as reading (i), `spec/0.7.0.md` §9.3.4 and §9.4.5–§9.4.8, and measured on `deligoez/cr-qa#23`:
  a push that moved three queued records two lines down carried all three, and the edited body
  survived into the next round's draft.* Found by the
  v0.6.0 QA pass, 2026-09-22, and read against `spec/0.6.0.md` with cr-research. §9.4.1 migrates
  every record that is not terminal and never a posted one, so its domain is `draft` and `queued`.
  §9.3.2 refuses every per-PR write while the head has moved, which is the only time a migration
  matters, and §9.3.4 moves every `draft` and `queued` record to `stale` on the brief that follows —
  and no row of §9.1's table leaves `stale`. So a migration can persist nothing before that brief and
  has nothing to act on after it. Two signatures follow: `state.FileMigrations` names `cr recheck`
  as its owner in §2.3's table and has no caller under `internal/cli`, and §9.4.5's "carried to a
  later round where the code may have settled" and §9.6's "the only way to close an unplaceable
  record" both presume a carry §9.3.4 does not give. **No wrong assertion can reach an author**: a
  migrated or declined `draft`/`queued` record is staled before it can be drafted or posted, which is
  why v0.6.1 shipped without it. The readings: **(i)** migration is load-bearing — §9.3.4 stales only
  records that did not place and carries placed ones with their migrated anchors, so drafted comments
  survive an author's small push, §9.4.5's MUST NOT becomes the line between carried and staled, and
  the brief that carries them writes `migrations.ndjson`; **(ii)** migration is a preview — `cr
  recheck` reports where a record would land, the §2.3 row and §9.4.5 are struck, and §9.6's
  sentence is reworded. cr-research leans to (i), the only reading under which §9.4.5, §9.6 and
  v0.6.1's migration fix all mean something. Either changes what a round holds, so it is the user's.

  **Decided 2026-09-22: reading (i), as v0.7.0.** The user chose it on the recommendation above.
  v0.7.0's spec settles §9.3.4 and §9.4 together — which records a push carries and which it stales,
  who writes `migrations.ndjson`, and what §9.4.5's refusal guards — and nothing else rides with it.

- ~~**A run that did not compile is disclosed as `no-tests-selected`.**~~ *Closed in `spec/0.5.0.md`:
  §5.3.4's and §5.4.3's zero-count rungs now carry a `reason` naming the exit code when it is not 0,
  and §5.5's table gains the `reason` row the implementation had been writing without one.*
  **A run that did not compile is disclosed as `no-tests-selected`, the same word as a filter that
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
- ~~**A withdrawal collects the best false-positive signal cr has and throws it away.**~~ *Shipped in
  v0.6.0 as the three parts below, and discharged against its own criterion: a confirmed
  `withdraw … wrong` writes `withdrawn-wrong` over the posting's `kept`, which §7.3.4 counts against
  the class (`TestAConfirmedWithdrawalWaivesTheConcernAndReplacesItsKeptOutcome`).*
  **A withdrawal collects the best false-positive signal cr has and throws it away.** §9.6 moves a
  retracted record to `withdrawn` and resolves its thread, and that is all. A concern the author saw
  and the reviewer then took back is the strongest evidence a class is wrong that cr can ever get —
  stronger than a `wrong` in triage, because the author read it — and §7.3 exists to act on exactly
  that. It sits here rather than under Defects on purpose: nothing in `spec/0.5.0.md` promises the
  waiver, so cr does what it says and this is a gap, not a broken contract.
  v0.5's draft spec carried the clause and it was dropped before the tag, for a reason worth keeping:
  `finding.WaiverFor` keys a waiver from the head's trees, which would have made `cr withdraw` a
  head-reading command and coupled a GitHub write to a git read. Keying from the stored
  `content_hash` instead is **not** a way out: it changes the pre-image, so every stored waiver and
  posted-index entry would stop matching, which is v0.1's break a second time
  (`contextkey_test.go:99` measured the narrower form). The design that survived review with
  cr-research, 2026-09-21, has three parts. First, `cr withdraw <pr> <id> wrong|not-here`, positional
  like `cr triage` and refused without it, because cr cannot establish which it was (P2), and
  `WaiverScope` already forbids deciding scope for the human. Second, two outcome actions,
  `withdrawn-wrong` and `withdrawn-not-here`, written under the posting round's triage key so they
  replace its `kept` and one raise keeps one outcome; `triageKey` already makes the later outcome
  win. Third, the waiver key's hash stamped at `cr record` time, where `StampAnchor` holds the
  anchored lines, so the pre-image is unchanged and no later command reads a tree. That third part
  matters because a withdrawal can follow a force-push, and cr-research measured the superseded
  commit absent from a fresh clone. Since v0.5.1 a posted record keeps its round, so the posting
  round is the record's own `round`. Discharged when a withdrawal counts against its class in
  `cr stats`.
- **No per-language correctness corpus.** This is the half split out of the item above, and it has a
  source: Alibaba's `rule_docs` is a reasonable first intake for per-language *defect and security*
  rules, with attribution, as a corpus intake and not as an adoption of their selector. It pairs with
  profiles rather than with house style. Discharged when a shipped profile other than `laravel-pest`
  carries a corpus.
- **Two profiles is not a tool.** *Half discharged in v0.6.0: `go` ships, with the occurrence mode
  and `tests.paths_default` below, measured against captured go1.27.1 output, and §2.4.5 now pins the
  property rather than the count, so the next profile is a commit. TypeScript and Python are what is
  left; pytest needs the occurrence mode too, jest needs `--json` with the sum.* Before it, only
  `laravel-pest` and `generic` shipped, so on any other stack the
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
- **Every non-Jira team writes configuration before its first review.** *Discharged for GitHub Issues
  in v0.6.0: `intent.tracker: github` reads the issue through the gh cr already requires, keyed from
  the pull request's closing link, with no command to write.* `intent.cmd` defaults to a `jira`
  binary (§3.1.2), and the intent axis is the one P1 calls the authority, so a team on GitHub Issues or
  Linear cannot run a first round as shipped: it configures a tracker command or passes `--intent-file` by
  hand. Ship the two commands. This is a tracker, not a profile, and it is listed on its own because the
  file's opening rule forbids riders.
- **A driver for the mechanical steps.** A round is ten commands of which three need the agent's judgement.
  Measured: the orchestration was written three separate times for the three measurement passes, and every
  time the plumbing — which prompts to run, where their output goes, collecting the cells — was the
  operator's to build rather than cr's. §7.3's statistics, the probe cap's report, and the round's
  completeness all exist; what is missing is the one command that walks the mechanical half.

*Five small items the v0.6.0 QA pass found against `deligoez/cr-qa` and `cr-qa-go`, 2026-09-22, in
the order cr-research ranked them. None let a wrong assertion reach an author, and* **all five
shipped in v0.6.2**, *each inside `spec/0.6.0.md` with no normative change. The rest of that
conversation was v0.7.0, §0's reconciliation, which shipped as reading (i). The driver stayed out
of it.*

- ~~**`cr test --json` carries no counts.**~~ v0.6.0's occurrence mode is checked by running
  `cr test`, and the counts reached only `runs.ndjson` under `~/.cr`, so the feature could not be
  checked from the command that runs it. The output now carries §5.2.4's `tests_run`,
  `tests_failed` and §5.2.5's `passed`, the counts left out rather than zeroed when the profile's
  patterns derive neither.
- ~~**A pull request whose head shares no history with its base fails on a raw error.**~~ `git
  merge-base` answers that with exit 1 and an empty stderr, so cr surfaced `exit status 1` and named
  no step. It is now `git.NoMergeBaseError`, and the hint says the head was not branched from the
  base.
- ~~**`cr config --resolved` shows the Jira key pattern beside `intent.tracker: github`.**~~ The row
  stays, because §2.7 annotates every setting, and it now says it is not in force and why — as does
  `intent.cmd`, which that tracker also never starts.
- ~~**A pull request that closes two issues needs `--issue` on every brief.**~~ The refusal was
  right and its hint named no candidate: `AmbiguousIssueError` carried a hint that did, and `hintFor`
  never read it. It is read now, and names every candidate as the flag to pass.
- ~~**GitHub reports the old head for a few seconds after a push.**~~ `cr brief` now reads the
  remote's `refs/pull/<n>/head` beside gh's answer and discloses both commits when they differ. A
  disclosure and not a refusal, which would be an exit-4 condition §9.3.2 does not have.

### 2. Measure what is still unmeasured

**After v0.6.0 the next step is a decision, not work.** The driver above, real use below, and the
honest AACR-Bench instrument are one activity — running rounds on real pull requests — and the
driver's shape should come from those rounds rather than from a guess. What it waits on is the
user's: which pull requests, under whose name the output is posted, and the budget for a benchmark
slice. Agreed with cr-research, 2026-09-22.

- **`TestAStoppedProbeLeavesNoRunnerAndTheNextRunStartsClean/terminated` failed once, under the
  full `-race` run, and has not been reproduced.** 2026-09-22, during v0.6.2's gate: `cr probe run`
  reached the state on disk 591ms after it started, and process group 81149 was still alive 15
  seconds after the signal, so the test reported that cr had exited leaving its runner running. The
  load at the moment of the failure was **not recorded**; the box read 3.07/2.58/2.27 afterwards,
  which is the wrong figure for the failing run and is written here only so nobody mistakes it for
  one. Two re-runs of that test alone and a second full `-race` run were clean. It is entered here
  rather than dismissed because the path it exercises is invariant 6 — a probe reverts even when the
  run fails, times out or panics. Raised by cr-research, 2026-09-22, who also read the evidence:
  **a wait shaped wrong fails by checking early, and this one did not.** `awaitRunnerGroupGone`
  polls every 20ms against a 15-second deadline, so the group was still there after fifteen seconds
  of asking. That leaves two readings of the mechanism and one of the instrument. The mechanism:
  the signal did not land, or it landed on a group the runner's descendants had already left — the
  M-1.6 tail v0.3.2 shipped the survivor disclosure for. The instrument: `runnerGroupAlive` asks
  `kill(-group, 0)`, which succeeds for a zombie as well as for a live process, and the helper's own
  comment says a loaded machine may reap late — so a killed runner nobody had reaped yet reads
  exactly like one that never died. **When it next fires, capture the group's process tree at the
  moment of the check** — `ps -o pid,pgid,ppid,stat,comm -g <group>` — **and the fifteen-minute
  load.** Whether the survivors still share the group, have re-parented, or sit in `Z` decides
  between all three in one line. If it never fires again, that is this entry's answer.

M1 and M3 answered two of this item's three slices. What is left has no data at all:

- **Real use.** Review real pull requests with cr, posting only what the reviewer would have posted anyway,
  and record per round: comments kept, softened, deleted, marked `wrong`; author replies; wall clock and
  token cost. §7.3's triage statistics exist for exactly this and hold one round's data from one field
  trial. It is the same question the volume question below asks, from the other end. *Unblocked
  2026-09-23: the user chose the pull requests awaiting their review, output under their own name, and
  nothing posted without their explicit approval of the exact payload.*

  **Round 1, tarfin-labs/backend#6292 (WB-3242, +341/−19, 15 files), 2026-09-23.** 7 claims from the
  Jira issue, 20 units, 4 roles, 80 cells complete, every claim mapped. 9 records, **all questions**
  (5 `cited`, 4 `argued`), 0 findings, no sandbox or probe (the suite would run against the
  reviewer's local database, so experiments wait for their say). Two questions were checked by hand
  against the code before the draft reached the reviewer and held: a car-sales retailer is never
  *created* as `CAR_SALES` anywhere in the application, so the PR's `creating()` default may never
  fire outside tests; and the guard's wiring into the machine is exercised on its passing side only.
  **The triage is not in yet** — the reviewer has not read the draft — so kept / softened / deleted
  counts are owed here when they are. What the round measured about cr itself:
  - **Speed was the blocker, and is fixed in v0.7.1.** `cr status` took 57–82 s and `cr record` over
    five minutes, because §4.3.1's symbol index started one `git cat-file blob` per source file of the
    head: 8,895 of `cr status`'s 8,913 git processes. One `cat-file --batch` brings `cr status` to
    3.8 s and `cr brief` to 5.8 s with byte-identical output.
  - **The orchestration was all by hand**, and it is the driver item's first measurement: extract the
    claims, record them, emit the intent pass, run a role, record its cells and the mapping, emit the
    other three axes, run three roles, record their cells, merge, record, draft — about fifteen
    commands and four agent runs, the only judgement being the claims, the roles' own and the draft.
    One role agent per axis over all 20 prompts worked: the intent pass took 4 minutes, convention and
    test-adequacy 32 each, and correctness stalled twice on the harness's stream watchdog, was resumed,
    and finished its cells and records before stalling a second time.
  - **One concern arrived twice from two roles at two lines** — the `,00` in the rendered amount, from
    intent-coverage on the translation line and correctness on the code line. `cr merge` listed them as
    a possible duplicate (shared citation) and left both, which is §6.4.1's key doing what it says;
    whether the draft should fold such pairs is a question for more rounds, not this one.
  - **The draft's bodies are English and the team writes Turkish.** §6.1.1 stores findings in English
    and §8.1 renders reader-facing prose at draft time, but only the labels are rendered; the bodies
    are the roles' English. Every kept comment will be rewritten by hand before posting — to be
    counted once the triage is in.
- **AACR-Bench as an instrument.** *This entry said its 640 negatives were a binary test of the grade
  ladder "with no matcher needed", and that was wrong; cr-research, whose sentence it was, corrected
  it on 2026-09-22.* cr grades its own records from their anchor, citations, probe and containment
  (§6.2.1); a benchmark comment carries none of them, so handed to the ladder it grades `argued`,
  positives and negatives alike, and the "test" reports that both were forced to questions — zero
  separation by construction. The honest instrument runs cr's roles over the benchmark's pull
  requests and scores cr's own records, which needs a matcher (unpublished) or expert judgement,
  runnable repositories, and roles per pull request: measurement 1 was 123 prompts and $89 for one
  pull request, so a 20-PR Go slice is of the order of $1,500. That is a budget decision, and it
  needed the Go profile, which v0.6.0 ships.
- ~~**`cr post --reconcile` has never run against a real unknown outcome.**~~ *Measured 2026-09-22
  on `deligoez/cr-qa#21`, and it works.* A `gh` shim on `PATH` forwarded the review POST to the real
  gh and then reported the answer lost (`net/http: TLS handshake timeout`), which is §8.4's unknown
  outcome from cr's side while GitHub held review 5282249926. `cr post --confirm` exited 4 and set
  `post_unresolved`; `cr post --reconcile`, with the real gh, adopted that review by §8.4.3's payload
  hash `807bf9e0aa6cd139`, moved `f1` to `posted` with its thread, and cleared `post_unresolved`; a
  second `cr post` was then refused with exit 4 as an already-posted round. The whole path had until
  then been exercised only through the unit suite's shim.

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

#### The shape v0.5 takes — implemented in `spec/0.5.0.md`, §9.4 through §9.6

*Everything below is built and under test. Two things the design met on the way are worth keeping
here, because neither was visible from the spec.*

**Two sets came apart that had been one by coincidence.** §9.3.4 stales `draft` and `queued`, and
the implementation asked for "the open states" — which was the same set until `posted` joined it, and
then meant cr abandoned every posted concern the moment the author pushed. §10.2.4 blocks completeness
the same way, and reading the open set there meant no round could ever be complete once it had posted
anything. `finding.UnsentStates()` is the separation. Both were caught by existing tests rather than
by reading, which is the argument for having them.

**A withdrawal cannot carry prose, and the fence found that rather than the design.**
`TestNoCommandAcceptsABodyArgumentOrABodyField` refused a `--body-file` on `cr withdraw` as a second
body channel beside the draft's, which §8.1.2 allows only one of. The retraction is now a resolution
and a record; the reviewer writes the explanation themselves, as §7.2.3 already has them do.

**`posted` stops being terminal, and that is the whole change.** §9.1 makes it terminal today because
v0.4 ends at posting; v0.5 gives it four exits and adds the states they lead to.

| From | To | Meaning |
|---|---|---|
| `posted` | `answered` | The author's reply settles a posted question |
| `posted` | `addressed` | The concern is gone at the new head |
| `posted` | `withdrawn` | The reviewer retracts it |
| `posted` | `posted` | It still stands at the new head, carried into the round |

`answered`, `addressed` and `withdrawn` are terminal. A record that can be neither carried nor placed
is the case the migration rule above governs: it does not silently move.

**P5 is the constraint the command surface has to satisfy, and it is easy to break here.** "Is this
concern addressed?" is a judgement, so cr may not make it — the same reason §3.6.2 already refuses to
let `cr answer` change a record's state, saying the judgement is v0.5's. The split that keeps P5:

| Command | What it does | Who judges |
|---|---|---|
| `cr recheck <pr>` | Read-only. Per posted record: what GitHub's own migration says (`line`, `isOutdated`, `originalLine`), what cr's migration says for what it owns (placed / declined-ambiguous / declined-absent), replies since posting, and whether a probe that supported it still reproduces | cr reports, nobody judges |
| `cr verify <pr> <record-id> addressed\|standing` | Records the judgement with its evidence | the agent |
| `cr resolve <pr> <record-id> --confirm` | Resolves the GitHub thread | the human confirms |
| `cr withdraw <pr> <record-id> --confirm` | Posts the retraction and resolves | the human confirms |

So cr executes and records; the agent judges; nothing reaches GitHub without `--confirm`, which is P4
unchanged. `cr recheck` making the call itself would be the P5 violation, and it is the tempting design
because the evidence is usually unambiguous — which is exactly when a tool starts forming opinions.

**Two things this settles that are already owed.** `cr answer` stops being the hand-fed channel: §4's
list wants replies read from GitHub, and the ingested reply becomes the evidence `cr verify` cites.
And the convergence rule the first spec asked for gets its footing — a question closed by an answer is
neutral, an open question is not — because `answered` is now a state that exists to be counted.

**What v0.5 does not touch.** Approving or requesting changes stays out (§1.3.3, and
`internal/post/review.go`'s unexported `event` constant is the mechanism). The Go profile and the
count-occurrence mode ride a later version: §2.4.5 and §5.2.1 both have to open for them, but neither
is part of closing the loop, and v0.4's own lesson was that a release should be one topic.

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
