# Measurement 4 — what the proposal channel carries

**Pre-registration, written 2026-09-19 before any run.** Every number, threshold and reading below is
fixed here; a result read against a threshold written afterwards is interpretation, not measurement.

ROADMAP.md "Next 1, Measure the bet". Subject: §5.7, shipped in v0.4.0. The obligation it added lets a role
that holds a suspicion it cannot establish propose the experiment that would settle it, and running that
proposal re-grades the record. Nothing has measured what the channel carries.

## The correction this measurement starts from

v0.4.0's release notes, and the argument that chose it as the release topic, read measurement 3's grade
distribution — 32 `cited`, 19 `argued`, **0 `probed`** — as evidence that the missing channel was what kept
cr from grading anything `probed`.

**That was overstated, and M3's own setup says so.** Its Setup section reads: "No sandbox is created and no
probe can run, so an unestablished suspicion stays a question." So M3's zero has **two** causes, not one:
roles had no way to ask for an experiment, *and* no experiment could have run in that environment even if
one had been asked for. §5.7 removes the first. It removes the second nowhere.

That is why this measurement is in two parts. Part A asks the question M3's subject can answer, and Part B
goes to a repository where a probe can actually run.

## Part A — does the channel carry traffic, and is the traffic any good?

Same subject, same setup, one variable changed.

- **Subject**: `tarfin-labs/backend#3757` at `3cef86cb`, the commit 16 of the human's 25 comments were
  written against. Identical to measurement 3's run B: 10 files, 263 lines, 19 units.
- **Changed**: the binary is v0.4.0 rather than v0.3.0, so every prompt now names a proposals file and a
  block of `x<n>` ids (§4.6.2) and carries §5.7's contract.
- **Unchanged**: the `gh` shim that answers `reviewThreads` with an empty page and refuses any call
  carrying `--method`, `-X` or `--input`; the pull request body's six claims as intent; profile
  `laravel-pest` with all four roles; Opus, one `claude -p` session per prompt from a detached worktree;
  the memory plugin off, with the runner refusing to start unless `hosts.claude_code.enabled` is `false`.
- **Still not runnable**: no sandbox, no probe. Part A measures what roles *ask for*, never what running it
  establishes.

### What is counted

| | |
|---|---|
| A1 | Proposals written, over 19 units × 4 roles |
| A2 | Of those, how many `cr proposals record` accepts, and the refusal of each it does not |
| A3 | Of the accepted, how many target a line inside one of the human's 16 comments |
| A4 | Of the accepted, how many a reader would spend a probe on, judged by hand against the code |
| A5 | Records that name a proposal in `finding`, and the grade each holds |

### Pre-registered readings

- **A1 = 0** is a real outcome and the most important one to be able to report. It would mean §5.7 shipped a
  channel no role uses, and that the next work is the prompt — or the role corpus — rather than more
  surface. It is written here so that a null result cannot be quietly re-read as "the subject was wrong".
- **A2 well below A1** means the contract is stated where roles do not read it, or is stated in a way they
  cannot satisfy. Every refusal is kept verbatim; the refusals are the finding, not noise.
- **A4 near zero with A1 high** is the expensive failure: roles asking for experiments that settle nothing,
  which would spend the reviewer's probe budget on noise. A4 is judged by hand, by one reader, against the
  code and with the human's comments visible — the judgement is made after the run and its criterion is
  fixed now: *would running this change what the record may assert?*
- A3 is context, not a score. A proposal on a line the human never commented on may be a better experiment
  than one on a line they did.

## Part B — does running a proposal change the register?

- **Subject**: cr's own repository, as measurement 1 used it: v0.1.0 packages presented as a pull request.
  A Go suite runs in a sandbox with `go test` and needs no database, which is what makes a probe possible
  here and impossible in Part A.
- Sandbox created, probes allowed, `probe.max_per_round` at its default of 10.
- Every accepted proposal that a reader judges worth running (Part A's A4 criterion) is run with
  `cr probe run --proposal <id>`, in the order the round holds them, until the cap.

### What is counted

| | |
|---|---|
| B1 | Proposals run, and the probe result each produced |
| B2 | Records that reached `probed`, against measurement 3's comparable 0 |
| B3 | Proposals that ran and changed no grade, with the reason (`failed` disproves the gap, a filtered run proves less, and so on) |
| B4 | **False assertions among the records that reached `probed`**, checked by hand against the tree |

### Pre-registered readings

- **B4 must be 0.** This is the trust-economy number and the only one with a threshold rather than a
  reading. A record that reached `probed` on an experiment that did not establish what it claims is worse
  than the argued question it replaced, because the register invites the reviewer to post it. One such
  record makes §5.7 a defect and the next work is its repair, whatever B2 says.
- **B2 = 0 with B1 > 0** means proposals run and settle nothing: the channel works and the experiments do
  not. The next work would be what a role is told to propose.
- **B3 high is not a failure.** §5.3.5 and §5.4.4 decide what a result establishes, and a mutation the suite
  noticed disproves the gap. A proposal that ran and left the record where it was is an answer.

## Fences

Inherited from measurements 1 and 3, each because it failed once:

1. The memory plugin is off and the role runner refuses to start otherwise. It leaked into a batch twice,
   voiding 123 transcripts and then 22.
2. Roles run as `claude -p` from a detached worktree or a fresh clone, never through the Agent tool, which
   inherits this repository's CLAUDE.md and its memory index.
3. codedbpro relative paths resolve against the daemon's tree rather than the clone, so roles are given
   absolute paths and every transcript's `path`/`file` argument is audited afterwards.
4. The `gh` shim logs every invocation with its verdict; a transcript that reached the answer key is voided.
5. The worktree is byte-clean after every batch, verified with `git status --porcelain`, and nothing is
   pushed.

## Record

Rig under `spec/measurements/m4/`. Results are written into this file below this line when the runs
complete, and the pre-registered text above is not edited afterwards.

---

# Result, Part A — the channel carries, and what it carries is well formed

Run 2026-09-19 to 2026-09-20. 76 prompts over 19 units × 4 roles, one `claude -p` session each, Opus.

| | |
|---|---|
| sessions | 76 of 76 |
| transcript audit | **76 clean, 0 voided** |
| wall clock | 46.1 hours |
| per-session gap | median 0.6 min, p90 1.6 min, **max 1745 min** |
| cost | $52.16 |
| worktree after every batch | byte-clean |

**The 46 hours are one wait, not a rate.** Seventy-five sessions landed about a minute apart; one,
`u14-test-adequacy`, sat for 29 hours. Its transcript carries two `rate_limit_event` entries with
`overageStatus: rejected, overageDisabledReason: out_of_credits`, and the two `resetsAt` values are 31.5
hours apart: the five-hour usage window was spent, overage was off, and `claude -p` waited the window out
rather than failing. The session then ran for 82 seconds and succeeded. So the stall is invisible to a
runner that watches exit codes, which is why `run-roles.sh` now caps a session in wall clock and
`run-batch.sh` retries a capped one on a later pass.

## The pre-registered numbers

**A1 — proposals written: 18**, over 8 of the 19 units.

| role | proposals |
|---|---|
| test-adequacy | 15 |
| correctness | 3 |
| convention | 0 |
| intent-coverage | 0 |

The null result A1 was written to allow did not happen, and the distribution is the part worth keeping:
the two roles whose findings can only be established by an experiment wrote every proposal, and the two
whose findings rest on reading wrote none. §4.4.2 says the same thing normatively — "a test-adequacy
finding asserts only with an experiment" — so the channel was used by exactly the roles the contract
points at. All 19 intent prompts ran first and produced 0; reading that pass alone as the answer would
have been wrong, and it is recorded here because it nearly was.

**A2 — accepted: 18 of 18, with no refusal.** 11 `mutation`, 7 `gap`; all stored `open`, so §5.7.5 found
none unrunnable; 17 of 18 name a record in `finding`. §5.7.1's refusals — the id block, the unit, the
target's containment, the active role, the empty required string — caught nothing, on the first run, from
roles that had never seen the section before.

**A3 — 16 of 18** target a file the human reviewer commented on.

**A4 — judged by the two clauses that decide whether running one can grade anything**, rather than by
taste:

| | |
|---|---|
| §6.2.2, target inside the named record's anchor range | **17 of 17** |
| §5.4.4, gap proposal whose record carries a claim mapped to its unit | **6 of 6** |

So every proposal that names a record is positioned to re-grade it if its result comes out the way its
hypothesis predicts. This is the measurable half of "worth spending a probe on"; the other half is
whether the experiment is a good one, which only Part B can answer by running it.

**A5 — the records the 17 proposals name**: 14 `argued` questions, 1 `cited` question, 2 `cited` findings.
The fourteen are precisely what §5.7 exists for — records that stayed questions because the role could not
establish them.

## Beside the pre-registration

**cr produced fewer records than measurement 3 did on the same subject**: 31 against 51, with 4 findings
against 9 and grades cited 13 / argued 18 against 32 / 19. The setup differs in one thing, the binary, so
the difference is either the model's run-to-run variation or the §5.7 section taking attention the other
sections had. This measurement cannot tell those apart and does not try; it is recorded because a reader
comparing the two runs will see it.

Probed records: **0**, as in measurement 3 — no sandbox, no probe, by construction.

## What went wrong in the operating, recorded because it cost evidence

1. **Progress was reported three times without once measuring elapsed time.** The session count said 93%
   done while the clock said a day had been spent inside one session. The count was the flattering number
   and it was the only one read.
2. **A process check used `grep -c 'run-roles'`, which counts its own command line.** It returned 4 and was
   read as "alive"; it would have returned a non-zero count with nothing running. CLAUDE.md records the
   same failure for `pgrep -f`.
3. **The new session cap was tested against a prompt whose evidence was already collected.** A five-second
   cap on `u1-convention` opened a fresh session, killed it, and truncated that transcript from ~40 KB to
   15928 bytes. No counted output was lost — that role wrote no records and no proposals anywhere, and its
   usage line and cell survive — but the transcript audit can no longer be re-run for that one session.
   The audit had already reported 76 of 76 clean. A destructive guard is tested on a throwaway input.


# Result, Part B — an experiment moves the grade, and the field that wastes one is `paths`

Subject: cr's own v0.1.0 packages presented as `deligoez/cr#1`, the measurement-1 fixture, in a fresh
clone at `86e2193b`, with a `gh` shim serving the pull request and a `go test` sandbox. One role,
`test-adequacy`, over 22 units.

22 sessions ran in 60 minutes (20:58:08–21:58:19) at $38.03, model opus, 59–298s each. The transcript
audit came back 21 clean and 1 flagged: `u58-test-adequacy` called `Skill`, which loaded codedbpro's
own skill and carried no lesson of this repository, so the session was kept and `Skill` was added to
the runner's `--disallowedTools` afterwards. The clone was byte-clean after the batch and nothing was
pushed.

## Two refusals that cost a role its finding, and are both correct

18 records and 18 proposals were written; 17 of each were stored.

`f31902` was a question about the branch that handles cr's own reserved marker sequence, and its
evidence quoted the sequence in order to name it. §8.1.3 reserves `<!-- cr:` for the marker and cr's
own regions, so `cr record` refused the record. `x31902` named `f31902`, so `cr proposals record` then
refused the proposal: *finding names "f31902", which is no record of the current round*. Neither
refusal is wrong and together they drop a whole unit's work, because a reviewer of cr's draft
machinery cannot describe the machinery in cr's own record. The exposure is narrow — it needs the
literal sequence — but it is exactly the self-referential case dogfooding is for.

## The pre-registered numbers

| | |
|---|---|
| **B1** proposals accepted | **17** — 15 mutation, 2 gap, all 17 naming a finding |
| **B1** judged worth running | **10** |
| **B1** ran | **9** |
| **B1** refused before running | **1** |
| **B1** `no-test-failed`, establishing a gap | **7** |
| **B1** `no-tests-selected`, establishing nothing | **2** |
| **B2** records that reached `probed` | **7**, against measurement 3's comparable 0 |
| **B3** ran and changed no grade | **2** |
| **B4** false assertions among them | **0 — see below, the set is empty by construction** |

Every baseline run counted 1453 tests and 0 failed. Each of the seven moved `argued` → `probed` inside
the round, through §5.7.4's exception to §6.2's ratchet, and eight proposals were left `open`.

## What Part B actually found: `paths` is unexplained, and it is the only thing that wasted a run

Three of the ten attempted runs produced nothing, and all three failed on the same field.

- `x36101` gave `paths: ["internal/probe/resolve.go"]`. The runner was invoked
  `go-runner.sh ./internal/probe/resolve.go`, which selects no tests: `no-tests-selected`, establishes
  `nothing`.
- `x36702` gave a `filter` and no `paths`, and also selected none.
- `x34101` gave `paths: ["internal/draft/rendered_decode.go"]` and was refused before running, because
  §5.4.2 places a gap probe's test file where `tests.probe_path_template` says — `internal/cli/` — and
  no `--path` covered it. Its `filter` read `-run TestARenderedJSONHoldingNullIsRefusedNamingTheFile
  ./internal/draft`: a whole `go test` argument string where a test name belongs.

`--path` scopes **which tests run**, not **which file is under test**, and nothing the role was given
says so. The emitted prompt glosses every other field and these two not at all:

> A proposal carries id (required), kind (required), role (required), unit (required), finding
> (optional), target (required), hypothesis (required), settles (required), input (required), filter
> (optional), paths (optional). `kind` is one of mutation or gap; `target` is a path:line inside this
> unit; `hypothesis` is what you believe and cannot establish; `settles` is the result that would
> settle it; `input` is the unified diff for a mutation or the test file's content for a gap.

`internal/review/contract.go:100` is where that sentence is built, and it glosses `kind`, `target`,
`hypothesis`, `settles`, `input` and `finding` and stops. §5.7's table does define the two rows — *the
test filter the run is to use*, *the `--path` values the run is to use* — but the role never sees the
table, and even in it the wording is circular for someone who has never run `cr probe run`. A role fills
an unexplained field with the file it is reasoning about, which is the one answer that cannot work.
Nothing else cost a run. The repair is two glosses in the prompt and a less circular pair of rows in
§5.7 — not the runner, which behaved exactly as §5.4.2 says.

## B4 is 0, and the 0 is vacuous

Two separate things are true, and reading them as one would overstate what §5.7 has bought.

**The seven results are what the tree says.** Each stored mutation was applied to a copy of its file
and mapped over the original with `go test -count=1 -overlay`, with the map also passed as `GOFLAGS`
so the tests that build the binary see it, over the whole tree in the clone at `86e2193b`. All seven
suites stayed green, which is `no-test-failed` independently. The instrument was checked in the same
run: an eighth, control mutation of `markerOpen`'s token went red, exit 1 with 5 `FAIL` packages, in
85 seconds against the seven greens' 91–93. So a green here is the tree's answer and not the overlay
failing to reach the code.

**But no record asserts anything.** §5.7.4 raises a record's *grade*; a record's *kind* is §7.2's, and
only the human's edit in the draft turns a question into a finding. All seven stayed
`kind: question`, 13 `low` and 4 `medium`. B4 asks for false assertions among the `probed` records and
there are no assertions among them at all, so 0 is a true reading of an empty set rather than evidence
that the grading is sound. The pre-registration did not anticipate that, and it is recorded here
rather than repaired in the pre-registered text.

What §5.7 demonstrably buys, then, is narrower than "a role can establish a finding": seven questions
the human *may* promote in the draft, since §6.3.3 admits `question` → `finding` only on `probed` or
`cited`, each carrying the cr-owned evidence region the draft renders under it. Whether a human then
promotes one, and whether the promoted one is true, is what a measurement 5 would have to watch, and
it cannot be watched without a human in the loop.

## One instrument note

`git apply` refuses a zero-context hunk unless `--unidiff-zero` is passed, and one of the seven stored
patches has zero context. Verifying by hand without the flag reports `patch does not apply` for a patch
cr had already applied successfully — a refusal that reads as a contradiction of cr's own result and is
not one. `patch(1)` applies the same file without a word. Six of the seven patches carried context and
applied either way, so the disagreement appears on exactly one row and looks like a stale target.
Check which applier is in hand before concluding a stored patch no longer fits.
