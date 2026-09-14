# cr — Code Review

Code review lifecycle manager for AI coding agents. Go CLI tool.

`VISION.md` explains why this exists and what it bets on. `spec/0.2.0.md` is the
normative v0.2 contract. This file holds the working conventions and the rules
that are easy to violate by accident.

## Install

```bash
brew tap deligoez/tap && brew install cr          # Homebrew
go install github.com/deligoez/cr/cmd/cr@latest   # or Go
```

## Key concept: trust economy

cr's output goes to a colleague. A wrong comment costs trust, and trust is spent
once. Every design decision optimises for the reviewer's standing with the
author, not for how many defects the tool can name.

That inverts the usual trade-off. Recall is cheap and nearly worthless here; a
false assertion is expensive and permanent. So cr never suppresses uncertainty,
it **re-shapes** it — a weak finding becomes a question, and a strong one carries
an experiment.

**Always evaluate a change through this lens: does it raise the cost of being
wrong, or lower it?**

## Foundational principles

| # | Principle | Definition |
|---|-----------|------------|
| P1 | Intent is the authority | The tracker issue is the specification; coverage is measured against it in both directions |
| P2 | Uncertainty asks | Anything cr cannot establish becomes a question, never an assertion |
| P3 | Evidence sets the register | `probed` and `cited` may assert; `argued` may only ask |
| P4 | The human is the author of record | cr drafts, the human edits, nothing posts without `--confirm` |
| P5 | The tool forms no opinion | cr fetches, executes, validates, records; the agent judges |
| P6 | Coverage is proven | Every unit times every active role is a filled cell, and skipped axes are declared |

## Invariants

These are not preferences. A change that breaks one is wrong regardless of how
convenient it is.

1. **cr never calls a language model.** No API client, no model name, no
   inference. If a feature needs cr to form a judgement, the design is wrong.
2. **cr never writes inside the repository under review.** All state lives under
   `~/.cr/`. The user's worktree, index, and branch are read-only to cr. The one
   write spec §5.1.1 permits is the probe sandbox's registration under the
   repository's `.git/worktrees/`, which `cr sandbox destroy` removes.
3. **The confirmation gate cannot be configured away.** There is no setting, env
   var, or profile field that makes a network write implicit.
4. **A finding graded `argued` cannot be posted as an assertion.** The forcing
   happens in cr, not in the prompt, so no agent can talk its way past it.
5. **Exit codes never get renumbered.** `internal/cli/exit.go` pins them and a
   guard test asserts the values.
6. **Probes revert.** A mutation is undone even when the run fails, times out, or
   panics.

## A prescribing sentence carries its measurement

This file's function is to tell the next agent what to do, so a wrong fact here
is not read and discounted — it is implemented. A wrong fact misinforms; a wrong
fact phrased as a rule instructs.

That happened once already: `--test-cpu` was pinned here as the fix that makes
two runs comparable, on reasoning rather than measurement, and it silently turned
every run into a 100% pass. It went to three sibling repositories before anyone
ran it. So a rule that prescribes carries the numbers that produced it, inline,
close enough that a reader can check the ground instead of inheriting the
conclusion — and close enough that the next person to doubt it knows exactly
which experiment to repeat.

Two related habits, both learned the same way. **State the signature, not the
mechanism**: what you can observe from outside survives a change of machine, tool
version and load; a claim about what happens inside is a hypothesis, and
reporting it as fact is what makes a correct finding useless elsewhere. And
**check the instrument** — `pgrep -f` matching its own shell, a `grep -v _test`
filter eating the line it was looking for, a shell that word-splits one binary's
arguments and not another's, a cached `go test` answering a dogfood question
about code that had changed (use `-count=1`), and a one-minute load average
reading 38 on ten cores while `ps` showed two workers under a single core — the
fifteen-minute figure, 9.08, was the honest one. Each produced a confident wrong
answer here, and none of them errored.

## Quick reference

```bash
# Build
go build ./cmd/cr

# Test
go test ./...

# Lint
golangci-lint run

# Quality gate (run after every task; tp runs it at `tp done`)
go test -race ./... && golangci-lint run && ./scripts/deadcode.sh

# Stripped binary
go build -ldflags="-s -w" -o cr ./cmd/cr
```

`golangci-lint` v2 only runs formatters that a `formatters:` block enables, so
`.golangci.yml` enables `gofmt` explicitly. Without it a gofmt-dirty file passes
the gate silently.

**The linter at the gate is pinned, and the pin has three conditions, not one.**
It must be a fixed version, it must be the version the gate was measured
against locally, and it must be compatible with the Go version CI resolves. All
three, or the gate becomes a function of the tool rather than of the code. The
third is the one that bites silently: `go-version: stable` moved to Go 1.27, and
`golangci-lint` v2.12.2 — the version measured here — panics inside staticcheck
while lowering Go 1.27's own `internal/poll` to IR, reporting `package "poll"`
as if the fault were in this repository. v2.13.0 added Go 1.27 support, so both
workflows pin v2.13.2. **"It passes locally" is a version-dependent claim for a
static analyser** in a way it is not for `go test` or `-race`: what the analyser
sees depends on the standard library, and that depends on the toolchain. Verify
a new pin locally before writing it into a workflow.

The formatter is part of that claim. Measured at the v0.1.0 tag, 2026-09-14: the
local gate ran Go 1.26.5 and passed, while both workflows resolved `stable` to
Go 1.27.1, whose gofmt indents a multi-value `return` of composite literals one
tab less, and failed the tag's release on `internal/post/review_test.go:52`.
Before tagging, run the gofmt of the Go version CI resolves, for example
`$(GOTOOLCHAIN=go1.27.1 go env GOROOT)/bin/gofmt -l internal cmd`; the first
run downloads that toolchain.

A fourth condition follows from the third: **a pin is the version that worked on
a date, so it carries one, and the date is a test rather than a comment.** The
failure mode is not the pin going stale — it is nobody noticing: v2.13.0 shipped
Go 1.27 support twelve days before the pin here was still v2.12.2, and a comment
saying when to revisit would have said nothing. `TestEveryGateToolIsPinned`
refuses an `@latest` in either workflow, and `TestThePinsHaveBeenReviewedRecently`
fails once `pinReview` passes. When it fails, install the current tools, run the
gate, pin what you measured, and then move the date — never the date alone.
Measured on a shared machine: `type -a golangci-lint` found two binaries with
`~/go/bin` shadowing Homebrew's, so a version number alone does not identify what
ran. Record **which binary** (`type -a` plus `go version -m`) beside a number
that matters.

`golangci-lint`'s `unused` skips exported identifiers by design, so an exported
function nothing calls passes it. `scripts/deadcode.sh` closes that: it fails
when a function is reachable from no main package **and** no test. The gate
string is pinned by `TestQualityGateRunsEveryStep`, because a step that can be
dropped without a test noticing is the blindness the step was added to close.

## Tooling beyond the gate

Review reads what is written; these read what is reachable. Each is cheap, and
three of them found defects in tp that a seventeen-round review and an
eight-round audit had passed over.

| Tool | When | Why |
|------|------|-----|
| `deadcode -test ./...` | In the gate | Fails on code nothing reaches at all |
| `deadcode ./...` | End of each implementation phase, **diffed against the previous run** | Reports test-only code; never in the gate, where it would fail on work in progress |
| `go fix ./...` | After a large push, and before a release | Go 1.26's modernizers; applies only fixes valid at the current `go` directive |
| `govulncheck ./...` | Before every release | Reports only vulnerabilities the code actually calls |
| `gremlins unleash ./internal` | Before a release, and whenever a claim is made about test quality | Mutation testing; pass `./internal`, not `./internal/...` |

Two rules that make the phase-boundary run worth doing:

1. **When the task that was meant to wire a function closes, that function must
   stop being test-only.** If it is still on `deadcode ./...`'s list afterwards,
   the wiring did not happen — and the tests will not say so, because they call
   it directly. In tp this exact signal was a real defect: a validator was
   written, tested, and never called, while every auditor prompt promised that
   unknown values are rejected.

   **The converse does not hold: absence from the list is not proof of wiring.**
   Measured 2026-09-13: `finding.CommentCap.Err()` — the one call that makes
   `cr post` refuse a round over `post.max_comments` — had no caller outside
   tests, and `deadcode ./...` did not list it; its task had closed a week
   earlier, tested at the package level. `cr post` sent two comments over a cap
   of one and exited 0. A sibling measured the same signature for
   `post.Review.Payload` a day before. So when a task wires a MUST, prove the
   wiring with a test that drives the command, not with a quiet deadcode run —
   `git grep` the call from non-test code is the cheap cross-check.
2. **A surviving mutant is not a score to drive down.** Classify them: an
   equivalent mutant nothing can observe, an undocumented boundary, or a
   documented contract with no boundary test. The last is always worth acting
   on. **An undocumented boundary is worth a test when the mutant crashes or
   changes what a reader sees** — measured 2026-09-11, four of the six boundary
   survivors from four slices panicked (`makeslice: cap out of range` twice,
   `index out of range`, `integer divide by zero`), each in an input range no
   fixture reached. One more class exists and must not be filed as equivalent:
   **unobservable but not equivalent**, a mutant that differs only on an error
   path nothing can inject. Enter it in `scripts/known-survivors.json` saying
   exactly that. Say which ones are being left and why. `gremlins` is load-sensitive — a
   run full of `TIMED OUT` is not a result.

   Two habits that classification paid for, both measured 2026-09-12.
   **A comment claiming a mutant is unobservable is a claim to check, not a
   finding to inherit**: five annotations in this tree said a sum sizing a slice
   was only a hint that no test could observe, and four were wrong — `make`
   panics on a negative capacity, and ordinary input reaches one in
   `activation`, `config`, `cli/context` and `sandbox/clean`. And **a test that
   goes red under the mutation is not yet a test that kills it**: a first
   attempt at `cli/record.go:350` made `findings.ndjson` a directory, which
   failed the command under the mutant too, because the next read failed as
   well. Isolating the mutated statement took a read-only PR directory, where
   the write fails and every read still succeeds. Check what the red is proving.
3. **Read `NOT COVERED` and `TIMED OUT` as their own categories, never as
   survivors.** A mutant on a tagless `switch`'s **case expression** is reported
   `NOT COVERED` however well the branch is tested: Go's cover tool starts each
   case block *after* the expression, so the mutant falls outside every covered
   block and gremlins can neither kill it nor call it a survivor. Measured on
   §5.3.4's seven-rung ladder — the coverage profile shows count 1 on every case
   body and the four mutants stay `NOT COVERED` regardless. A `TIMED OUT` is
   often a **detection**: breaking the process-group kill or a `flock` check
   makes the suite hang rather than fail, and the timeout coefficient then buys
   that answer at thirty times the suite's runtime. Both are why a run takes
   15–30 minutes and why the efficacy number understates detection.
4. **Fix `--workers`. Never pass `--test-cpu`: it silently turns every run into a
   100% pass.** With neither set, a run on this ten-core box drove the
   fifteen-minute load average to 66 — more than six times the core count, and a
   saturated box makes slow mutants time out, so an unpinned run reports timeouts
   it caused itself. `--workers 4` fixes that and changes nothing else. But
   `--test-cpu` corrupts the result outright. Measured on `./internal/finding`,
   one package, one tree, one binary:

   | invocation | result |
   |---|---|
   | no flags | 83 killed, **5 lived**, 3 timed out, 94.32% — 1m58s |
   | `--workers 4` | 83 killed, **5 lived**, 3 timed out, 94.32% |
   | `--test-cpu 2` | 91 killed, **0 lived**, 0 timed out, **100%** — 44s |

   The sharpest tell is **`Not covered > 0` beside `Lived: 0`**, because the two
   cannot honestly co-occur: a tree with mutants no test reaches is a tree whose
   tests are not exhaustive, so some mutant should survive. Measured on
   `./internal/finding`, one package, one tree — sound: 83 killed, 5 lived, 7 not
   covered, 3 timed out; broken: 91 killed, 0 lived, 7 not covered, 0 timed out.
   83 + 5 + 3 = 91 exactly. **The broken mode scores every runnable mutant as
   not-survived and leaves NOT COVERED alone**, because coverage is gathered
   before any mutant runs and never touches the failing exec.

   A second corruption reaches the same fake 100% by another road, found in a
   sibling repository: gremlins can measure its baseline from a **cached** `go
   test`, so the baseline reads ~0, the coefficient multiplies it to ~0, and
   every slow mutant times out. There the tell was `Gathering coverage... done
   in 186ms` against a real 73-second suite. cr is not currently hit — three
   runs here gathered in 21.9s, 22.2s and 24.3s against a 26.7s suite — but
   `go clean -testcache` before a run costs nothing and forecloses it.

   **The tell to reach for first is the gathering time itself: it should be
   close to a cold `go test` of what is being mutated.** One number, one
   comparison, and it measures the quantity that gets corrupted rather than a
   consequence of it — so it catches the cached baseline and the `--test-cpu`
   failure alike. Here: 21.9s, 22.2s and 24.3s against a 26.7s suite.

   Wall-clock arithmetic is the obvious second check, and it is **an upper bound
   only**. A run cannot take much less than `runnable × per-mutant-cost ÷
   workers`, but per-mutant cost is neither the tree's suite nor reliably the
   package's: gremlins uses the coverage profile, so a mutant costs the tests
   that *reach* it. Measured in a sibling repository: 74 runnable against a 73s
   package suite could not finish under 22 minutes and finished healthy in
   2m36s, because that package's cost is bimodal — 1.5s of unit tests beside 72s
   of tests that spawn the binary. The arithmetic condemned a sound run.
   It appeared to hold here — 1218 runnable at a 2.1s package average predicts
   10.7 minutes, plus 12 timeouts at 110s, against an observed 17m28s — but that
   agreement is homogeneity, not confirmation: cr's package suites are all within
   a small factor of each other, so "this package's tests" and "the tests that
   reach this mutant" happen to cost about the same. **Read a short run as a
   question, never as a verdict**, and answer it with the gathering time.

   *Which* not-survived status it lands in is machine-dependent, so do not grep
   for one. Here every LIVED and TIMED OUT became KILLED; a sibling repository
   measured the same flag on the same kind of tree and got the opposite — 47
   KILLED and all 5 LIVED became TIMED OUT, 2 mutants killed, 100% efficacy.
   Anyone matching on this repository's mechanism would read that run's 52
   timeouts as a loaded machine. The signature that survives both is the pair of
   counts, not the transition.

   A tree that carries a documented equivalent mutant can check the run against
   it directly. `finding/id.go`'s `n > highest` is one — `>=` assigns the value
   already held — and a `--test-cpu` run reports it KILLED. Applying that mutation by hand
   leaves `go test ./internal/finding` green, so nothing killed it. **Treat a 100%
   efficacy figure as a broken run, not a good one.** The same flag is what made
   `./internal` finish in seven seconds claiming 1270 killed: `go test ./internal`
   fails with "no Go files", every mutant's run exits non-zero, and every one is
   counted killed. Without `--test-cpu`, `./internal` works and takes 15–30 minutes.
   Note gremlins leaves `/var/folders/.../gremlins-*` behind even on a clean
   exit; a CI run needs a cleanup step.
5. **The timeout coefficient is 5, not 30, and that is measured.** The suite runs
   in 26.7s, so the coefficient is the price of one hanging mutant: 13.4 minutes
   at 30, 2.2 at 5. It dominates the run — seven hangs at 30 is 94 minutes of
   waiting across four workers, about three quarters of a 36-minute run.

   Lowering it risks one thing, and it is not losing timeouts: a legitimately
   slow **passing** test could be cut short and counted as detected, which would
   make the suite look stronger than it is — the expensive direction. So the test
   is not "same efficacy" but **did any mutant that LIVED at 30 become TIMED OUT
   at 5**. Measured over both runs' `-o` output, matched on file, line, column
   and mutator:

   | | |
   |---|---|
   | LIVED → TIMED OUT | **0** |
   | survivors present at 30 and absent at 5 | **0** |
   | KILLED → TIMED OUT | 5 — both mean "did not survive" |
   | NOT COVERED → LIVED | 1 — more information, not less |
   | wall clock | 35m59s → **15m32s** |

   Do not read the efficacy percentage across a coefficient change; read the
   survivor set. `-o <file>` writes per-mutant `file_name`/`line`/`column`/`type`/
   `status`, which is what makes that comparison a set difference rather than an
   eyeballing exercise — and what should eventually hold a checked-in list of
   known-equivalent survivors, so "new survivor" is decided by construction.

   **The coefficient is calibrated against the whole tree's suite, so it is too
   tight to read on a small package.** Measured 2026-09-12 on
   `./internal/mapping`, whose cold suite is 0.55s: at `--timeout-coefficient 5`
   the run reported 3 killed and **4 TIMED OUT**; at the default 30, on the same
   tree, 7 killed, 0 lived, 0 timed out. That is exactly the risk this rule
   names — a legitimately slow *passing* test cut short and scored as detected —
   and it appears when the coefficient multiplies a suite far shorter than the
   26.7s the 5 was measured for. Scope a run to one small package and the
   coefficient must move with it, or the timeouts are the instrument's, not the
   code's.
6. **`--dry-run` is a coverage instrument, not a cost estimate, and it is free.**
   It reports RUNNABLE and NOT COVERED without executing a single mutant: 25
   seconds for `./internal` against 15m32s for the real run, and `-o` writes the
   same per-mutant records. So the question "do this package's own tests reach
   this code at all" is answerable before spending anything — and it is a
   question worth asking, because a package whose mutants are all NOT COVERED
   returns silence from a real run, which reads like nothing to report. A sibling
   repository found its newest engine file that way: five mutants, all NOT
   COVERED, thoroughly tested but only through the CLI, so the engine package's
   own suite never reached it.

   Run it before a real run and read the NOT COVERED map. Here the largest
   clusters are `probe/gap.go` at 7 of 7 and `probe/mutation.go` at 7 of 9 —
   both entirely the tagless-`switch` artifact of rule 3, every one on a case
   expression of the ladders, which are among the best-tested code in the tree.
   That is the point: the instrument shows you where to look, and the existing
   rule says which of those places are already explained.
7. **Two gremlins runs must never overlap.** It happened here: a subagent's
   closing run and an orchestrator measurement collided, load hit 17 on ten
   cores, and both numbers became worthless — a saturated box times out mutants
   it would otherwise kill. Check `pgrep -x gremlins` before starting one. Use
   `-x`: a waiter written as `until ! pgrep -qf 'gremlins unleash'` matches its
   own command line and never exits, which left an agent hanging for an hour.
   **`pgrep` itself is not reliable on this machine, and its failure reads as
   "no run".** Measured 2026-09-12 inside a subagent, and on 2026-09-13 from the
   orchestrator too, sandboxed and not: `pgrep` exits 3 with `sysmond service not
   found` / `Cannot get process list`, and `pgrep -x gremlins | wc -l` prints
   `0` — exactly what a quiet box prints. `ps` still answers, so use
   `ps -A -o comm= | awk -F/ '$NF=="gremlins"' | wc -l` (`comm` is a full path,
   hence the last-field match), and check the instrument before trusting a `0`:
   the same line with `logd` in place of `gremlins` printed `1`. The quiet window
   is the orchestrator's to establish and the unit's to be told about — a unit
   must not be the one deciding the box is free.

   **A quiet box is not only one with no second run — memory is part of it, and
   exhausting it forges survivors.** Measured 2026-09-13: a whole-tree run at
   `--workers 4 --timeout-coefficient 5` was killed by the system for low memory
   at about 45 minutes, with swap at 6.0 of 7.2 GB, and wrote no `-o` file,
   because gremlins writes it only at the end. Its log still held 1785 results.
   A per-package rerun agreed on 1780 of them; the other four were all **LIVED in
   the killed run and not survivors at all** — two KILLED, two TIMED OUT —
   clustered in `git` and `gh`, the last packages processed before the kill.
   Applied by hand, `git/patch.go:147`'s mutant fails
   `TestParsePatchRefusesWhatItCannotRead`, and the other two hang the package
   past 120 seconds. So a pressured run does the opposite of `--test-cpu`: it
   invents survivors rather than hiding them, and a classifier would have spent
   units on defects that do not exist. Two things follow. Read swap
   (`sysctl vm.swapusage`) before and during a run, and treat results from the
   minutes before a kill as unmeasured. And scope the run one package per
   `gremlins unleash` with its own `-o` file — measured the same day, the 29
   packages other than `internal/cli` took 8 minutes together, `internal/cli`
   alone took 1h45m at `--workers 2`, swap stayed between 4.3 and 5.7 GB, and a
   kill would lose one package, not the run. The per-package coefficient follows
   rule 5's small-package measurement: default for sub-second packages, 5 for
   `internal/cli`.
8. **`--diff` does not work in v0.6.0.** Measured: `-D main` while on `main`
   should mutate nothing and mutated 116; a `-D HEAD~6` run mutated files absent
   from that diff and took *longer* than the unscoped run. Upstream has three
   open bugs on it (#278, #296, #301). Scope by naming packages instead, which is
   exact rather than approximate: gremlins' default mode runs only the mutated
   package's own tests, so an unchanged package's mutants have an unchanged fate.
   That also means the efficacy figure understates detection — `internal/cli`
   tests that drive `internal/probe` do not count toward `internal/probe`'s
   mutants.
9. **A gremlins run redirected to a file looks stalled and is not. Never judge
   its progress by the log.** Measured here: `wc -l` on the redirected log sat
   at 297 after twenty-five minutes, which reads as ~10 mutants/minute and
   projects a three-hour run; the same run was at 1232 lines a moment later and
   finished all 1365 in 17m28s. gremlins writes its per-mutant lines to a pipe,
   and a pipe is block-buffered where a terminal is line-buffered, so the log is
   a record of what has been *flushed* and never of what has been *done*. An
   agent that extrapolates from it will either abandon a healthy run or burn an
   hour re-planning around a number that was never true.
   **Judge liveness from the process, not the output**: `ps -o etime=,pcpu= -p
   $(pgrep -x gremlins)` gives elapsed time and whether it is still burning CPU,
   and elapsed time against the ~17-minute expectation is the only honest
   progress signal there is. Wait on the process — `until ! pgrep -x gremlins;
   do sleep 30; done` — never on a line count.

**`-race` is in the gate**, added by `state-write-locking`: §2.3.1 puts an
advisory lock on every per-PR write and §2.3.2 makes reads lock-free, so the
suite now spawns goroutines and a race has somewhere to hide. It does **not**
guard the file lock itself — `flock(2)` establishes no happens-before edge the
detector can see, so removing the lock fails the file-state assertions and emits
no `DATA RACE`. Measured, not assumed. A lock defect is caught by asserting on
what reached the file, and a witness added only to make `-race` fire would
report a race even when the lock works. **Do not adopt `apidiff`**: every
package is under `internal/`, so there is no importable API to compare.

## Command surface

Every command below is **implemented**. `spec/0.2.0.md` §11 is the
source of truth; this table is a map, not a promise.

| Command | Purpose |
|---------|---------|
| `cr init [--eject-roles]` | Create the `~/.cr` tree and write the default profiles; `--eject-roles` also writes the built-in roles as editable files |
| `cr brief <pr> [--issue <key>] [--intent-file <path>]` | Orientation payload; opens a new round when the head moved |
| `cr review <pr> [--axis <id>]` | Emit per-role, per-unit prompts and output paths |
| `cr claims record <pr> <file> [--intent-file <path>]` | Store the claims extracted from the issue |
| `cr map record <pr> <file>` | Store the claim-to-unit mapping |
| `cr claims set-aside <pr> <claim-id> --note <id>` | Mark an unimplemented claim out of scope |
| `cr cells record <pr> <file>` | Store the coverage cells the roles filled |
| `cr merge <files...> -o <out> [--repo <r>] --pr <n>` | Merge and deduplicate per-role findings |
| `cr record <pr> <file>` | Record a round's merged findings |
| `cr sandbox create\|destroy <pr>` | Manage the probe worktree |
| `cr test <pr> [--filter]` | Run the suite inside the sandbox |
| `cr probe run <pr> --kind <kind> ...` | Execute and record a mutation or gap probe |
| `cr draft <pr>` | Render the editable draft |
| `cr post <pr> [--confirm] [--reconcile]` | Validate and post the round's one review; settle an unknown outcome |
| `cr answer <pr> <record-id> <text>` | Store the answer to a posted question as a note |
| `cr note <ISSUE-KEY> <text> --pr <n>` / `--remove <id>` | Store or retract an out-of-band fact |
| `cr context <ISSUE-KEY>` | Print accumulated context with provenance |
| `cr rules list\|check\|suggest` | Inspect, run, and harvest project rules |
| `cr waivers list\|remove [--repo <r>] [--pr <n>]` | Inspect and edit waivers in either scope |
| `cr stats [--repo <r>]` | Triage statistics, demotion and volume candidates |
| `cr status <pr>` | Coverage, states, and completeness |
| `cr config [--resolved]` | Effective configuration and its layers |

v0.2 ends at posting. `cr recheck`, `cr verify`, `cr resolve`, and `cr accept`
are **not** v0.2 commands — the re-review half of the loop, anchor migration
included, is v0.3, declared out of scope in `spec/0.2.0.md` §1.3.6. A moved
head makes the round stale (§9.3); `cr brief` opens a new one.

## Project structure

```
cmd/cr/              Main entry point
internal/
  cli/               Cobra commands, exit codes
  activation/        Which axes run this round, and why the others do not (§4.5)
  axis/              The closed set of review axes (§1.5)
  brief/             Orientation payload (§3.7)
  config/            Layered configuration resolution (§2.7)
  coverage/          Coverage cells (§4.5.5)
  draft/             Editable draft rendering and triage ingest (§7.1, §7.2)
  finding/           Review records, grades, anchors (§6)
  gh/                Pull request reads through gh; the single network-write door
  git/               Pinned git runner for every repository read
  glob/              Path globs of cr's data files
  intent/            Issue text through the configured tracker command (§3.1)
  mapping/           Claim-to-unit mapping (§4.1.6)
  note/              Out-of-band context store (§3.6)
  post/              The one review a round posts (§8.3)
  probe/             Baselines, mutation and gap probes (§5)
  profile/           Mechanical per-project configuration (§2.4)
  reinvention/       Candidate pre-existing symbols (§4.3.1)
  render/            Author-facing body language, including the tr labels
  review/            Review fan-out and the items the axes raise (§4.6)
  role/              Role files (§2.5)
  rule/              Rule files (§2.6)
  run/               Test run records (§5.2.4)
  sandbox/           Probe worktree (§5.1)
  state/             State paths, locked writes, the ~/.cr tree (§2.2, §2.3)
  suggestion/        Where a suggestion may land (§8.2)
  symbol/            Head symbol index (§4.3.1)
  testadequacy/      Test-adequacy evidence (§4.4)
  text/              The one text normalisation (§1.4)
  unit/              Review units from the diff's hunks (§3.4)
scripts/             deadcode.sh, speccheck.py, frontier.py, survivors.py, known-survivors.json,
                     mutation-run.sh, mutation-merge.py
spec/
  0.1.0.md           Normative v0.1 contract
  0.2.0.md           Normative v0.2 contract, the current one
  <version>.md       One spec per version
skills/cr/
  SKILL.md           Claude Code skill (ships with the release)
.claude-plugin/
  marketplace.json   Skill distribution manifest
```

Runtime state never lives here. It lives under `~/.cr/`, laid out in
`spec/0.2.0.md` §2.2.

## Sibling codebase: tp

tp lives one directory up at `../tp` (`github.com/deligoez/tp`). It is the
spec-to-task lifecycle manager, and cr borrows several of its proven ideas: the
NDJSON finding contract, a project-owned role corpus, honest convergence
accounting, unit briefs, and "agent plans, tool executes".

1. Read `../tp` freely. Its `internal/engine` is the closest working example of
   most of the machinery cr needs — `cluster.go` for dedup, `reviewstate.go` for
   round bookkeeping, `rolefile.go` for corpus loading, `lock.go` for flock.
2. Share **no code**. No Go module dependency on tp, ever.
3. Copying a well-understood implementation and adapting it is expected.
   Importing it is not.
4. **Never write into `../tp`.** It is a separate repository with its own agent
   working in it; an edit there lands in someone else's uncommitted tree. When
   tp has a bug or a gap that cr exposes, record it in `spec/tp-feedback.md`
   here rather than working around it silently, and let the user carry it over.

## Self-development: cr uses tp

cr is built with tp, the same way tp builds itself.

1. **Write a spec** in `spec/<version>.md`.
2. **Lint it**: `tp lint spec/<version>.md`.
3. **Init + workflow**: `tp init spec/<version>.md --quality-gate "go test ./... && golangci-lint run"`,
   then `tp set --workflow` for convergence counts and round budgets, before the
   review loop so the loop reads them.
4. **Review loop**: `tp review spec/<version>.md` → spawn sub-agents →
   `tp review --merge` → `tp review spec/<version>.md --record merged.ndjson` →
   resolve findings → repeat until `tp review spec/<version>.md --status --check`
   exits 0.
5. **Decompose** into tasks with `source_sections` on every task.
6. **Import**: `tp import <tasks.json>`.
7. **Validate**: `tp validate` for coverage gaps.
8. **Implement each task**, then commit with `hc` and close with
   `tp done <id> "evidence" --commit <sha>`.
9. **Audit loop**: `tp audit spec/<version>.md` → sub-agents →
   `tp audit spec/<version>.md --record results.ndjson` → fix → repeat until
   `tp audit spec/<version>.md --status --check` exits 0.
10. **Release**: tag, push, write release notes.

### Rules

- **A unit orients with `tp brief <id>`, never `tp next --brief`.** `tp next`
  claims — its help says "Get/resume next task", and `--brief` changes what it
  prints, not what it does. A unit told to orient first and then steered to a
  named task leaves the claim it just took stranded, and tp has no transition out
  of `wip` short of finishing: `tp set status=open` refuses the field as managed,
  `tp reopen` refuses a task that is not `done`. That is how `cell-computed-fields`
  sat `wip` from 2026-08-31 through four units. `tp brief` is the read-only one.
- Every task MUST have `source_sections` in canonical form (`"## Heading Text"`);
  `source_lines` is optional precision. A task with neither anchor fails
  validation.
- Every table row and numbered list item in a spec must appear in some task's
  acceptance. Run the backward pass with `tp validate`.
- The quality gate runs automatically at `tp done`. `--skip-gate "why"`, raising
  `review_max_rounds`/`audit_max_rounds`, and `tp import --force` are
  **user-approved decisions, never the agent's own**.
- **Convergence differs by phase.** Spec review iterates until a counted round
  surfaces no critical or high finding; low and medium may be accepted with
  recorded justification. Implementation audit always runs to the full
  clean-round count and is never cut short by a cap — a hit cap means fix and
  continue, with a user-approved raise.
- **An audit round's delta starts at the commit the previous round audited,
  never at the commit that recorded it.** Repairs land between the two. v0.1's
  rounds 6–8 diffed from the record commit, so those repairs were carried
  without being measured; only round 7's fresh look from round 1 caught up. And
  a clean delta round is not a clean tree: round 13 came back 796/796 on a
  delta, round 14's fresh look over every file changed since round 1 found 7
  findings, and a QA run against a real pull request after round 14 found the
  one defect that would have posted a false claim. Alternate delta rounds with
  fresh looks, and run the QA recipe before a release rather than after the
  audit.
- **A spec repair is the minimum normative change.** No rationale sentence, no
  restated motivation, no new concept unless a finding strictly requires one.
  Every explanatory clause in a normative document is itself normative surface —
  a fresh claim that can contradict another section and a fresh cross-reference
  that can go stale. This is measured, not preferred: across v0.1's nine review
  rounds the blocking count sat flat at 34–46 while repairs averaged ~8 lines,
  and fell to 14 the round the rule became ~2 lines. The spec did not change;
  the repair style did. Explanation belongs in the round record and the commit
  message.
- **During implementation, a spec repair is never a unit's own call.** The spec
  is frozen; a task's acceptance criteria carry the findings review left open, and
  implementing the criterion is the unit's job. When the criterion cannot be
  satisfied without the normative text also changing, the unit stops and reports
  it — the orchestrator decides, and anything beyond restating an existing rule in
  the section that already owns it goes to the user. Rounds 10–13 are why: four
  rounds of individually reasonable repairs raised the blocking count from 5 to 9,
  and every one of them looked correct alone.
- **Run `scripts/speccheck.py <spec>` after every spec edit**, before the next
  review round. It resolves every `§X.Y` against real headings and numbered
  items and finds numbered-list breaks — the two failure modes that fixing one
  clause while stranding another produces. It caught defects in most rounds and
  costs nothing.
- **Tell the reviewer roles where the review stands.** Emit the prior rounds'
  severity distribution and say plainly that an empty result is a valid outcome.
  Without it, roles promote ever-narrower items to `high` as the real defects run
  out, and the count stops measuring the spec.
- Commit the `.tp-review/` state directory, the spec, and its `.tasks.json`.
- **Never commit the per-role review or audit working files.** `review-*.ndjson`
  and `merged-*.ndjson` at the repository root are scratch output of one round's
  fan-out; `.gitignore` drops them and they are deleted once the round is
  recorded. The durable record is `.tp-review/`, which keeps its own
  `review-round-<n>.ndjson` and snapshots.
- One task = one commit = one `tp done --commit <sha>`.
- **Every commit goes through the `hc` skill** (hunk-based atomic commits). Never
  raw `git commit`, for task closures, spec progression, docs, or tooling alike.
  cr's effective `commit_strategy` is `hc`, so `tp commit`,
  `tp done --auto-commit`, and a bare `tp done` are rejected; a close needs
  `--commit` or `--covered-by`.
- **No Claude or Anthropic attribution** in any outward-facing artifact: no
  session links, no `Co-Authored-By: Claude`, no generated-with footers, in
  commits, PR bodies, comments, or release notes.
- **Never post to GitHub without explicit approval of the exact content.** Draft
  it, show it, wait. This applies to PR comments, reviews, and review replies
  even when asked to "address" a reviewer's note.
- **English in every committed artifact** — code, comments, specs, docs, commit
  messages, closure reasons, release notes. Author thinking may be in any
  language; nothing in the repository may be. Rendered review comments are
  Turkish, but they live in `~/.cr/` state, never in this repository. The one
  exception is §8.1.4's question labels: the spec requires them **built in** and
  non-configurable, so `internal/render`'s `tr` label strings are Turkish and
  belong in the tree. They are the only ones.

### Dogfooding

- Always run the freshly built binary during development
  (`go build -o /tmp/cr-dev/cr ./cmd/cr`), never a PATH-installed release.
  Rebuild after every implementing commit.
- Once a task adds a command, exercise it immediately against a real pull request
  to surface what unit tests cannot.
- **Use a scratch repository for anything that posts.** Never point a development
  build at a real work pull request with `--confirm`. Create a throwaway repo and
  a throwaway PR, and keep the real ones for deliberate, approved runs.
- cr's own repository is a legitimate dogfooding target once it has pull requests
  of its own.

### Two units at once

Two units may run in parallel on disjoint packages: tp takes a flock on the task
file for `claim`, `next` and `done` (`engine.WithFileLock` in each), so claims
do not race. Three rules make it safe, each learned by breaking it.

- **Never rewrite history while another unit is working.** `hc rewrite`,
  `hc split` on landed commits, rebase, amend: all of them re-hash every commit
  after the rewrite point, including the other unit's. It happened here — a
  unit split its own test commits and three of its sibling's commits came out
  with new SHAs. The sibling had already recorded the new ones in tp, so nothing
  was lost; had it recorded the old ones, `tp done --commit` would name commits
  that no longer exist. Split a commit before the next one lands, or not at all.
- **Commit only your own hunks, chosen by content.** The task file carries the
  other unit's claim hunks beside yours, interleaved. Read `hc diff --json` and
  pick by what each hunk adds, not by position.
- **Never `git checkout --` a file you did not verify is yours alone.** Measured
  2026-09-12: two units edited `internal/review/run.go` in the same minutes, the
  writes raced, and the file was left syntactically broken. The unit that
  reverted it discarded the other's uncommitted conversion of that file — work
  no commit held. `git status` at the start of a task is not evidence: a file
  clean then can carry someone else's work an hour later. Re-read `git status`
  and `hc diff --json` at the moment you revert, and if a hunk is not yours,
  leave the file broken and say so instead. A broken build is recoverable in
  minutes; an uncommitted hour is not.
- **Every codedbpro write carries `if_revision`, and a codedbpro read is not
  proof of what is on disk.** The same file, `internal/review/run.go`, was lost
  twice on 2026-09-12 by two different mechanisms, and this is the second. A
  batch of three writes landed on it while another unit had it open: the one op
  carrying `if_revision` failed closed exactly as designed, and **the two
  without it overwrote a function each**. Afterwards the instrument agreed with
  itself and not with the world — `read` and `diff` both reported the file
  unchanged at a stale revision, with `live:true` and `fresh:true`, while
  `git diff` showed 159 parse errors and `diff {file}` answered
  `changed:false` for a file git called modified. So: pass `if_revision` from
  the read that informed the write, prefer `str_replace` with `expected:1` over
  a line range whenever the anchor text is unique, and when the two disagree
  about a file's contents, **git is the truth**.
- **Never run a full mutation run while either unit is live.** Units do the
  25-second dry run; the full run happens in a quiet window, once.
- **A unit applies a mutation by hand through `go test -overlay`, never on
  disk.** A mutation written into the working tree is also compiled by every
  other unit's `go test ./...` while it stands, and reverting it is the
  `git checkout --` this section already forbids. Measured 2026-09-13: a unit
  classified 28 survivors this way — each mutant a scratchpad copy of the file,
  mapped over the original with `go test -count=1 -overlay <map.json>
  ./internal/<pkg>` — and `git diff` stayed empty throughout while a sibling
  unit worked in the same tree. It checked the instrument before trusting a
  green: the killable `n <= highest` mapped onto `finding/id.go` the same way
  turned `TestAnIDCrDidNotWriteIsNotCounted` red. The overlay map is a JSON
  object `{"Replace": {"<absolute original path>": "<absolute copy path>"}}`.
  **A test that builds the binary is not reached by `-overlay`.** `crBinary`
  runs its own `go build`, which does not inherit `go test`'s flags: measured
  2026-09-14, a mutant of `internal/cli/output.go` overlaid that way left the
  pseudo-terminal test green against unmutated code. Passing the map as
  `GOFLAGS=-overlay=<map>` reaches the child build, and the same mutant went red.

**`.tp-review/` is tp's, including `REVIEW-DECISION.md`.** Since tp 1.1.1 a
`PreToolUse` hook refuses a hand edit anywhere under it, citing tp's §6.2 scope
fence. Earlier entries in that file predate the fence. An open spec question
found during implementation now goes where tp accepts it: into the acceptance
of the open task that will meet it, through `tp set`, or into a closure reason.

### Reset-native subagent-per-unit

Prefer running each unit — one implementation task, or one review round's
per-role reviewers — in a **fresh subagent context**, not inline in the
orchestrator. The subagent's work reaches disk (commit, `tp done`, `.tp-review`
record) and the orchestrator re-orients from durable state (`tp status`,
`scripts/frontier.py`) between units — never `tp next`, which claims.

A fresh subagent inherits CLAUDE.md and skills but not session history, so its
first call is `tp brief <id>` for the task it was given, then `tp claim <id>`.
Inject only what tp cannot know: runtime setup
(native Read/Edit/Write may be hook-blocked, so use codedbpro) and live
operational gotchas. Subagents do not nest, so the orchestrator runs each round's
fan-out itself.

### Continuous improvement

- After each cycle, note friction and fix it immediately if it is cr's fault.
- Feedback from real reviews is the highest-signal input there is; a false
  positive that reached a colleague is a design bug, not noise.
- Every improvement is judged by the trust economy: does it reduce the chance of
  a wrong assertion reaching a human?

## Tech stack

Mirrors tp so the experience transfers.

| Concern | Choice |
|---------|--------|
| Language | Go |
| CLI | spf13/cobra |
| Colors | fatih/color |
| Terminal detection | mattn/go-isatty, with creack/pty in tests |
| File locking | gofrs/flock |
| Testing | stretchr/testify |
| JSON | encoding/json |
| Validation | manual struct validation |
| External tools | `git`, `gh`, and the configured tracker command |

## Conventions

- Exit codes: 0 success, 1 validation, 2 usage, 3 file or external command,
  4 state conflict. A line of a file the caller handed a command that does not
  decode is 1 (`state.MalformedLineError`); a line of cr's own stored state that
  does not decode is 3 with `state.UnusableHint`. A new error class is a new row
  in `internal/cli/exit.go`'s table, with its hint.
- Record lines are read the way `encoding/json` binds keys, letter case folded:
  a field fence compares the keys `state.FoldedFields` gives, never the raw
  spelling, and a line giving one key twice under that folding is refused.
- JSON output when piped or `--json`, coloured text in a TTY.
- Pretty-printed JSON with two-space indentation.
- Slice fields serialise as `[]`, never `null`. Watch for `var x []T` reaching
  JSON output; use `x := make([]T, 0)`.
- Every error carries a `hint` naming the next actionable step.
- All writes take a flock; reads are lock-free.
- Findings are stored in English; reader-facing prose is produced at draft time.
- `--compact` omits `output_tail`, `input`, and ingested thread bodies. It never
  omits `evidence` or `citations`: §8.1.2 has the agent compose every posted body
  out of those two, so a compacted payload without them leaves the composition
  unfounded. `internal/cli/output.go` refuses the pair at package init rather
  than trusting the table to stay right.
- **Every `git` invocation goes through `internal/git`'s runner.** It inherits an
  allowlist of environment variables rather than filtering a denylist, so
  `GIT_DIR`, `GIT_EXTERNAL_DIFF` and friends cannot redirect a read, and it pins
  the diff knobs that have no flag with `-c`. This is not theoretical: a dev
  machine here has `diff.external` set, and an unpinned `git diff` returned that
  differ's output instead of a unified diff. §2.1.1 requires the same inputs to
  give the same result, and git reads a lot of ambient state.
- **The review request body is the payload alone.** `cr post --confirm` hands gh
  exactly `commit_id`, `event`, `body` and `comments` on standard input
  (`--input -`), never `posted.json`, whose records, discards and outcomes
  sections exist for `--reconcile` and must not reach GitHub.

## Distribution

1. GoReleaser on a `v*` tag via `.github/workflows/release.yml`.
2. Homebrew formula published to `deligoez/homebrew-tap` under `Formula/`,
   installing `cr` and testing `cr --version`, in the `brews` block shape tp's
   v1.1.1 release uses. Measured with GoReleaser 2.17.1: `goreleaser check` prints
   `DEPRECATED: brews should not be used anymore` and exits 2 on that block, while
   `.github/workflows/release.yml` runs `goreleaser release`, which publishes it.
   So no workflow step runs `goreleaser check`; `release_test.go` asserts both.
3. `go install github.com/deligoez/cr/cmd/cr@<tag>`.
4. Skill shipped in-repo at `skills/cr/SKILL.md`, exposed through
   `.claude-plugin/marketplace.json`.
5. Version injected at build time via
   `-X github.com/deligoez/cr/internal/cli.version`.

### Pre-release checklist

1. `skills/cr/SKILL.md` reflects every new command, flag, and workflow change.
2. `CLAUDE.md` reflects any new convention or invariant.
3. `README.md` reflects every new command and feature.
4. All three are committed and included in the release tag.

### Post-release

```bash
go install github.com/deligoez/cr/cmd/cr@v<VERSION>
npx skills update -g
```

Use the exact tag, never `@latest` — the module proxy and the skill registry lag.

## Manual QA

cr cannot be QA'd against a fixture file the way tp can; it needs a real pull
request. Set up a disposable one and keep it.

```bash
# 1. Build to a temp dir. `go build` compiles uncommitted edits too, so a QA of
#    a commit builds from an extracted tree while anyone has work in progress:
#    mkdir -p /tmp/cr-qa/src && git archive HEAD | tar -x -C /tmp/cr-qa/src
#    (cd /tmp/cr-qa/src && go build -o ../cr ./cmd/cr)
mkdir -p /tmp/cr-qa && go build -o /tmp/cr-qa/cr ./cmd/cr
export CR=/tmp/cr-qa/cr

# 2. Create a scratch repository and a pull request with a deliberate mix:
#    - one hunk that implements a stated requirement
#    - one hunk that implements nothing stated (unmapped, becomes a question)
#    - one requirement with no implementation (a gap `cr status` reports)
#    - one new helper duplicating an existing one (reinvention candidate)
#    - one changed branch with no test covering it (mutation probe target)
#    - one existing comment from another reviewer (dedup and ingestion)

# 3. Point cr at it with a file-based intent source, so no tracker is needed.
#    --issue is still required: §3.1.4 bypasses the tracker command, not §3.2's
#    key resolution, and §3.3 forms every claim id as <ISSUE-KEY>#c<n>. Without
#    a key the run marks the intent axis unavailable and extracts no claim, so
#    the recipe would exercise none of what it is here to exercise.
$CR brief 1 --repo <owner>/<scratch> --issue CR-1 --intent-file issue.txt

# 4. `cr claims record` re-reads the issue, so it needs its own --intent-file.
#    Without one it shells out to the configured tracker command and fails with
#    a 404. The flag is not inherited from the brief that opened the round.
$CR claims record 1 --repo <owner>/<scratch> --intent-file issue.txt claims.ndjson
```

The scratch PR is the regression fixture. When a bug is found in a real review,
reproduce it there before fixing.

**Serialise the full suite when several units run at once.** During the v0.2.0
QA repairs, 2026-09-14, parallel units each ran the full `go test -race ./...`
gate in the same minutes and saturated the box, so the gate was put behind one
lock every unit takes first: `mkdir` of a lock directory, the holder's command
written into it, a bounded wait (40 minutes), and a lock older than 45 minutes
treated as abandoned. Measured by the orchestrator before the lock: four
`cli.test` binaries running at once held the load average at 10–12 on this
ten-core box. With the lock and two units live, the same evening read 14.11 over
one minute and 3.87 over fifteen: a burst, not a sustained saturation. When the
rule is next doubted, compare the fifteen-minute figures with and without the
lock; the one-minute figure misleads, see above.

Harness facts the 478-case QA pass against `deligoez/cr-qa` measured, kept
because they hold for the next pass:

1. **A `gh` shim cannot read the environment cr runs in.** `internal/gh/run.go`
   starts gh with an allowlist, `PATH`, `HOME` and `TMPDIR` (and `SystemRoot`)
   beside its pinned values, so a shim takes its settings (a rewritten head, the
   review POST's answer) from files at absolute paths. For the same reason a
   machine that authenticates gh only through `GH_TOKEN` has no credentials
   under cr.
2. **Fence the tracker on every command, not only where `--intent-file` might
   be dropped.** The default `intent.cmd` is a real `jira`, and on this machine
   `gh` and `jira` share `/opt/homebrew/bin`, so dropping that directory from
   `PATH` to hide jira hides gh too. Put a fake `jira` first on `PATH`, build a
   `PATH` directory of symlinks to the tools the run needs without jira, or set
   `intent.cmd` to a stub.
3. **A no-profile fixture needs files removed from the working tree.** Profile
   selection stats the marker files at the repository root
   (`internal/profile/select.go`), not in the head's tree, so delete
   `composer.json` and `phpunit.xml` from a copy of the clone rather than
   rewriting the head.
4. **`script` needs `< /dev/null` from an agent's shell.** Measured 2026-09-14:
   without it `script -q /dev/null cr --version` exits 1 with
   `tcgetattr/ioctl: Operation not supported on socket`; with it cr runs under a
   pseudo-terminal and the captured output starts with a literal `^D`.
5. **`git --no-ext-diff diff` is not a command.** git exits with
   `unknown option: --no-ext-diff`; the flag belongs to the subcommand, as
   `git diff --no-ext-diff`.

### QA checklist

| Area | What to verify |
|------|----------------|
| Intent | Unmapped hunk becomes a question; unimplemented claim is a gap in `cr status` |
| Context | A recorded note suppresses the matching unmapped-unit question |
| Grades | An `argued` record is forced to a question and the forcing is reported |
| Probes | Mutation reverts after failure and after timeout; lock serialises runs |
| Draft | Deleting a block writes a waiver; the waiver survives the next round |
| Suggestions | An out-of-hunk suggestion blocks posting with the record id named |
| Posting | No network write without `--confirm`; all comments land in one review; a posted round refuses a second |
| Unknown outcome | A 5xx or timeout exits 4; `cr draft`, `cr post --confirm` and a moved-head `cr brief` refuse until `cr post --reconcile` |
| Moved head | `cr brief` opens a new round with open records stale; anchors are not migrated (v0.3) |
| Dedup | A finding matching an existing human thread is suppressed |
| Honesty | A disabled axis appears in the report with its reason |
| Nil slices | Empty collections serialise as `[]`, never `null` |
