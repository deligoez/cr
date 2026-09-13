# Feedback for tp, found while building cr

cr is built with tp, so cr exercises tp harder than most callers do. What this
file is for: recording a tp bug or gap the moment cr hits it, instead of quietly
working around it and losing the observation.

**This file is cr's, not tp's.** Nothing here is written into `../tp` — that is a
separate repository with its own agent working in it, and an edit there lands in
someone else's uncommitted tree. Carrying an entry over to tp is the user's call.

Each entry: what was observed, what it cost, and a proposed fix.

---

## 1. `set --workflow`: the project layer accepts a `quality_gate` write that can never take effect

**Observed** at tp v0.34.0, adding a `deadcode` step to cr's gate at 7 of 233
tasks.

`tp set --workflow quality_gate=...` refuses:

```
{"error":"quality_gate is not settable via tp set --workflow; it is authored
 only by tp init","code":2}
```

`tp set --workflow --project quality_gate=...` then **succeeds**, writes the
value into `.tp/config.json`, and reports it:

```
{"updated":{"quality_gate":"go test ./... && golangci-lint run && ./scripts/deadcode.sh"}}
```

But the value never resolves. `tp config --resolved` still reads:

```
"quality_gate": {"source":"override","value":"go test ./... && golangci-lint run"}
```

because the task file's `workflow` block — authored by `tp init` — is the
`override` layer and outranks `project`. The write is inert, and nothing says so.

**Cost.** Two separate ones, and the second is the sharper.

A project cannot change its gate after `tp init` through any supported path. That
matters because a gate legitimately grows: cr hit this adding `deadcode -test
./...`, which `golangci-lint`'s `unused` cannot cover since it skips exported
identifiers by design. Adopting such a step while the tree is still green is
nearly free; adopting it after two hundred tasks is a cleanup backlog. The only
route left was hand-editing the task file — which every other part of tp's
contract tells an agent never to do, and which then has to reproduce Go's
`encoding/json` escaping (`&` for `&`, non-ASCII literal) or it churns the
whole file. cr's first two attempts produced 480- and 52-line diffs for a
one-line change.

The refusal message is also incomplete rather than wrong: it says the field is
authored by `tp init`, which does not warn that a project-layer write will be
accepted and ignored.

**Proposed fix.** Refuse the project-layer write for the same field and the same
reason, so the two paths agree — an accepted write that cannot resolve is worse
than a refused one. Then either let `tp set --workflow quality_gate=...` edit the
task file's `workflow` block under the usual lock, or, if the gate is
deliberately immutable after init, extend the refusal hint to name the supported
route so the hand-edit is sanctioned rather than a contract violation.

A narrower alternative, if the layering is meant to stand: have `tp set
--workflow --project` warn whenever the value it just wrote is shadowed by a
higher layer, for any field. The same trap exists for every field the task file
happens to set.

---

## 2. `tp done`: `\bdeferred\b` refuses a closure reason that uses the Go keyword

**Observed** at tp v0.34.0, closing `mutation-probe-run`.

The closure reason described the revert as running "from a deferred call", which
is what the code does — `defer` is the Go statement the invariant rests on. `tp
done` refused it:

```
{"error":"closure verification failed: deferral is forbidden. Leave the task
 open or complete it","code":1}
```

`internal/engine/closure.go`'s `patDeferred` is `(?i)\bdeferred\b`, matched over
the whole reason with no context, so every use of the word is read as a promise
to do the work another time.

**Cost.** Small in minutes and larger in what it does to the evidence. The
refusal names no offending phrase, so the first response is to rewrite the whole
reason rather than one word; and the word is unavoidable in exactly the tasks
where it matters most — anything about cleanup, rollback, or resource release in
Go is described with `defer`. The reason that finally passed says "from a defer
it owns", which is worse English for the same fact, so the check made the record
less clear about the thing it was verifying.

**Proposed fix.** Two cheap ones, either alone would have avoided this.

Name the match in the error: "the reason says `deferred`, which reads as a
promise to finish the work another time" lets the author correct one word instead
of guessing. Every other forbidden-pattern refusal has the same gap.

Then narrow the pattern to the deferral sense — `\bdeferred (to|until|for)\b`,
or `\bdeferred\b` only when no code-ish neighbour (`defer`, backticks, a
`.go:`/`func` reference) sits near it. `will be done later` is already spelled as
a phrase for this reason; `deferred` is the one bare word in the list, and it is
also a common technical term.

## A task cannot leave `wip` except by being finished

`cell-computed-fields` has been `wip` since 2026-08-31, claimed by a unit that
ended without closing it and left nothing on disk. Three separate units have now
been handed it by `tp next --brief` as their first instruction, and all three had
to be told out of band to work something else. It carries no `claimed_at`, so
nothing distinguishes it from work in progress.

**And the orientation command is what claims it.** `tp next` is documented as
"Get/resume next task"; `--brief` changes what it prints, not what it does. A
fourth unit opened with `tp next --brief`, watched it flip
`waived-findings-dropped` from `open` to `wip` with `started_at` written, and had
to revert the task file by hand to undo it. `tp brief <id>` is the read-only
one — the help text says so — but a prompt that says "orient yourself first"
reaches for `next`, and every unit that is then steered to a different task
strands the claim it just took.

That closes the loop with the missing transition: the command an agent runs
*before it knows what it is working on* takes a claim, and nothing can give it
back. The two defects are individually reasonable and jointly produce a task file
that accumulates permanent `wip` entries at the rate agents are started.

There is no transition out. `tp set … status=open` refuses the field as managed
and names three commands; of those, `tp claim` is open→wip, `tp close` is
wip→done, and `tp reopen` refuses with *"cannot reopen: task is wip (must be
done)"*. So the only exits from `wip` are finishing the task or editing the task
file by hand, and the second is what this repository's own rules forbid.

That is fine when a unit always outlives its claim, and units do not: an agent
can be interrupted, run out of context, or be stopped. The state machine has no
edge for the case that actually happens.

**Proposed fix.** `tp release <id>` (or `tp claim --release`), wip→open,
recording who released it and when — the same shape as `tp reopen` but from the
other state. A `claimed_at` stamp on `tp claim` would also let `tp next` skip or
flag a claim older than a threshold, which is the cheaper half: it does not add a
transition, it just stops handing a stale claim to the next unit as if it were
theirs.

## `tp done` refuses a reason for deferral without naming the line

Measured 2026-09-12, twice in one session by different units. `tp done <id>
--reason-file <f>` rejected an otherwise accurate closure reason with a message
saying deferral is forbidden, and named no line, no word, and no position. The
reason files were four and five lines long and every line was criterion-shaped;
the offending word was a `defer`/`deferred` substring inside prose that was
describing what the task DID, not what it postponed.

The rejection itself is right and worth keeping — a closure that says "left to
later" is how a task closes without closing. What costs a round each time is
that the agent has to guess which line tripped it, and the usual repair is to
reword every line rather than the one.

Contrast with the sibling check in the same command: when the reason's line
count does not match the acceptance's part count, tp prints each part it found,
and the fix is mechanical. That is the shape this check needs.

**Proposed fix.** Name the line number and quote the matched word, the way the
part-count error already names each part. A secondary improvement: match on a
word boundary rather than a substring, so `deferred` in "the caller deferred the
close" is caught while a sentence that merely contains those letters is not.

## `tp done` counts a `./...` package pattern as a sentence end

Measured 2026-09-13 with tp v1.1.1 on `record-id-refusal-names-next-free`. The
acceptance had three `- ` lines, the second reading "finding.NextID has a caller
outside tests, and deadcode ./... no longer lists it." A reason file with one
line per acceptance line was refused on the part count: tp reported four parts,
splitting that criterion at the `.` of `./...`. The close went through once the
reason carried a line for each of the two fragments.

`./...` is the ordinary way to name every Go package, so it appears in gate
commands and acceptance criteria of any Go project using tp, and neither
fragment is a sentence the author wrote. The repair is mechanical once the
split is known, but the reason file ends up with a line per fragment rather
than per criterion, which is the opposite of what the one-line-per-part rule is
for.

**Proposed fix.** Do not end a sentence at a `.` that is followed by a
non-space character. That one rule covers `./...`, `go.mod`, `v1.1.1` and
`internal/cli.version`; skipping a `.` inside backticks also covers a quoted
path that ends a clause.

## `tp audit` gives a nested list's items the ids of the list that holds them

Measured 2026-09-13 with tp v1.1.1 on `tp audit spec/0.1.0.md --affected-from-tasks`,
round 1. The spec-coverage checklist held 731 items and **719 distinct
`item_id`s**. The twelve repeated ids are `list-28-1` through `list-28-7` and
`list-29-1` through `list-29-5`: §5.3.4 and §5.4.3 are numbered items that each
introduce a nested numbered ladder, and every rung of the ladder took the id of
the outer item with the same number. `list-28-4` names both §5.3's "The result
MUST be determined by the first matching rung" (spec line 686) and that ladder's
fourth rung (line 691).

`tp audit --merge` dedups by `role` + `item_id`, so twelve of the auditor's rows
collapse into the other twelve, and whichever survives the merge decides both
requirements' status. It bit in this round: the auditor recorded `list-29-5`
`PASS` for §5.4.3's fifth rung and, eight lines later, `PARTIAL` for §5.4.5 (a
severity bound `cr post` never re-checks). `tp audit --merge` on that one file
reported `"duplicates_removed": 9`, `merged_count` 109 of 118, and the merged
`list-29-5` row is the `PASS` — **the first row wins, and the finding is gone
from the round with nothing in the output saying a non-`PASS` row was dropped.**

**Proposed fix.** Number a nested list item under its parent (`list-28-4-1`), or
number every list item in document order. Either keeps ids unique; a check at
emission that refuses a checklist with a repeated id would have caught this one.

## `tp validate` passes a task whose `source_sections` names no heading

Measured 2026-09-13 with tp v1.1.1, on a scratch copy of `spec/0.1.0.tasks.json`
with one task's `source_sections` set to `["### 8.1 Comment rendering"]` — the
spec's heading is `### 8.1 Render contract`. `tp validate --json` exited **0**
with `"valid": true` and `"errors": 0`, and reported the entry only among the
warnings: `rule: "section-anchor"`, `source_sections entry "### 8.1 Comment
rendering" not found in spec`, with a correct "Did you mean" list.

The warning is accurate; the severity is the problem. A task whose only anchor
resolves to nothing is exactly the task CLAUDE.md's rule means by "a task with
neither anchor fails validation" — it is anchored to no section — yet it passes,
and a driver that reads the exit code (here `tp validate >/dev/null; echo $?`)
sees a clean file. The same run carried dozens of atomicity warnings, so one
anchor warning among them is easy to miss even when the output is read.

**Proposed fix.** Make a `source_sections` entry that matches no heading an error
when the task has no `source_lines` to fall back on, or at least exit non-zero
under a `--strict` flag, so the anchor check can gate a script.
