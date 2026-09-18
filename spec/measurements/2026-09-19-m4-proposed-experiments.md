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
