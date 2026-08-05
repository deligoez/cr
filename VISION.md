# cr — Vision

Code review lifecycle manager for AI coding agents.

`cr` turns a pull request review into a durable, auditable process: intent is
extracted from the tracker, every changed unit is provably examined, findings
carry graded evidence, the human edits before anything is posted, and the
conversation is tracked across force-pushes until every thread is closed.

---

## 1. Why this exists

Four things break in a real code review. No existing tool addresses them
together, and three of them are not bug-finding problems at all.

### 1.1 The intent is not in the pull request

Work is tracked in Jira. The author reads the issue, finds it incomplete, and
asks the product manager. The answer arrives in a chat window and goes straight
into the code — it is never written back to the issue.

The reviewer then sees code that does more than the issue asks, and has no way
to tell agreed scope from scope creep. A tool that mechanically checks the diff
against the issue will flag this every single time, and be wrong every single
time.

The fix is not a better checker. It is a second output channel: when the tool
cannot tell, it **asks** instead of asserting — and it **remembers the answer**,
so the same question is never asked twice.

### 1.2 Review output has a social cost

A review comment goes to a colleague. A wrong one costs trust, and trust is
spent once. This rules out any design where a model posts directly. The human
must read every word before it leaves the machine, and editing must be as cheap
as typing — not a sequence of CLI flags.

### 1.3 A finding without an experiment is an opinion

Experienced reviewers do not only read a diff. They check out the branch, run
the tests, break a line to see whether anything fails, and write a throwaway
test to prove a gap is real.

That converts a claim into a proof, and a proof is both more persuasive and far
less likely to be wrong. Almost every AI review tool skips this step entirely,
which is exactly why their output reads as plausible rather than true.

### 1.4 A review is a conversation that spans days

The author pushes, replies, argues, fixes half of it. Threads must be re-checked
against new code, anchors survive force-pushes, and some concerns are answered
in chat rather than on GitHub. None of this fits in one session or one context
window, so it has to live on disk.

---

## 2. What cr bets on

### 2.1 Intent is the authority

The tracker issue is the specification. Every changed unit must map to a claim
from that issue, and every claim must map to changed code. Both directions are
reported. This is the single check that catches the failures nobody else
catches: silent scope creep, and quietly unimplemented requirements.

### 2.2 Uncertainty routes to a question, never to a claim

Asking is socially free; asserting wrongly is expensive. So low-confidence
output is not suppressed, it is re-shaped. This preserves recall without paying
for it in trust.

### 2.3 Evidence grade decides the register

A finding is `probed` (an experiment backs it), `cited` (concrete code backs
it), or `argued` (only reasoning backs it). An `argued` finding is never posted
as an assertion — it is posted as a question. One rule, and it encodes the
judgement an experienced reviewer applies by instinct.

### 2.4 The reviewer stays the author of record

cr produces a draft. The human edits it in an editor, in prose, and deleting a
block is the discard verb. Nothing is posted without an explicit confirmation
step. cr is an exoskeleton for a reviewer, not a replacement for one.

### 2.5 Deleted findings never come back

Every discard becomes a waiver keyed by content, so the same false positive does
not resurface in the next round or the next pull request. Triage decisions also
accumulate into per-class statistics, which tell you which classes should stop
asserting and start asking.

### 2.6 Out-of-band context is captured, not lost

Answers that arrive in chat, in a Jira comment, or in another reviewer's thread
are recorded against the issue key with their provenance. Over time this becomes
the record of every decision that never made it back to the tracker — arguably
the most valuable thing cr produces, and a by-product.

### 2.7 Coverage is proven, not claimed

Every review unit times every active role is a cell that must be filled with
`pass`, `finding`, `question`, or `n/a` plus a reason. A review is complete when
the matrix is full — not when the model stops talking. Disabled axes are
reported as disabled, never silently skipped.

### 2.8 The tool never calls a model

cr emits prompts, executes commands, and records state. An agent does the
reading and the judging. This keeps cr deterministic, testable, cheap, and
independent of any model vendor.

### 2.9 Standards are data, and they compound

A project's conventions are not general knowledge. "Use `CarbonImmutable`, never
`Carbon`" is true in one repository and irrelevant in the next. So conventions
live in a rule corpus that is versioned per repository, next to the roles rather
than inside them.

Two things follow. A rule that can be matched mechanically becomes a cheap,
high-precision detector instead of an expensive judgement call, and it can carry
its own fix as a ready suggestion. And a comment you have written by hand three
times is a rule waiting to exist, so cr reports it as a candidate and the corpus
sharpens itself from your own review history.

---

## 3. What cr is not

- Not an autonomous reviewer. It never posts without confirmation.
- Not a linter or a static analyser. Mechanical checks belong in CI.
- Not competing on bug-finding recall. The differentiator is coverage,
  evidence, and an auditable conversation record.
- Not a Jira client. In v0.1 it reads the tracker through an existing CLI and
  never writes to it.

---

## 4. Relationship to tp

`tp` manages the spec-to-task lifecycle for work you are writing. `cr` manages
the review lifecycle for work someone else wrote.

cr borrows tp's proven ideas — the NDJSON finding contract, a project-owned role
corpus, honest convergence accounting, unit briefs, and "agent plans, tool
executes" — and shares **no code** with it. The semantics diverge too far
(a question channel, an external state of record, human triage, day-scale
asynchrony) for a common abstraction to serve either side well.

The relationship is one-directional and clean: **cr is developed using tp.**

---

## 5. Roadmap

| Version | Theme |
|---------|-------|
| v0.1 | Full reviewer loop: intent, four axes, probes, draft, post, re-review |
| v0.2 | Author side: ingest incoming review comments as a work list |
| v0.3 | Write context supplements back to the tracker; share the context store |
| v0.4 | Profile ecosystem beyond the first two profiles |
