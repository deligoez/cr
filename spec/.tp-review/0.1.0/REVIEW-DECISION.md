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

## Amendments after this decision

Carrying a finding in acceptance does not freeze the spec forever. Where a unit
could not satisfy its criterion without the normative text also saying so, the
repair was made and is recorded here, so this file stays an accurate account of
what the spec says versus what it said when the decision was taken.

- **§2.3.3, at `stamp-head-round-fields`.** Round 8's `head-round-stamping-unassigned`
  was carried in acceptance, but the criterion — `cr` owns `head` and `round`, and
  rejects a record supplying either — is a claim about the contract, not only about
  the implementation. Three lines were added to the item that already states the
  requirement, citing §6.1.4's existing computed-field rule rather than inventing a
  concept. `speccheck.py` clean.

CLAUDE.md now makes this the orchestrator's call rather than a unit's, and sends
anything larger than restating an existing rule in its own section to the user.

- **§2.4 and §5.2.1, at `ship-laravel-pest-profile`.** Shipping the first real
  profile found a defect thirteen review rounds could not: `tests.count_pattern`
  required "exactly two capture groups yielding the executed and failed test
  counts", and Pest cannot satisfy it. Its recap is generated by Collision's
  `writeRecap`, which omits any status whose count is zero — so an all-passing
  run prints `Tests: 3 passed` and the word `failed` never appears. A two-group
  pattern needs both groups to match, so the counts stayed undetermined exactly
  in the case §5.2.5 requires to pass: the baseline. Every alternative was
  checked against a real Pest 4.7.8 install and rejected — `--testdox` needs
  four groups across three mutually exclusive shapes, `--log-junit` splits
  errors from failures so a throwing mutation would report `failures="0"` and
  manufacture a `probed` grade, and the `--json` formatter was never merged.
  `--log-events-text` does print a root-suite total, which sharpened the
  diagnosis: the problem is not a missing total, it is that **a zero is never
  printed**. The repair splits the field in two — `tests.count_pattern` and
  `tests.failed_pattern`, each one capture group, each matched repeatedly and
  summed, with no match of the failed pattern meaning zero. One field added,
  one field's meaning changed. `speccheck.py` clean.

- **§11 row 1 versus §2.5.2, at `role-eject-command`. Open, not repaired.**
  §11's table row for `cr init` reads "Create `~/.cr`, write default profiles and
  roles", while the next row gives `--eject-roles` the job of writing the roles.
  Read literally, row 1 makes the flag a no-op and §2.5.4's built-in resolution
  layer unreachable after any `init`, since the global layer would always hold a
  copy. The implementation takes the flag-gated reading — the only one under
  which §2.5.2 and §2.5.4 both have effect — and pins it with
  `TestInitWritesNoRoleWithoutTheEjectFlag`. Nothing was changed in the spec:
  the criterion is satisfiable without a normative change, and §11's rows are
  one-line summaries of the clauses that own the behaviour. Carried here so the
  audit re-tests the reading rather than inheriting it silently.

- **§11 gives `cr record` no way to name its input's door, at `record-command`.
  Open, not repaired.** `finding.Source` requires the caller to say which door a
  file came through — it is what stops an agent-authored file carrying
  `duplicate_of`. But §11's row for `cr record` has no flag for it, so the
  command must choose one reading for every invocation. It passes `SourceMerge`,
  because §11's purpose line calls the input a round's merged findings and
  §6.5.1 has `cr record` apply `duplicate_of` from merge output. The consequence
  is stated rather than hidden: a hand-written file fed to `cr record` may carry
  `duplicate_of` and will be believed. Whether §11 wants a flag is a decision for
  the user, not the implementation.

- **§2.3's table names no sandbox baseline, at `sandbox-baseline-record`. Open,
  not repaired.** Round 8's `unhomed-state` requires the post-setup baseline to
  have a named row in §2.3's per-PR state table and to live outside the
  worktree, because the sandbox is a worktree of the repository §2.2 forbids
  writing into. The second half is satisfiable; the first is not, because the
  normative table has no such row. The implementation added it where the table
  is expressed in code — the constant block in `internal/state/prfiles.go` that
  `WriteStamped` routes against — and deliberately left it out of `prFiles`,
  which `EnsurePR` creates: an empty NDJSON file means "no records", while an
  absent baseline means "no sandbox has been prepared", which §5.1.6 must be
  able to tell apart. Carried here so the audit re-tests the reading rather
  than inheriting it silently.

- **§5.2.5 narrowed by a contaminated run, at `baseline-recording`. Accepted, no
  spec edit.** Round 12's `baseline-contamination` requires a `cr test` run whose
  sandbox was found dirty afterwards to be void as a baseline, and the only way
  to stop `passed: true` is a fourth clause on the verdict — which narrows
  §5.2.5's literal biconditional over three fields. Accepted because §5.1.7 sets
  the precedent in the same chapter: a cleanliness failure overriding a computed
  outcome another rule declares total. §5.2.5 describes a run of the head's code,
  and a contaminated run is not one. If §5.2.5 should say so in its own words
  that is a one-clause repair, deferred to the audit rather than taken now.

- **`passed` is not a baseline-resolution filter, proven on real data.** My own
  instruction to the unit said it was. §5.2.5's "so it cannot serve as a
  baseline" is a consequence clause: making it a resolution filter would render
  §5.3.5's and §5.4.4's explicit `passed: true` qualifiers vacuous, and would
  never converge — on the QA fixture's own `runs.ndjson`, a filtered mutation
  probe resolves to a run whose filter selected nothing and so did not pass, so
  a `passed` filter would perform a fresh filtered run before every probe for
  ever and resolve none. `Passed()` travels inside the resolved baseline instead.

- **§5.3.4's `no-tests-selected` rung is unreachable through the shipped
  profile, at `mutation-result-ladder`. Open, not repaired.** The rung needs a
  *known* executed count of zero, and the comment in `internal/probe` says the
  reader is owed the difference between "the filter selected nothing" and "the
  counts could not be read". With `laravel-pest` that difference is invisible:
  an empty selection makes Pest print `INFO No tests found.` with no count at
  all, `tests.count_pattern` never matches, and the ladder correctly answers
  `inconclusive`. Driving the rung for real needed a runner that reports the
  empty selection *as* a count. The ladder is right and the profile is right;
  what is unstated is that the rung is only reachable from a runner that prints
  a zero. Carried here rather than repaired — the audit should decide whether
  §5.2.1 or §5.3.4 owes that sentence.

- **A gap probe's id is reserved outside the lock it is written under, at
  `gap-probe-placement`. Accepted, no spec change.** §2.4 puts `<probe-id>` in
  `tests.probe_path_template`, so a gap probe must know its id *before* it runs,
  while §2.3.1 has the record written under the per-PR lock. The reservation is
  therefore read outside the lock it is later honoured under. The implementation
  adds `probe.IDTakenError`: the append re-checks the reserved id against the
  allocation and refuses with exit 4 rather than writing a duplicate §5.5.2
  reference. One error type and one exit mapping beyond the literal criterion,
  accepted because a silently ambiguous probe id is worse than a loud refusal —
  §5.5.2 has findings cite probes by id, so two records sharing one id make a
  citation point at two experiments.

- **§5.4.2's occupied-probe-path refusal is only reachable from a *tracked*
  file.** Round 12 scoped §5.1.6's leftover-artefact scan to untracked files, so
  an untracked file planted at the probe path is wiped by the recreation before
  the placement check ever runs. The refusal is therefore reachable only from a
  tracked file, or one `sandbox.copy`/`sandbox.setup` puts back. Both rules are
  right; the interaction is unstated, and its practical effect is that the exit-4
  path is narrower than §5.4.2 alone suggests. Recorded so the audit tests the
  reachable case rather than assuming the wider one.

This is what the implementation phase is for. Review reads what is written; only
running the thing reads what is reachable, and no reviewer had run Pest.
