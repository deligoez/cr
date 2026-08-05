# cr

Code review lifecycle manager for AI coding agents.

> **Status: pre-release.** The v0.1 contract is specified in
> [`spec/0.1.0.md`](spec/0.1.0.md) and nothing beyond the command skeleton is
> implemented yet. This README will grow with the implementation.

`cr` reviews a pull request someone else wrote. It reads the intent from your
tracker, proves that every changed unit was examined, grades every finding by
the evidence behind it, hands you a draft to edit before anything is posted, and
then tracks the conversation across force-pushes until every thread is closed.

Read [`VISION.md`](VISION.md) for the problems it is built to solve.

## What makes it different

- **Intent coverage, both ways.** Every changed unit maps to a tracker claim and
  every claim maps to changed code. Silent scope creep and quietly dropped
  requirements both surface.
- **Uncertainty asks instead of asserting.** A finding backed only by reasoning
  is posted as a question, never as a claim. Asking is free; being wrong is not.
- **Findings carry experiments.** `cr` runs the suite in a throwaway worktree,
  breaks a line to prove a test gap, or runs a new test to prove an edge case is
  unhandled. A proven finding is not a plausible one.
- **You are still the reviewer.** `cr` writes a draft; you edit it in your
  editor; deleting a block waives it forever. Nothing reaches GitHub without an
  explicit confirmation.
- **Conventions are data.** Project rules live in a versioned corpus, carry their
  rationale, and can ship their own fix as a ready suggestion. A comment you have
  written by hand three times is reported as a candidate rule.

## Install

```bash
brew tap deligoez/tap && brew install cr    # once released
go install github.com/deligoez/cr/cmd/cr@latest
```

## License

MIT
