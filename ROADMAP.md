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
every later theme in VISION.md's table moved with it. v0.3.0 (2026-09-16) then went the same way: the
first field trial on a real pull request, and the test-environment incident it exposed, took v0.2.2,
v0.2.3 and v0.3.0 between them, so the conversation half moved again — to v0.4.

The lesson that reorders this list: **what cr has not been run against, it has not been shown to do.** The
largest open question is not a missing feature but an unmeasured bet, so measurement comes before breadth.

## Shipped

| Version | What it delivered |
|---------|-------------------|
| v0.1.0 | One-head reviewer loop: intent, four axes, probes, draft, human triage, one posted review |
| v0.2.0 | The loop repaired against real pull requests; marker `side`, context-window waiver keys, `commit_id`-pinned reviews, exit-code-aware probe ladders, closed/merged disclosure |
| v0.2.1 | Known low defects and two limitations of v0.2.0 (per-clone probe lock, runner start window), no contract change |
| v0.2.2 | Test-environment safety after a field trial ran the sandbox against a developer's application database: `.env.testing` copied, every gitignored env file the sandbox lacks reported, an experiment header before every run, a stale sandbox rebuilt |
| v0.2.3 | What the field trial's operator asked for, within the v0.2 contract: notes that postdate a prompt reported, recorded cells marked, duplicate candidates listed, issue text cleaned and its links and uncovered paragraphs disclosed |
| v0.3.0 | The contract caught up with the trial: a probe baseline scoped to its own filter and paths, `sandbox.require`, `--intent-extra` issue files, a `cr review` that emits only what the round still owes (`--units`, `--shard`, `--all`) over one per-round contract file, role class vocabularies, `cr triage`, §2.3's state-file fence, and `cr init` refreshing ejected roles |

## Next

### 1. Measure the bet (before anything else)

cr has reviewed scratch pull requests on `deligoez/cr-qa` and one real pull request, in the field trial
recorded in `spec/field-feedback.md`. That trial measured the operator's experience, not the bet: cr's
central claim — a wrong comment costs trust, so cr trades recall for precision and turns weak findings
into questions — still has no measurement.

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

  **Measured 2026-09-18** (`spec/measurements/2026-09-18-m1-recall-against-known-defects.md`; deligoez/cr#1,
  the v0.1.0 `brief`, `sandbox`, `probe`, `run`, `draft` and `coverage` packages added from nothing, 24 targets
  with pre-registered rules, two readers matching blind to each other, Opus roles from a fresh clone with the
  memory plugin off). Reading roles with 110 mechanically extracted claims: 123 prompts, $89, 23 records (2
  findings, 21 questions), **0 of 16 targets found, 0 of 8 partials, 0 false assertions**, 0 of 6 omissions
  raised as gaps; five near-misses within lines of a target asking a different question. Without the intent
  axis: 82 prompts, $60, 9 distinct records, 0 and 0 again, 0 false assertions. Found instead: two real defects
  the 14 recorded audit rounds had passed (`rekey.go:14-40` PASS in all 14), one more without claims, and two
  0.1.0-text departures 0.2.0 later codified, as questions. The reading offered: the roles reached the right
  places and lacked the scenario that turns a place into a defect; the 24 were found by executing scenarios.

  **Pass 2, the same subject with probes** (a `go` profile, test axis active, roles proposing experiments the
  orchestrator ran): recall **did not move — 0 of 16 and 0 of 8 again**, with more mechanical candidates (8) and
  the same behaviour mismatch. What execution bought instead: 21 mutations proven uncaught by the tests they
  selected, 20 records resting on them as `probed`, and two gap probes that failed on the head, deciding two
  suspicions by experiment; both became v0.3.1 fixes. Consequence for this list: not "probes before breadth" — a
  probe only settles the question a role already asked. The instrument none of these three passes has is the
  **scenario**: the 24 were found by driving commands against a fixture until one behaved wrongly. QA 24 of 24,
  reading 0, probes 0. The misses did not cluster on cross-cutting items (5), they were case-shaped.

### 2. Isolation for untrusted code

Probes run the pull request's `tests.cmd` on the reviewer's machine in a git worktree, with the reviewer's
permissions. That is acceptable for a colleague's pull request and not for an outside contributor's. Before
cr is pointed at code from outside the team: a container or equivalent sandbox for test runs, no network by
default, and an explicit profile opt-in for anything else.

### 3. The conversation (v0.4)

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

### 4a. Scenario coverage (new, from measurement 1)

The measurement gives all three instruments a number against the same 24 defects: a QA pass that drove commands
found 24, reading roles found 0, and probes proposed by those roles found 0. A probe decides a suspicion; nothing
in the loop *produces* the suspicion a scenario produces. Candidates, cheapest first: a role whose prompt carries
the command surface and asks what a user would do with this unit and what would then go wrong; a fixture
repository the round can drive commands against, which is what `deligoez/cr-qa` already is for cr itself;
`cr probe run --kind gap` used the way the QA used its cases, one command sequence per probe rather than one unit
test. To be chosen after a cheap trial: run one scenario-shaped role over the same subject and count against the
same 24.

### 5. Cross-cutting requirements

A unit is built from hunks, so a requirement that spans the change ("every write takes the lock") is seen in
thirty pieces and never whole. Candidates, to be chosen after measurement 1 shows whether misses cluster
here: a claim mapped to more than k units gets a whole-change pass by a dedicated role, and a claim about
tree state no hunk touches (a README listing every command) gets a tree-level check.

### 6. Operator cost

A round is about ten commands orchestrated by the skill. v0.3.0's narrowed fan-out, shards and `cr triage`
took the repeated work out of a second pass; what is left, once the round's cost is measured (1):

- a single driver for the mechanical steps, leaving the agent only the judgement steps;
- prompt size budgets for large pull requests (248 prompts measured 2.3 MB), and an honest report of what
  was cut when `post.max_comments` or the probe cap bites.

### 7. Team use

State, waivers and triage statistics live in one reviewer's `~/.cr`. A team cannot share "this class is
wrong" or a context note. VISION.md's original team theme — write context supplements back to the tracker,
share the context store — belongs here, together with shared repository-wide waivers.

### 8. Author side

VISION.md's original author-side theme: on the user's own pull requests, ingest incoming review comments as
a work list.
Sequenced after the reviewer side has been measured, because it is a second product.

## Questions to settle by measurement

- **Does the question channel converge?** If the question-to-finding ratio on a real spec is high, a
  convergence rule that counts open questions never reaches clean, and one that does not count them loses
  recall. First data point, measurement 1 (2026-09-18): 21 questions to 2 findings with the spec's items as
  claims, 6 to 4 without; every finding true. The ratio is high exactly when the claims are present.

## Known limits kept by design

- `COMMENT` is the only review event; cr never approves or requests changes.
- An interrupted runner exits 4; the next command recreates the sandbox.
- The out-of-block record-id refusal applies once `cr review` has emitted the round.
- `cr status` on a moved head omits the halves computed for a head that is no longer current.
- Windows is not supported: probes rely on process groups and `flock`.
