# cr — Code Review

Code review lifecycle manager for AI coding agents. Go CLI tool.

`VISION.md` explains why this exists and what it bets on. `spec/0.1.0.md` is the
normative v0.1 contract. This file holds the working conventions and the rules
that are easy to violate by accident.

## Install

```bash
brew tap deligoez/tap && brew install cr          # Homebrew, once released
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
   `~/.cr/`. The user's worktree, index, and branch are read-only to cr.
3. **The confirmation gate cannot be configured away.** There is no setting, env
   var, or profile field that makes a network write implicit.
4. **A finding graded `argued` cannot be posted as an assertion.** The forcing
   happens in cr, not in the prompt, so no agent can talk its way past it.
5. **Exit codes never get renumbered.** `internal/cli/exit.go` pins them and a
   guard test asserts the values.
6. **Probes revert.** A mutation is undone even when the run fails, times out, or
   panics.

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
2. **A surviving mutant is not a score to drive down.** Classify them: an
   equivalent mutant nothing can observe, an undocumented boundary, or a
   documented contract with no boundary test. Only the last is worth acting on,
   and say which ones are being left and why. `gremlins` is load-sensitive — a
   run full of `TIMED OUT` is not a result.

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

Everything below is **specified, not implemented**. `spec/0.1.0.md` §11 is the
source of truth; this table is a map, not a promise.

| Command | Purpose |
|---------|---------|
| `cr init [--eject-roles]` | Create `~/.cr`, write default profiles, roles, and rules |
| `cr brief <pr>` | Orientation payload; opens a new round when the head moved |
| `cr review <pr> [--axis <id>]` | Emit per-role, per-unit prompts and output paths |
| `cr claims record <pr> <file>` | Store the claims extracted from the issue |
| `cr map record <pr> <file>` | Store the claim-to-unit mapping |
| `cr claims set-aside <pr> <claim-id> --note <id>` | Mark an unimplemented claim out of scope |
| `cr cells record <pr> <file>` | Store the coverage cells the roles filled |
| `cr merge <files...> -o <out> --repo <r> --pr <n>` | Merge and deduplicate per-role findings |
| `cr record <pr> <file>` | Record a round's merged findings |
| `cr sandbox create\|destroy <pr>` | Manage the probe worktree |
| `cr test <pr> [--filter]` | Run the suite inside the sandbox |
| `cr probe run <pr> --kind <kind> ...` | Execute and record a mutation or gap probe |
| `cr draft <pr>` | Render the editable draft |
| `cr post <pr> [--confirm] [--reconcile]` | Validate and post the review |
| `cr answer <pr> <record-id> <text>` | Store the answer to a posted question as a note |
| `cr note <ISSUE-KEY> <text>` / `--remove <id>` | Store or retract an out-of-band fact |
| `cr context <ISSUE-KEY>` | Print accumulated context with provenance |
| `cr rules list\|check\|suggest` | Inspect, run, and harvest project rules |
| `cr waivers list\|remove --repo <r> [--pr <n>]` | Inspect and edit waivers in either scope |
| `cr stats --repo <r>` | Triage statistics, demotion and volume candidates |
| `cr status <pr>` | Coverage, states, and completeness |
| `cr config [--resolved]` | Effective configuration and its layers |

v0.1 ends at posting. `cr recheck`, `cr verify`, `cr resolve`, and `cr accept`
are **not** v0.1 commands — the re-review half of the loop is v0.2, declared out
of scope in `spec/0.1.0.md` §1.3.6. A moved head makes the round stale (§9.3);
`cr brief` opens a new one.

## Project structure

```
cmd/cr/              Main entry point
internal/
  cli/               Cobra commands, exit codes
spec/
  0.1.0.md           Normative v0.1 contract
  <version>.md       One spec per version
skills/cr/
  SKILL.md           Claude Code skill (ships with the release)
.claude-plugin/
  marketplace.json   Skill distribution manifest
```

Runtime state never lives here. It lives under `~/.cr/`, laid out in
`spec/0.1.0.md` §2.2.

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
  Turkish, but they live in `~/.cr/` state, never in this repository.

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

### Reset-native subagent-per-unit

Prefer running each unit — one implementation task, or one review round's
per-role reviewers — in a **fresh subagent context**, not inline in the
orchestrator. The subagent's work reaches disk (commit, `tp done`, `.tp-review`
record) and the orchestrator re-orients from durable state (`tp resume`,
`tp next`) between units.

A fresh subagent inherits CLAUDE.md and skills but not session history, so its
first call is `tp next --brief`. Inject only what tp cannot know: runtime setup
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
  4 state conflict.
- JSON output when piped or `--json`, coloured text in a TTY.
- Pretty-printed JSON with two-space indentation.
- Slice fields serialise as `[]`, never `null`. Watch for `var x []T` reaching
  JSON output; use `x := make([]T, 0)`.
- Every error carries a `hint` naming the next actionable step.
- All writes take a flock; reads are lock-free.
- Findings are stored in English; reader-facing prose is produced at draft time.
- `--compact` omits `evidence`, `output_tail`, `input`, and ingested thread
  bodies.
- **Every `git` invocation goes through `internal/git`'s runner.** It inherits an
  allowlist of environment variables rather than filtering a denylist, so
  `GIT_DIR`, `GIT_EXTERNAL_DIFF` and friends cannot redirect a read, and it pins
  the diff knobs that have no flag with `-c`. This is not theoretical: a dev
  machine here has `diff.external` set, and an unpinned `git diff` returned that
  differ's output instead of a unified diff. §2.1.1 requires the same inputs to
  give the same result, and git reads a lot of ambient state.

## Distribution

1. GoReleaser on a `v*` tag via `.github/workflows/release.yml`.
2. Homebrew formula published to `deligoez/homebrew-tap`, installing `cr`.
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
# 1. Build to a temp dir
mkdir -p /tmp/cr-qa && go build -o /tmp/cr-qa/cr ./cmd/cr
export CR=/tmp/cr-qa/cr

# 2. Create a scratch repository and a pull request with a deliberate mix:
#    - one hunk that implements a stated requirement
#    - one hunk that implements nothing stated (unmapped, becomes a question)
#    - one requirement with no implementation (becomes a finding)
#    - one new helper duplicating an existing one (reinvention candidate)
#    - one changed branch with no test covering it (mutation probe target)
#    - one existing comment from another reviewer (dedup and ingestion)

# 3. Point cr at it with a file-based intent source, so no tracker is needed.
#    --issue is still required: §3.1.4 bypasses the tracker command, not §3.2's
#    key resolution, and §3.3 forms every claim id as <ISSUE-KEY>#c<n>. Without
#    a key the run marks the intent axis unavailable and extracts no claim, so
#    the recipe would exercise none of what it is here to exercise.
$CR brief 1 --repo <owner>/<scratch> --issue CR-1 --intent-file issue.txt
```

The scratch PR is the regression fixture. When a bug is found in a real review,
reproduce it there before fixing.

### QA checklist

| Area | What to verify |
|------|----------------|
| Intent | Unmapped hunk becomes a question; unimplemented claim becomes a finding |
| Context | A recorded note suppresses the matching unmapped-unit question |
| Grades | An `argued` record is forced to a question and the forcing is reported |
| Probes | Mutation reverts after failure and after timeout; lock serialises runs |
| Draft | Deleting a block writes a waiver; the waiver survives the next round |
| Suggestions | An out-of-hunk suggestion blocks posting with the record id named |
| Posting | No network write without `--confirm`; all comments land in one review |
| Recheck | Anchors migrate across a force-push; unresolvable ones become stale |
| Dedup | A finding matching an existing human thread is suppressed |
| Honesty | A disabled axis appears in the report with its reason |
| Nil slices | Empty collections serialise as `[]`, never `null` |
