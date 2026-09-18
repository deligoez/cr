# Measurement 3 — cr against a human review

2026-09-18. ROADMAP.md "Next 1, Measure the bet", second measurement. Subject: `tarfin-labs/backend#3757`
(WB-800, January 2025, before agents commented on this repository), a pull request one reviewer read
carefully and left **25 root line comments** on, 23 of which the author answered and fixed. Merged, so the
discussion is closed. Nothing was posted, no test or probe ran, and the repository was read through a
detached worktree that is byte-clean after every batch.

## Setup

- **Two runs, because the head moved under the review.** 22 of the 25 comments are outdated at the merged
  head: the author fixed them. Run **A** reviews the merged head `59b553ce` (12 files, 359 lines, 22 units)
  and answers "what does cr find in code a careful human review already passed?". Run **B** reviews
  `3cef86cb`, the commit 16 of the 25 comments were written against (10 files, 263 lines, 19 units), and is
  the comparison proper. Run A was built first by mistake — the error is kept because the run it produced
  answers its own question.
- **The human's comments are the answer key, so cr must not see them.** cr ingests review threads and
  attaches them to the units it reviews (§3.5.3). A `gh` shim answers the one query naming `reviewThreads`
  with an empty page, passes every other read through to the real gh, and refuses any call carrying
  `--method`, `-X` or `--input`; run B's shim also rewrites `headRefOid`/`baseRefOid` to the reviewed
  commit and its merge base. Both briefs report **0 threads ingested**. Every invocation is logged with its
  verdict (`gh.log`, `ghb.log`).
- **Intent** is the pull request body's WB-800 changelog, six claims extracted mechanically. The tracker
  itself was not reachable; §3.3's "produced by the agent" is departed from on purpose, as in measurement 1,
  so the number reads as *given perfect claims*.
- Profile `laravel-pest` by configuration, so all four roles are active — the human's comments are mostly
  about tests, and dropping the test lens would have removed the lens that matters most here. No sandbox is
  created and no probe can run, so an unestablished suspicion stays a question.
- Roles: Opus, one `claude -p` session per prompt from the worktree, tools limited to reading it plus
  writing the prompt's own output. The memory plugin is off, and after it leaked into a batch twice the
  runner now **refuses to start** unless `~/.honcho/config.json` has `hosts.claude_code.enabled: false`.

## Result, run B (the comparison)

Ground truth: the **16** comments written against `3cef86cb`. The other nine were written on later commits,
against code that did not exist yet at this head, and are outside the set.

| | |
|---|---|
| cr records | 51 — 9 findings, 42 questions; grades cited 32, argued 19, probed 0 |
| prompts / cost | 78 Opus prompts, $81 |
| **human comments cr also raised** | **8 of 16** |
| human comments cr missed | 8 of 16 |
| cr-only records | 43 |
| **false assertions among cr's 9 findings** | **0 of 9**, each checked by hand against the reviewed tree |
| transcript audit | 3 of 78 voided (tool-allowlist breaches, no answer-key access) |

**The eight cr also raised**, with cr's record beside the comment:

| | the human | cr |
|---|---|---|
| H9 | "why is this needed?" on the `timestamps = false` toggle | f3001: disabling timestamps on the relation's model instance has no effect on the row `decisions()->create()` inserts, so the line is dead |
| H10 | "is this needed? the merge above already overwrites those dates" | f1102: the toggle plus the post-create re-save rebuild what Eloquent already does |
| H13 | "this filter isn't needed; rejected status is enough" | f1401: both tests build the negative fixture from `waitingApplicationStatusesList()`, the provider belonging to another endpoint |
| H14 | "returning the count isn't enough; assert the right applications come back" | f7102: the positive test asserts only the number of elements and never which applications came back |
| H17 | name it `it_doesnt_send_...` | f1601: named `it_not_sends_...` while the suite names negatives `it_does_not_...` in 218 places |
| H19 | "if you used `assertNothingSent` this isn't needed" | f1602: `assertNothingSent()` already proves it, so the `assertNotSentTo` block cannot fail and its callback never runs |
| H20 | name it `it_doesnt_send_...` (retailer) | f1802: same, for the retailer test |
| H22 | `assertNothingSent` redundancy (retailer) | f1801: same, as a finding |

**The eight cr missed** are H1 (two variable declarations used once), H8 (`$decision` → `$specification`),
H11, H12, H15, H18, H21 (a naming suggestion and the repeated "extract one `rejectedWithReason()` helper"),
and H6 (the last decision may belong to a guarantor, so filter by subject here). Seven of the eight are
*shaping* comments — name it this way, extract this helper — which is a reviewer telling an author how the
team writes code, not a defect. H6 is the exception: a real correctness concern cr came at from a different
angle (f2701, below) without naming the guarantor case.

**cr found, at this commit, what the human found two commits later.** H7 (`use latestOfMany()`) was written
against `868020bf64`; cr's f801 says it at `3cef86cb`: `lastDecision()` constrains the has-one with
`->latest()` instead of `->latestOfMany()`. And f2701, a finding, carries the consequence the human did not
state: because the relation is an ordered `hasOne` rather than a one-of-many, the `whereHas` in the new
endpoint matches *any* decision of the application rather than the last one — verified: `hasOne(Decision::class)->latest()`
at Application.php:1060, and an `EXISTS` subquery ignores the ordering. H4, the `retailer_user_id` versus
`retailer_id` bug the human found at `697bf737`, is not in cr's set either, but f5801 says at this commit
that nothing would notice if the retailer scoping were removed, "so a cross-retailer data leak on this
endpoint would ship green".

**The nine findings, all verified true by hand** against the reviewed tree: two unused imports that
`.php-cs-fixer.php` forbids (`no_unused_imports => true`; `ApplicationDecision` occurs exactly once in
ApplicationFactory.php, the import itself, and `SendRejectionNotificationToFarmerAction` once in the
retailer test); the `where(function …)` wrapper around a single `whereHas` that groups nothing; the
ordered-`hasOne` relation above; the two `assertNotSentTo`-after-`assertNothingSent` blocks; and the two
test-discrimination findings — no fixture in either file is rejected under a specification other than
`OtherReasonsRejectionSpecification`, confirmed by grep, so the type predicate that defines the endpoint is
never exercised.

## Result, run A (the merged head)

51 role prompts over 22 units, $99; **47 records — 11 findings, 36 questions**, cited 30, argued 17. This is
code that a careful reviewer had already read, commented on 25 times, and approved after the author fixed
everything. cr still found, among others: the `last_decision` array passed to `whenLoaded` as a literal, so
`$application->lastDecision->id` is dereferenced before the guard can protect it and every application with
no farmer decision would throw; the `subject = Farmer` filter applied outside `latestOfMany()`, so
`lastDecision` resolves to null when the newest decision is a guarantor's — which is the human's H6, found
in the *fixed* code; two `@see` annotations naming a class that does not exist; and a changelog announcing a
factory method the pull request does not ship. Those are unverified beyond reading and are listed here as
what a second pass over reviewed code produces, not as confirmed defects.

## Reading

Against a careful human reviewer on the same code, cr **recovered half the review** (8 of 16) and asserted
nothing false (0 of 9 findings). What it recovered and what it missed split cleanly by kind: every
dead-code, redundant-assertion and discrimination point was found, and nearly every *shaping* point — name
this test that way, extract this helper, call this variable `$specification` — was missed. That division is
not a defect of the roles; it is what "the tracker issue is the specification" (P1) buys and does not buy. A
team's house style is not in the issue and not in the diff, and cr has a place for it that this measurement
did not use: §2.6's rule corpus, where a convention is written once with a rationale quotable to the author.
Six of cr's eight misses are rules that corpus would hold.

Against the trust economy's own question the answer is the one that matters: **43 records the human never
wrote, and zero wrong assertions among the nine that assert.** The 42 questions are the register cr exists
to protect — cr asked where it could not establish, and the one place it could establish something the human
had not (the ordered `hasOne`) it stated as a finding and was right.

Beside measurement 1, the picture is now: against defects a QA pass found by driving commands, reading roles
score 0 of 24; against a human reviewer reading the same diff, they score 8 of 16 and add 43 more
observations without a false assertion. The instrument matches the defect: reading finds what reading finds.

## Files

`ground-truth.json` (the 25 comments with their original lines and commits), `human-comments.json`, the two
shims, `run-roles.sh` and `run-roles-b.sh` (with the memory-plugin precondition), `audit.py`/`audit-b.py`,
`b-refused.json` and `b-stripped-probe-fields.json` (what cr refused and what the orchestrator unlinked),
`role-logs/` and `role-logs-b/` (one transcript per prompt), `home/` and `home-b/` (the two rounds' state).
