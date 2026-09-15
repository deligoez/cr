# cr

Code review lifecycle manager for AI coding agents.

> **Status: v0.2.1.** The release implements the contract in
> [`spec/0.2.0.md`](spec/0.2.0.md); its notes are in
> [`spec/0.2.1-release-notes.md`](spec/0.2.1-release-notes.md), and the upgrade
> notes for v0.1 state in
> [`spec/0.2.0-release-notes.md`](spec/0.2.0-release-notes.md).

`cr` reviews a pull request someone else wrote. It reads the intent from your
tracker, proves that every changed unit was examined, grades every finding by
the evidence behind it, and hands you a draft to edit before anything is posted.
v0.2 ends at posting: when the pull request's head moves, the round goes stale
and `cr brief` opens a new one. Following the conversation after posting, and
migrating anchors across a head change, is v0.3.

`cr` never calls a language model. It fetches, executes, validates and records;
the agent driving it forms the judgements. Read [`VISION.md`](VISION.md) for the
problems it is built to solve.

## What makes it different

- **Intent coverage, both ways.** Every changed unit maps to a tracker claim and
  every claim maps to changed code. An unmapped hunk becomes a question; a claim
  nothing implements is reported by `cr status`.
- **Uncertainty asks instead of asserting.** cr grades every record itself:
  `probed` (an experiment supports it), `cited` (it points at code outside its
  own unit), or `argued`. An `argued` record is posted as a question, and no flag,
  setting or environment variable changes that.
- **Findings carry experiments.** `cr` runs the suite in a throwaway worktree, a
  mutation probe breaks a line to prove a test gap, and a gap probe runs a new
  test against the head. Every probe reverts, even when the run fails or times out.
- **Coverage is proven.** Every unit times every active role is a filled cell,
  and a disabled or unavailable axis is reported with its reason.
- **You are still the reviewer.** `cr` writes a draft; you edit it. Deleting a
  block discards it for this pull request; marking it `disposition="wrong"`
  discards it repository-wide and counts against its class. A waiver holds while
  the anchored lines and the context lines around them are unchanged. Nothing
  reaches GitHub without `cr post --confirm`, all comments go in one review
  pinned to the round's head (`commit_id`), and a round posts at most one. A
  closed or merged pull request is disclosed before you confirm, never refused. Comment bodies follow `render.lang` (default `tr`); the
  review body is always English.
- **Conventions are data.** Project rules live in a versioned corpus, carry their
  rationale, and can ship their own fix as a ready suggestion. A comment body
  posted three times (`rules.harvest_min`) is reported as a candidate rule.

## Install

```bash
brew install deligoez/tap/cr                     # Homebrew formula
go install github.com/deligoez/cr/cmd/cr@latest  # or Go
```

`cr` needs `git`, `gh` authenticated through its own configuration under `HOME`
(`cr` does not pass `GH_TOKEN` to it), and a tracker command
(`intent.cmd`, default `jira issue view {key} --plain`) unless the issue text is
passed with `--intent-file`. State lives under `~/.cr/` (`CR_HOME` overrides it);
`cr` never writes inside the repository under review. Beside each round's
`rounds/<n>/summary.json`, `rounds/<n>/intake.json` holds the record ids and
§6.4.1 identities `cr merge` and `cr record` dropped, which the summary counts
without naming.

The Claude Code skill that teaches an agent the loop ships in this repository at
[`skills/cr/SKILL.md`](skills/cr/SKILL.md).

## The loop

```bash
cr init
cr brief 1 --issue CR-5 --intent-file issue.txt   # orientation payload, opens round 1
cr claims record 1 claims.ndjson --intent-file issue.txt
cr review 1 --axis intent                         # the intent pass runs first
cr map record 1 pairs.ndjson
cr review 1                                       # per-role, per-unit prompts
cr cells record 1 cells.ndjson
cr merge ~/.cr/state/acme/shop/pr-1/fanout/1/*/review-*.ndjson -o merged.ndjson --pr 1
cr record 1 merged.ndjson
cr sandbox create 1
cr probe run 1 --kind mutation --patch mutation.diff
cr draft 1                                        # edit the draft it names
cr post 1                                         # validate and print the payload
cr post 1 --confirm                               # the only network write
```

A record that names a probe is recorded after the probe runs; `cr record` may
run again in the round. If `cr post --confirm` exits 4 because the call's
outcome is unknown (a 5xx, a timeout, a dropped connection), run
`cr post 1 --reconcile` before anything else: until it adopts the review or
clears the flag, `cr status` and a `cr post` dry run report it, and
`cr post --confirm`, `cr draft` and a `cr brief` on a moved head are refused, because each could post the review twice or move records it may
already have posted.

## Commands

| Command | Purpose |
|---------|---------|
| `cr init [--eject-roles]` | Create the `~/.cr` tree and write the default profiles; `--eject-roles` also writes the built-in roles as editable files |
| `cr brief <pr> [--issue <key>] [--intent-file <path>]` | Orientation payload; opens a new round when the head moved |
| `cr claims record <pr> <file> [--intent-file <path>]` | Store the claims extracted from the issue |
| `cr claims set-aside <pr> <claim-id> --note <id>` | Mark an unimplemented claim out of scope |
| `cr review <pr> [--axis <id>]` | Emit per-role, per-unit prompts and output paths |
| `cr map record <pr> <file>` | Store the claim-to-unit mapping |
| `cr cells record <pr> <file>` | Store the coverage cells the roles filled |
| `cr merge <files...> -o <out> --pr <n>` | Merge and deduplicate per-role findings |
| `cr record <pr> <file>` | Record a round's merged findings |
| `cr sandbox create\|destroy <pr>` | Manage the probe worktree |
| `cr test <pr> [--filter <f>]` | Run the profile's test command inside the sandbox |
| `cr probe run <pr> --kind mutation\|gap ...` | Execute and record a probe (`--patch`, `--test`, `--target`, `--filter`) |
| `cr draft <pr>` | Render the editable draft and read back its triage |
| `cr post <pr> [--confirm] [--reconcile]` | Validate and post the review; resolve an unknown outcome |
| `cr answer <pr> <record-id> <text> [--source <s>]` | Store the answer to a posted question as a note |
| `cr note <ISSUE-KEY> <text> --pr <n> [--source <s>]` | Store an out-of-band fact |
| `cr note --remove <note-id>` | Retract a note |
| `cr context <ISSUE-KEY>` | Print accumulated notes with provenance |
| `cr rules list [--dead]` | Effective rules and the layer each came from |
| `cr rules check <pr>` | Run mechanical rule detection over the diff |
| `cr rules suggest` | Propose rules from recurring comment history |
| `cr waivers list [--pr <n>]` | List waivers, repository-wide and for a pull request |
| `cr waivers remove <id> [--pr <n>]` | Remove a waiver |
| `cr stats` | Triage statistics, demotion and volume candidates |
| `cr status <pr>` | Coverage, record states, and completeness |
| `cr config [--resolved]` | Effective configuration and the layer of each setting |

Global flags: `--json`, `--compact`, `--quiet`, `--no-color`,
`--repo <owner/repo>` (overrides detection from the clone's one GitHub remote),
`-v`/`--version`.

Exit codes: 0 success, 1 validation (including an input line that is not one
JSON object or gives a key twice), 2 usage, 3 file, configuration or external
command, 4 state conflict (a moved head, an unknown post outcome, a round
already posted, a lock timeout).

## License

MIT
