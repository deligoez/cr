# v0.1.0 — decision to leave spec review and begin implementation

`tp review spec/0.1.0.md --status --check` exits 1 and `converged` is `false`. The 233-task
decomposition was imported over that state with `tp import --force`. The decision was the operator's,
taken on the record below, and this file exists so that a reader who finds `clean: false` in
`state.json` can see what it was traded for.

## The numbers

| | |
|---|---|
| Review rounds | 13 |
| Findings, all rounds | 574 — 19 critical, 240 high, 251 medium, 64 low |
| Blocking (critical + high) by round | 46, 35, 37, 34, 35, 14, 17, 11, 7, 5, 4, 9, 5 |
| Round 13 | 7 findings, resolving to **3 distinct defects**, all in the two sections that round's own repair touched |
| Structural check | `speccheck.py` — 71 headings, 0 numbering breaks, 0 dangling references |
| Tasks imported | 233, over 59 of the spec's 72 headings; 13 context-only, 0 unmapped |
| Gate at import | `go test ./...` → ok, `golangci-lint run` → 0 issues |

## Why `converged` is false

The review never reached a counted clean round, and the reason is visible in the record rather than
hidden by it: **from round 10 on, each round's repairs produced the next round's findings.** Rounds
1–9 fell from 98 findings to 20 and from 46 blocking to 7. Rounds 10–13 then read 10, 30, 25, 7 —
and roughly two thirds of round 11's findings, and three of round 12's four distinct ones, were
defects in the repairs made after the preceding round.

The pattern is not noise. Sorted by what the repair did to the document:

| Repair shape | Rounds 10–12 outcome |
|---|---|
| Pure deletion of a clause | clean, every time |
| One precise added sentence | clean |
| Reworded cross-reference | clean where complete; twice a third occurrence was missed |
| **Added a record field** | produced a deadlock — the clause creating the baseline disqualified it |
| **Added a grading input** | moved a forgery path one field along rather than closing it |
| **Added a scoped exclusion** | dropped a severity ceiling the excluded clause also carried |

This is the same measurement CLAUDE.md already records for rounds 1–9, arrived at independently:
defect volume tracks the amount of new normative text, not the number of problems addressed. Three
consecutive rounds of "this repair is minimal and safe" were wrong three times, and each of the
three looked correct in isolation.

What made stopping defensible is the shape of round 13 rather than fatigue. Seven findings resolve
to three defects; all three sit inside the two-line repair that round applied; each has a stated,
one-clause fix; and no finding anywhere else in the document survived the round. That is a review
that has run out of spec to examine, not one still finding spec defects.

## What the review found that the repairs did not create

The strongest argument for having run rounds 10–13 is one defect that eleven earlier rounds missed.

§6.2.1 promises that a record's grade "MUST be computed by `cr` ... never asserted by the agent" —
the mechanical basis for invariant 4, principle P3, and the whole argued-becomes-a-question rule.
That promise was unenforced. The grading engine's inputs were never enumerated, so nobody could see
that they are fields the agent writes: `role` (from which `axis` derives), `unit` (which *is* the
containment boundary), and `anchor`. Naming the inputs explicitly is what made four independent
roles see it in the same round.

Two of the three are closed in the spec. The third — that validating a `unit` proves the unit exists,
not that it is the record's own — was found in round 13 and is carried in acceptance instead.

## Open findings, carried in task acceptance

Every surviving finding from rounds 8–13 is written into the acceptance criteria of the task that
will implement its section, named by finding class and round, so the implementation audit re-tests
it. Roughly 51 from rounds 8–9 and 30 from rounds 10–13. The three from round 13:

| § | Defect | Carried by |
|---|---|---|
| §6.1 | `cr merge` must reject a record whose `role` is not the emitting file's role, but §4.6.2 fixes no path form, so no path-to-role binding exists | `finding-required-field-validation` — pins the fan-out path to `review-<role-id>.ndjson` and derives `role` from the emitting file |
| §6.2 | A record may name a `unit` other than the one its `anchor` falls in, so a citation into its own hunk counts as outside its own unit and buys `cited` | `unit-containment-predicate` — rejects a record whose anchor does not fall inside the unit it names |
| §6.2 | The mapping is the first grading input mutable within a round, while the grade is recomputed three times | `grade-computation` — a recomputation may lower a grade, never raise it |

Carrying a finding in acceptance is weaker than repairing the spec: it binds the implementation but
leaves the normative text saying something the implementation contradicts. That is the cost paid,
and the audit's spec-coverage lens is where it comes due.

## Interpretations the implementation makes where the spec is open

§2.4.3 requires a malformed profile to abort with exit code 3 but never defines *malformed*
exhaustively. The `profile-schema` unit rejects a non-positive `tests.timeout_seconds` or
`tests.output_tail_bytes`, which the §2.4 table does not state. Kept deliberately, on the asymmetry
the trust economy asks for:

- Rejecting a legitimate value costs one rename, reported loudly with the field named.
- Accepting `timeout_seconds: 0` costs every run being killed at once, so every probe returns
  `timeout`, which §5.3.5 bars from supporting a `probed` grade — every test-adequacy finding falls
  to `argued` and posts as a question, permanently and silently. Accepting
  `output_tail_bytes: 0` empties the §8.1.7 evidence region, which is the thing that makes a
  `probed` assertion checkable by the author.

A loud wrong beats a silent one, and neither value has a reading under which it works: §5.2.3 and
§5.6 assume a finite kill, so `0` is not "no timeout" in this spec's model.

## Honest note on this decision

Rounds 10–13 cost 20 sub-agent runs and left the blocking count where round 10 found it. Judged on
convergence alone they failed. Judged on what they surfaced — the unenforced grading promise, and a
measurement of repair style precise enough to act on — they paid for themselves. Both readings are
true and the record should carry both.

The remaining risk is concentrated and named: three defects, all in §6.1 and §6.2, all in the
evidence chain that decides whether `cr` may assert to a colleague. If the implementation audit
finds this decision wrong, that is where it will find it.
