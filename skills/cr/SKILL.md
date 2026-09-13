---
name: cr
description: Code review lifecycle manager for AI coding agents. Orients on a pull request, fans review out per unit and role, merges and grades findings, runs mutation and gap probes, renders an editable draft for a human, and posts only on --confirm. Use when reviewing a GitHub pull request with cr, when a ~/.cr state directory exists for the repository, or when the user mentions cr brief, cr review, cr draft, or cr post.
---

# cr — Code Review

cr's output goes to a colleague, and a wrong comment costs the reviewer standing
that is spent once. cr never forms an opinion (it calls no language model): it
fetches, executes, validates and records, and **you** judge. Where you cannot
establish something, cr turns it into a question rather than an assertion.

Everything below was run against a scratch repository with a local `gh` shim, on
the binary built from this repository. Output is JSON when piped (which is how an
agent runs it) and text in a terminal; the shapes quoted are the JSON ones,
trimmed with `…`.

## Setup

```bash
cr init
```

```json
{
  "root": "/Users/you/.cr"
}
```

`cr init --eject-roles` also writes the built-in roles as editable files. State
lives only under `~/.cr/` (override with `CR_HOME`); cr never writes inside the
repository under review.

**Repository detection.** Every PR-scoped command reads owner/repo from the
repository's one GitHub remote. `--repo <owner/repo>` overrides it. No remote,
several remotes, or a non-GitHub remote is refused with exit 2:

```json
{
  "error": "cannot detect the repository under review: /path/to/repo declares no remote",
  "hint": "run cr inside a clone whose one remote is its GitHub repository, or pass --repo <owner/repo>"
}
```

`cr` needs `git`, `gh` (authenticated), and, unless you pass `--intent-file`, the
configured tracker command (`intent.cmd`, default `jira issue view {key} --plain`).

## The loop

brief → review fan-out → merge → record → probe → draft → **human read** → post.
**v0.1 stops at posting.** Whether the author addressed anything is outside what
cr can observe; re-review is v0.2.

### 1. Brief

```bash
cr brief 1 --issue CR-5 --intent-file issue.txt
```

Prints the orientation payload and opens round 1: PR identity, head and merge
base, the selected profile and the layer that chose it, the issue key and text,
recorded claims and drift, the units of the diff, files excluded by
`ignore.globs` and binary or generated files listed but not clustered, ingested
threads, notes and candidate notes, and the active, disabled and unavailable
axes with reasons.

```json
{
  "round": 1,
  "profile": {"id": "shop", "selected": true, "layer": "match.files"},
  "issue": {"key": "CR-5", …},
  "units": [{"id": "u1", "path": "order.go", "side": "RIGHT", "hunk_ranges": [{"start": 1, "end": 9}], "hash": "…", …}, …],
  "files": {"excluded": 0, "listed": []},
  "active_roles": ["convention", "correctness", "intent-coverage", "test-adequacy"],
  "honesty": []
}
```

Extract the claims from the issue text yourself — each a verbatim span — and
record them. `claims record` re-reads the issue, so it takes `--intent-file` too:

```bash
cr claims record 1 claims.ndjson --intent-file issue.txt
```

```json
{
  "recorded": [{"id": "CR-5#c1", "text": "A discount larger than the price yields zero.", "source": "acceptance", "span": "…", "span_hash": "…", "issue_hash": "…", "head": "…", "round": 1}],
  "round": 1
}
```

### 2. Review fan-out

The intent pass runs first; the other axes are refused until its mapping is
recorded.

```bash
cr review 1 --axis intent
```

```json
{
  "round": 1,
  "prompts": [{"role": "intent-coverage", "axis": "intent", "unit": "u1", "output": "~/.cr/state/acme/shop/pr-1/fanout/1/u1/review-intent-coverage.ndjson", "prompt": "# Intent coverage (intent-coverage) on unit u1 …"}, …],
  "honesty": ["lens test/symbols unavailable, per §4.5.4: cr built no symbol index for symbols.lang \"go\""],
  "skipped_roles": [],
  "expected_cells": [{"unit": "u1", "role": "convention"}, …]
}
```

Record the claim-to-unit mapping the intent pass produced (`{"claim": "CR-5#c1", "unit": "u1"}` per line), then emit the rest:

```bash
cr map record 1 pairs.ndjson
cr review 1
```

Spawn one sub-agent per prompt. Each writes its findings, one JSON record per
line, to the `output` path the prompt names, and nothing else. A role reports
every unit it looked at as a coverage cell:

```bash
cr cells record 1 cells.ndjson
```

```json
{
  "recorded": [{"unit": "u2", "role": "convention", "result": "pass", "unit_hash": "676014eb06ee8adb", "head": "…", "round": 2}, …],
  "round": 2
}
```

A cell is `{"unit", "role", "result": "pass"|"finding"|"question"|"na"}`; a
test-axis cell also carries `"coverage": {"classification": "covered"|"partially-covered"|"uncovered", "test_paths": [...]}`.
Do not supply `unit_hash`; cr writes it. A cell and the records of its role on
its unit must agree, in whichever order they arrive: a `pass` cell where that
role already holds records is refused with exit 1 ("this round holds record(s)
f10, f11 from that role on that unit; … file the cell as finding or question"),
and so is a record where that role filed `pass`.

A claim with no implementation is not a finding (there is no code to anchor it
to): it appears only in `cr status`. Take it out of scope with a note:

```bash
cr claims set-aside 1 CR-5#c1 --note CR-5#n1
```

### 3. Merge and record

```bash
cr merge ~/.cr/state/acme/shop/pr-1/fanout/1/u1/review-correctness.ndjson ~/.cr/state/acme/shop/pr-1/fanout/1/u1/review-test-adequacy.ndjson -o merged.ndjson --pr 1
cr record 1 merged.ndjson
```

`cr merge` binds each file's records to the role its file name carries,
deduplicates by anchored line and class, drops findings an active waiver or an
earlier posting already covers, and counts by role, axis, severity and grade.
`cr record` stores the round's records; it re-applies both drops, so recording
a role's file directly is safe too.

```json
{
  "recorded": [
    {"id": "f1", "kind": "question", "grade": "argued", "state": "draft", "axis": "correctness", …},
    {"id": "f2", "kind": "finding", "grade": "probed", "state": "draft", "axis": "test", …}
  ],
  "duplicates": 0, "suppressed": 0, "probes": [],
  "waived": {"dropped": 0, "waivers": []},
  "already_posted": {"dropped": 0, "posted": []},
  "honesty": []
}
```

A record the agent writes:

```json
{"id":"f2","kind":"finding","role":"test-adequacy","class":"untested-branch","severity":"high","unit":"u1","claim":"CR-5#c1","probe":"p1","anchor":{"path":"order.go","side":"RIGHT","start_line":5,"line":6},"summary":"No test exercises a discount larger than the price.","evidence":"Changing > to >= leaves the suite green."}
```

`grade`, `state`, `axis`, a citation's `content_hash` and `origin`, and `head` and
`round` are cr's to write; a record supplying one is refused with exit 1. cr also
refuses, with exit 1, a repeated record id, an anchor outside the unit the
record names, a `probe` naming no probe record or one from another head, and a
correctness record whose `claim` is not mapped to its own unit.

### 4. Probe

Probes run in a sandbox worktree at the round's head. The profile supplies
`tests.cmd`, `tests.count_pattern` and friends.

```bash
cr sandbox create 1
cr test 1
cr probe run 1 --kind mutation --patch mutation.diff --filter TestDiscount
```

```json
{
  "probe": "p1", "kind": "mutation",
  "command": ["./run-tests.sh", "-run", "TestDiscount"],
  "filter": "TestDiscount",
  "result": "no-test-failed",
  "establishes": "gap",
  "target": "order.go:5",
  "baseline": "r2", "run": "r3",
  "warnings": ["the probe lock covers cr's own runs only, per §5.6.3: …"],
  "honesty": []
}
```

A mutation probe derives its target from the patch. A gap probe places a test
you write and needs `--target`:

```bash
cr probe run 1 --kind gap --test gap_test.go --target order.go:5 --filter TestDiscountAboveThePrice
```

```json
{"probe": "p2", "kind": "gap", "result": "passed", "establishes": "behaviour", "target": "order.go:5", …}
```

Only a mutation probe's `no-test-failed` over a passing baseline establishes a
missing test. A gap probe's `failed` supports a finding only when its baseline
passed and the record's `claim` is mapped to its unit; `passed` shows the
behaviour is present and supports no `probed` grade. Reference a probe from a
record with `"probe": "p1"`; it supports that record only when its target lies
inside the record's RIGHT anchor range. `cr sandbox destroy 1` removes the
worktree.

### 5. Draft

```bash
cr draft 1
```

```json
{
  "path": "~/.cr/state/acme/shop/pr-1/rounds/1/draft.md",
  "round": 1, "queued": 2,
  "triaged": [], "retriaged": [], "preserved": [],
  "forced_to_question": [{"class": "negative-discount", "count": 1}],
  "new_classes": ["negative-discount", "untested-branch"],
  "warnings": []
}
```

The draft holds one block per record under a marker line, e.g.
`<!-- cr:record id="f2" kind="finding" path="order.go" start_line="5" line="6" severity="high" grade="probed" disposition="" -->`,
then the body you edit, then for a probed record a cr-owned evidence region (the
probe's kind, target, filter, result, input and output tail).

### 6. Human read

**cr cannot tell whether a human read the draft.** Nothing it can observe
separates a read-and-approved draft from an unopened one, so the human is the one
who must read every block before posting. Hand the draft to the user; do not post
on their behalf without that read.

`cr post` without `--confirm` validates and prints the exact payload, and is the
last check before the write. A question written as a statement is refused:

```json
{
  "error": "the body of record f1 is a kind=question body holding no \"?\" character, and §8.1.5 refuses to post a question written as a statement",
  "hint": "edit that record's body in the draft; §8.1.3 refuses an empty one and one carrying cr's own marker sequence"
}
```

Edit the body into a question in the draft; `cr draft 1` then reports it under
`"preserved": ["f1"]`.

#### Triage verbs

Triage is editing `draft.md`. `cr draft` and `cr post` read the file back; a run
of `cr draft` applies the triage and renders the draft again.

| In `draft.md` you… | Effect |
|---|---|
| leave a block unchanged | posted as rendered |
| edit the body prose | posted as edited (`preserved`) |
| change `kind="finding"` to `kind="question"` | softened, recorded as a triage event |
| delete the block entirely, marker included | discarded `not-here`; a **pull-request-scoped** waiver |
| set `disposition="wrong"` in the marker | discarded as a false positive, body or not; a **repository-wide** waiver that counts against the class |

**`wrong` and `not-here` are different decisions.** `not-here` means the finding
is true but not worth a comment on this pull request, the ordinary volume
decision, and never counts against the class. `wrong` means the finding is false;
it is the only signal that demotes a class. Never mark a true finding `wrong` to
make room.

After deleting f13's block, marking f12 `disposition="wrong"`, changing f14 to
`kind="question"` (rewording its body as a question) and f15's severity from
`high` to `medium`, `cr draft 1` printed:

```json
{
  "queued": 2,
  "triaged": [
    {"id": "f12", "outcome": "discarded-wrong", "counts_against_class": true},
    {"id": "f13", "outcome": "discarded-not-here", "counts_against_class": false},
    {"id": "f14", "outcome": "softened", "counts_against_class": true}
  ],
  "retriaged": [{"id": "f15", "severity": "medium"}],
  "preserved": ["f14"],
  …
}
```

and `cr waivers list --pr 1` shows the two scopes:

```json
{
  "scopes": ["repository", "pull-request"],
  "waivers": [
    {"id": "wr1", "path": "order.go", "class": "naming", "disposition": "wrong", "scope": "repository", …},
    {"id": "wp1", "path": "order_test.go", "class": "test-naming", "disposition": "not-here", "scope": "pull-request", …}
  ]
}
```

**Marker fields.** Only these edits are admitted; any other marker edit aborts
with exit 1 naming the draft line and the record:

| Field | Edit semantics |
|---|---|
| `id` | immutable; a changed or unknown id aborts (there is no manual-comment channel) |
| `kind` | `finding`→`question` softens; `question`→`finding` only when cr's grade is `probed` or `cited` |
| `path`, `start_line`, `line` | re-validated against the head; aborts when the anchor no longer resolves |
| `severity` | freely editable within `critical`, `high`, `medium`, `low` |
| `disposition` | only `wrong` by hand; `not-here` is cr's word for a deleted block |
| `grade` | informational; cr recomputes it and ignores the edit |

The refusals, as `cr draft 1` printed them (`hint` for all marker edits: "§7.2's
table is the whole of what a marker may be edited to; correct that record's block
in the draft"):

```text
draft line 41, record f15: kind asks for "finding" on a record cr graded "argued", and §6.3.3 admits that register only on "probed" or "cited"; leave it a "question" or give the record an experiment
draft line 11, record f12: kind reads "nit", and §6.1's register is "finding" or "question"
draft line 11, record f12: disposition reads "not-here", which cr writes itself when a block is deleted; delete the block to say it
draft line 11, record f12: disposition reads "maybe", and the one disposition §7.2 admits by hand is "wrong"; delete the block for the other
draft line 41, record f15: severity reads "urgent", and §6.1's four are critical, high, medium, low
draft line 11, record f99: id names no record this round rendered, and §7.2.3 gives v0.1 no manual-comment channel in the draft; restore the id cr wrote, and write a comment of your own on GitHub after posting
draft line 41, record f15: anchor runs to line 99 of "order.go", which holds 10 lines at the head under review
```

An edited `grade="probed"` on an argued record is accepted and rendered back as
`grade="argued"`. A marker missing a field or out of order is malformed, and both
`cr draft` and `cr post` refuse it with exit 1:

```text
draft line 11 is a malformed record marker: " grade=" does not follow, and §7.1.1 fixes the eight fields and their order
```

Fix the line the refusal names and run `cr draft` again.

### 7. Post

```bash
cr post 1
```

```json
{
  "round": 1,
  "comments": [{"id": "f1", "kind": "question"}, {"id": "f2", "kind": "finding"}],
  "payload": {"event": "COMMENT", "body": "…", "comments": [{"path": "order.go", "line": 8, "side": "RIGHT", "body": "…"}, …]},
  "forced_to_question": [{"class": "negative-discount", "count": 1}],
  "posted": false,
  "confirm_given": false
}
```

```bash
cr post 1 --confirm
```

```json
{"round": 1, "comments": […], "payload": {…}, "forced_to_question": […], "posted": true, "confirm_given": true}
```

**Every network write needs `--confirm`.** No setting, environment variable or
profile field makes it implicit. All comments go in one review. If a post's
outcome is unknown (a timeout, a dropped connection), `cr post 1 --reconcile`
matches the payload hash against the pull request's reviews instead of posting
twice.

```bash
cr status 1
```

```json
{
  "records": {"total": 2, "by_state": [… {"name": "posted", "count": 2} …], …},
  "completeness": {"complete": true, "reasons": []},
  "honesty": ["§9.3.1: round 1 was opened at head …, which is still the pull request's current head", "round complete, per §10.2", …]
}
```

A complete round is not an approval: cr never approves a pull request.

### A moved head

When the pull request's head moves, every command that writes per-PR state
refuses with exit 4:

```json
{
  "error": "§9.3.1: round 1 was opened at head e23991d… and the pull request's current head is aaaef61…; §9.3.2 refuses every write to per-PR state until the round moves with it: run `cr brief 1 --repo acme/shop`",
  "hint": "run `cr brief <pr>` to open the round the current head belongs to"
}
```

`cr brief 1` opens round 2: open records move to `stale`, the mapping is
cleared, units are recomputed, claims carry forward. Anchors are never migrated.
Re-run the loop for the new round.

## The argued rule

A record's grade is computed by cr, never asserted:

| Grade | Requirement |
|---|---|
| `probed` | references a probe at this head whose result supports it |
| `cited` | carries a citation cr resolved that lies outside the record's own unit, or has `origin: rule`, and the axis is not `test` |
| `argued` | neither |

**A record graded `argued` is posted as a question.** The forcing happens at
record, draft and post time, and **nothing overrides it**: no flag, config key,
environment variable, profile field or role instruction. A setting that addresses
it is refused (exit 3):

```json
{
  "error": "\"CR_FORCING_OFF\" is not a setting: it addresses the argued forcing of §6.3, which no configuration layer may supply; remove it",
  "hint": "§2.7 protects this decision from configuration; remove the variable or key the message names"
}
```

The draft reports the forcing per class under `forced_to_question`.

**When to probe instead of asserting.** If you believe a finding and want to
assert it, earn the grade: a claim that a branch is untested takes a mutation
probe; a claim that behaviour is wrong takes a gap probe with a mapped claim; a
claim about code elsewhere takes a citation. A record on the test axis can never
be `cited`, so test-adequacy findings assert only with a probe. If you cannot
establish it, let it be a question — that is the register cr exists to protect.

## Citations versus evidence

`citations` is the machine-readable evidence field; `evidence` is prose.

```json
"citations": [{"path": "internal/api/store.go", "line": 7}]
```

cr resolves every citation against the head (a path or line the head does not
hold is refused with exit 1), stamps its `content_hash` and `origin`, and grades
on citations and probes alone. **`evidence` prose is never parsed**: however it is
worded, a finding without citations or a supporting probe is graded `argued` and
posted as a question. A citation inside the record's own unit buys nothing, since
it points at the code the record is already about. A `cited` grade means only that
a human-checkable location was supplied, not that it supports the summary; every
citation is rendered verbatim for the human to judge.

## Comment economy

A review comment spends the reviewer's standing with the author, so volume is a
cost in itself.

- **Fewer comments.** `post.max_comments` (default 20) caps a round. Every
  `cr draft` header measures the queue against it:

  ```
  comments: 3 comments queued against post.max_comments 1, 2 over the cap
  ```

  Over the cap, `cr post` refuses with exit 1, with or without `--confirm`, and
  never drops a comment to fit; triage the draft down instead:

  ```json
  {
    "error": "3 comments queued against post.max_comments 1, 2 over the cap: triage the draft down to 1, or raise post.max_comments; cr will not drop 2 to fit",
    "hint": "discard records in the draft until the count is within `post.max_comments`, or raise the cap"
  }
  ```
- **Each anchored to the line it concerns.** Every posted comment sits on a line
  of the diff; there is no unanchored channel. An item with no code location, such
  as an unimplemented claim, is reported by `cr status` and never posted.
- Setting a true finding aside for volume is a `not-here` discard, scoped to the
  pull request, so it never silences the class repository-wide.

## Context and notes

```bash
cr note CR-5 "Negative discounts are validated upstream." --source chat --pr 1
cr context CR-5
cr answer 1 f1 "Upstream validation rejects negative discounts." --source thread
```

```json
{"note": {"id": "CR-5#n2", "text": "Upstream validation rejects negative discounts.", "source": "thread", "pr": 1, "record": "f1", "recorded_at": "…"}, "honesty": […]}
```

Notes are keyed by issue key and loaded on every later round and pull request
with that key. Sources are `chat`, `jira`, `thread`, `meeting`, `other`.
A note names the pull request it came from, so `cr note` without `--pr` is refused
with exit 2. `cr note --remove CR-5#n2` retracts one (the id is the only
argument) and prints it with `"standing": "retracted"`; an id the store does not
hold is refused with exit 1. `cr status` reports any cell or record citing a
retracted note as needing re-evaluation.

## Roles, profiles and rules

All three are JSON data files, never prompt code, and each file's stem is its
`id`. A malformed file aborts the command with exit 3 naming the file and field.

| Kind | Global | Per repository (wins) |
|---|---|---|
| role | `~/.cr/roles/<id>.json` | `~/.cr/repos/<owner>/<repo>/roles/<id>.json` |
| profile | `~/.cr/profiles/<id>.json` | selected by `profile` in `~/.cr/repos/<owner>/<repo>/config.json` |
| rule | `~/.cr/rules/<id>.json` | `~/.cr/repos/<owner>/<repo>/rules/<id>.json` |

### Roles

A role is a lens on one axis: persona and focus, while cr owns the output
contract. Fields: `id` (kebab-case), `title`, `axis` (`intent`, `correctness`,
`convention` or `test`), `instructions`, and optionally `focus` (questions
appended to the prompt) and `profiles` (empty means all). `cr init --eject-roles`
writes the four built-ins (`intent-coverage`, `correctness`, `convention`,
`test-adequacy`) as editable files. More than one role may serve an axis.

To add one, write the file:

```json
{
  "id": "money-safety",
  "title": "Money safety",
  "axis": "correctness",
  "instructions": "Look for arithmetic on monetary values that can lose or invent money.",
  "focus": ["Can this value go negative, and who would notice?"]
}
```

then run `cr brief 1` again: a round's active roles are settled by the brief, so
`cr review 1` emits prompts for the new role (`money-safety` on `correctness`)
only after it. A role naming an unknown axis:

```json
{
  "error": "…/repos/acme/shop/roles/money-safety.json: axis is \"money\", which is not an axis id; v0.1 has exactly intent, correctness, convention, test",
  "hint": "§1.5 closes the axis set at intent, correctness, convention and test; correct the axis field of the file the message names"
}
```

### Profiles

A profile is mechanical, language-specific configuration. Required: `id`,
`match.files` (marker files that select it; empty means never auto-selected),
`match.globs`, and `axes` (default on/off per axis). Optional: `sandbox.copy`,
`sandbox.setup`, `tests.cmd` (argv; absent disables the test axis), `tests.globs`
(required with `tests.cmd`), `tests.filter_flag`, `tests.timeout_seconds`,
`tests.output_tail_bytes`, `tests.count_pattern` and `tests.failed_pattern` (one
capture group each), `tests.probe_path_template`, `rules`, `symbols.lang`. cr
ships `laravel-pest` and `generic`.

```json
{
  "id": "shop",
  "match": {"files": ["go.mod"], "globs": ["**/*.go"]},
  "axes": {"intent": true, "correctness": true, "convention": true, "test": true},
  "tests": {"cmd": ["./run-tests.sh"], "globs": ["**/*_test.go"], "filter_flag": "-run", "probe_path_template": "cr_probe_<probe-id>_test.go"},
  "symbols": {"lang": "go"}
}
```

To add one, write `~/.cr/profiles/<id>.json`; `cr brief` reports the one selected
and the layer that chose it. The profile with the most matched marker files
wins. A tie is refused with exit 3:

```json
{
  "error": "profiles go-service, shop match the same number of marker files, and §2.4.2 forbids picking one of them; set `profile` in the per-repository config to the one this repository is",
  "hint": "the profiles the message names match equally well; give one of them a marker file the other does not have"
}
```

With `{"profile": "shop"}` in the per-repository config, `cr brief` reports
`"profile": {"id": "shop", "selected": true, "layer": "configuration"}`.

### Rules

A rule is one written standard. Required: `id` (kebab-case), `title`, `rationale`
(quotable to the author), `class`. Optional: `axis` (default `convention`),
`severity` (default `medium`), `kind` (default `question`), `detect`, `fix`
(`fix.replace` and `fix.with`, a regexp and its replacement producing a
suggestion), `globs`, `exempt`, `profiles`. Layers, highest first: per
repository, global, the profile's `rules`; a higher layer replaces a same-id rule
whole.

```json
{
  "id": "no-zero-floor",
  "title": "Do not silently floor a monetary value at zero",
  "rationale": "A floored total hides the input error that produced it.",
  "class": "zero-floor",
  "severity": "medium",
  "kind": "question",
  "detect": {"mode": "regex", "pattern": "return 0"},
  "globs": ["**/*.go"]
}
```

To add one, write the file and check it:

```bash
cr rules list
cr rules check 1
```

```json
{"repo": "acme/shop", "profile": "shop", "rules": [{"id": "no-zero-floor", "title": "…", "layer": "repo", "path": "…"}], "honesty": []}
```

```json
{
  "round": 2,
  "hits": [{"rule": "no-zero-floor", "path": "order.go", "line": 6, "text": "\t\treturn 0"}],
  "units": [{"unit": "u1", "hits": […]}, {"unit": "u2", "hits": []}],
  …
}
```

`detect` runs only over added and modified RIGHT-side lines, in Go `regexp`
syntax; a pattern that does not compile is refused with exit 3 (`detect.pattern
of rule "no-zero-floor" is not a Go regexp: …`). A hit is a hit, never a
verdict: it reaches the draft only when you confirm it by recording a record that
carries `"rule": "no-zero-floor"` and cites the hit's path and line. cr stamps
that citation `origin: rule`, so the record is graded `cited`:

```json
{"id": "f16", "rule": "no-zero-floor", "grade": "cited", "citations": [{"path": "order.go", "line": 6, "content_hash": "7e9a1237bd6a4ea7", "origin": "rule"}], …}
```

A rule without `detect` is injected into its axis role's prompt as text.

**A comment you have written by hand more than twice belongs in the rule
corpus**, not in a fourth comment: write it as a rule with its rationale, and
give it a `detect` block when the violation is mechanically matchable (a
pattern over the changed lines). `cr rules suggest` reports posted comment bodies
that recur `rules.harvest_min` times (default 3) as candidates; it never writes a
rule file:

```json
{"harvest_min": 3, "scanned": 2, "candidates": []}
```

## Inspection

```bash
cr waivers list
cr stats
cr rules list
cr rules check 1
cr rules suggest
cr config --resolved
```

`cr waivers list --pr 1` includes the pull request's own scope, and
`cr waivers remove <id>` removes one (an unknown id is refused with exit 1).
`cr rules list --dead` reports rules with no hit or record in the recent rounds.
`cr config --resolved` annotates every setting with the layer it came from.

## Global flags

| Flag | Effect |
|---|---|
| `--json` | force JSON output |
| `--compact` | minimal JSON (omits `output_tail`, `input`, thread bodies; never `evidence` or `citations`) |
| `--quiet` | suppress informational messages; honesty disclosures are always printed |
| `--no-color` | disable colour |
| `--repo <owner/repo>` | override repository detection |
| `-v`, `--version` | print the version |

## Exit codes

| Code | Meaning | Seen when |
|---|---|---|
| 0 | success | every step of the loop above |
| 1 | validation failure | a question body with no `?` at `cr post`; a marker edit §7.2 does not admit; a round over `post.max_comments`; an unknown waiver id |
| 2 | usage error | no detectable repository and no `--repo`; `cr note` without `--pr` |
| 3 | file, configuration or external command failure | a config key addressing the argued forcing; a malformed role or rule file; a profile tie |
| 4 | state conflict, lock timeout, partial post | a write after the head moved |

Every error carries a `hint` naming the next step.
