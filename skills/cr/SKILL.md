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

- **Fewer comments.** `post.max_comments` (default 20) is the round's comment
  budget. cr never drops a comment to fit it; every `cr draft` header measures
  the queue against it, and triage in the draft is how a round comes under it:

  ```
  comments: 0 comments queued against post.max_comments 1
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
| 1 | validation failure | a question body with no `?` at `cr post`; an unknown waiver id |
| 2 | usage error | no detectable repository and no `--repo` |
| 3 | file, configuration or external command failure | a config key addressing the argued forcing |
| 4 | state conflict, lock timeout, partial post | a write after the head moved |

Every error carries a `hint` naming the next step.
