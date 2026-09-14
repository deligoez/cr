# cr — Roadmap

`VISION.md` says why cr exists; each `spec/<version>.md` is the normative contract of one release. This
file is neither. It is the working list of what cr lacks, what is sequenced next, and what has to be
measured before it is decided. It is revised whenever a release ships or a measurement lands, and an item
leaves it only by becoming a spec or by being dropped with its reason.

## Where the plan came from, and why it moved

The first spec (`e106957`, 2026-08-05) put the whole reviewer loop in v0.1, including "re-reviewing after
the author pushes, and resolving threads". Spec review cut the re-review half out of v0.1, and VISION.md's
roadmap then placed it in v0.2. v0.2.0 (2026-09-14) became a repair release instead: a 478-case QA pass
against real pull requests found defects fourteen audit rounds had passed, and the user chose to ship those
repairs and the spec changes they forced as v0.2.0. The conversation half therefore moved to v0.3, and
every later theme in VISION.md's table moved with it.

The lesson that reorders this list: **what cr has not been run against, it has not been shown to do.** The
largest open question is not a missing feature but an unmeasured bet, so measurement comes before breadth.

## Shipped

| Version | What it delivered |
|---------|-------------------|
| v0.1.0 | One-head reviewer loop: intent, four axes, probes, draft, human triage, one posted review |
| v0.2.0 | The loop repaired against real pull requests; marker `side`, context-window waiver keys, `commit_id`-pinned reviews, exit-code-aware probe ladders, closed/merged disclosure |
| v0.2.1 | Known low defects and two limitations of v0.2.0 (per-clone probe lock, runner start window), no contract change — in progress |

## Next

### 1. Measure the bet (before anything else)

cr has only reviewed scratch pull requests on `deligoez/cr-qa`. Its central claim — a wrong comment costs
trust, so cr trades recall for precision and turns weak findings into questions — has no measurement.

- **Real use.** Review real pull requests with cr, posting only what the reviewer would have posted anyway,
  and record per round: comments kept, softened, deleted, marked `wrong`; author replies; wall clock and
  token cost of the round. §7.3's triage statistics exist for this and have no data yet.
- **Against a human review.** On pull requests that already carry a careful human review, compare what cr
  raises with what the human raised: overlap, cr-only, human-only, and cr's false assertions (the number
  that should be zero).
- **Recall against known defects.** The defects the v0.2.0 QA pass found are recorded with repros and the
  sections they violated. Reverse the `qa-` fix commits so the diff under review is the defective code, give the violated
  sections as intent, and count findings, questions and misses — and whether the misses are cross-cutting
  (see 5).

### 2. Isolation for untrusted code

Probes run the pull request's `tests.cmd` on the reviewer's machine in a git worktree, with the reviewer's
permissions. That is acceptable for a colleague's pull request and not for an outside contributor's. Before
cr is pointed at code from outside the team: a container or equivalent sandbox for test runs, no network by
default, and an explicit profile opt-in for anything else.

### 3. The conversation (v0.3)

The re-review half the first spec promised:

- anchor migration across a push, using the context window and content hash v0.1 and v0.2 already record;
- recheck and verification: after the author pushes, is each posted concern addressed;
- resolving and withdrawing threads, still behind `--confirm`;
- reading author replies from GitHub instead of storing them by hand with `cr answer`;
- a question closed by an answer is neutral, an open question is not — the rule a convergence criterion needs.

### 4. Profiles and ecosystems

Only `laravel-pest` and `generic` ship. On any other stack the reinvention lens is off (no symbol index),
the test axis needs a hand-written `tests.cmd`, and mutation probes need profile templates.

- profiles for Go, TypeScript and Python, each with a symbol scanner and probe templates;
- tracker commands for GitHub Issues and Linear alongside the jira CLI;
- GitLab and Bitbucket are not planned until a user needs them; cr's single network-write door keeps that
  change contained.

### 5. Cross-cutting requirements

A unit is built from hunks, so a requirement that spans the change ("every write takes the lock") is seen in
thirty pieces and never whole. Candidates, to be chosen after measurement 1 shows whether misses cluster
here: a claim mapped to more than k units gets a whole-change pass by a dedicated role, and a claim about
tree state no hunk touches (a README listing every command) gets a tree-level check.

### 6. Operator cost

A round is about ten commands orchestrated by the skill. Once the round's cost is measured (1):

- a single driver for the mechanical steps, leaving the agent only the judgement steps;
- prompt size budgets for large pull requests (248 prompts measured 2.3 MB), and an honest report of what
  was cut when `post.max_comments` or the probe cap bites.

### 7. Team use

State, waivers and triage statistics live in one reviewer's `~/.cr`. A team cannot share "this class is
wrong" or a context note. VISION.md's v0.4 theme — write context supplements back to the tracker, share the
context store — belongs here, together with shared repository-wide waivers.

### 8. Author side

VISION.md's v0.3 theme: on the user's own pull requests, ingest incoming review comments as a work list.
Sequenced after the reviewer side has been measured, because it is a second product.

## Questions to settle by measurement

- **Can cr take over the implementation audit of `tp`?** Its intent axis already does the audit's backward
  pass (every requirement mapped, unmapped ones reported as gaps), with executed evidence where an audit
  reads. It cannot yet give a forward verdict with convergence (needs 3), and it sees cross-cutting and
  tree-state requirements poorly (5). Decide with the reversed-fixes experiment in 1; the likely answer is a
  hybrid: cr on the change, audit roles only on the requirements cr's mapping flags as cross-cutting or
  unmappable. cr's own releases keep external QA whatever the result.
- **Does the question channel converge?** If the question-to-finding ratio on a real spec is high, a
  convergence rule that counts open questions never reaches clean, and one that does not count them loses
  recall.

## Known limits kept by design

- `COMMENT` is the only review event; cr never approves or requests changes.
- An interrupted runner exits 4; the next command recreates the sandbox.
- The out-of-block record-id refusal applies once `cr review` has emitted the round.
- `cr status` on a moved head omits the halves computed for a head that is no longer current.
- Windows is not supported: probes rely on process groups and `flock`.
