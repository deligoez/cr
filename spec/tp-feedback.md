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
