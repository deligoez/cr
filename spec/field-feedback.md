# Field feedback

Observations from running a released cr against a real pull request, recorded as they arrive so each can
become a task, a roadmap item, or a documented decision. An entry states what was observed and by whom;
a suggestion is the reporter's, and the triage line is cr's. Nothing here is normative.

## FT-1 · tarfin-labs/backend#6233 (WB-3155), cr v0.2.1, laravel-pest profile

Reporter: a Claude Code session reviewing the pull request on the user's behalf, with nothing posted.
Shape: 81 files, ~9.8k added lines, a module move; 106 units, 4 roles, 25 claims, 57 mapping pairs.

### Batch 1 (2026-09-15, after the intent pass and the first record)

| # | Observation | Reporter's suggestion | Triage |
|---|-------------|-----------------------|--------|
| 1.1 | The issue links a Google Docs spec; cr read only the tracker text, and `honesty` says nothing about the link. A rule living only in the linked document can never become a claim. A local tp spec for the same key existed and was reachable only through `cr note`, with nothing prompting it. | `cr brief` lists URLs found in the issue text in `honesty`; a repeatable `--intent-file` or `intent.extra_sources`; `candidate_notes` surfaces a local spec file matching the issue key. | open |
| 1.2 | `jira issue view --plain` wraps at terminal width, pads with trailing spaces and appends ANSI footer lines; spans across a wrap are unwritable, and one span failed on a U+00A0 in the source. | Normalise whitespace, NBSP and ANSI both when storing the issue text and when matching a span, and hash the normalised form; or fetch the raw description. | open |
| 1.3 | 106 units × 4 roles = 424 prompts of 13.5–15k chars each. One sub-agent per prompt, as the skill says, is not feasible; the reporter batched 8 agents × ~13 units. The full issue text is repeated in every prompt (~40% of the bytes). | Document batching as the normal mode; `cr review --shard k/n` or `--units`; a brief-time warning above a units × roles threshold; one shared context file referenced by path. | open |
| 1.4 | A module move produced 6 LEFT units for deleted originals and separate RIGHT units for the destinations, ~40 cells spent confirming "moved, unchanged", although git reports the renames. | Rename-aware units: pair a rename's sides, review only the content delta, default cells to `na` when the delta is imports or namespace only. | open |
| 1.5 | Notes recorded after `cr review --axis intent` emitted its prompts never reach those prompts; an intent record then asked the question note n4 settles, and `cr record` accepted it silently. | Stamp prompts with a notes/claims hash; `cr record` or `cr status` reports records from a prompt whose hash is stale; `cr note` hints that emitted prompts are out of date. | open |
| 1.6 | `cr review` after the mapping re-emits all 424 prompts, including the 106 intent prompts whose cells are recorded; re-running the intent pass by mistake is easy. | Emit only unrecorded (unit, role) cells, or mark prompts `already_covered`. | open |
| 1.7 | The laravel-pest `tests.cmd` has no path, so `cr test` without `--filter` runs the whole suite; on this monolith that is tens of minutes and against the team's rule. `--filter` is name-only. | Positional test paths (`cr test --path …`, `tests.path_args`); confirm or warn with the discovered test count when neither filter nor path is given. | open |

Worked well: per-(role, unit) id blocks kept eight parallel writers collision-free; `cr record` graded all
five intent records `cited` because their citations lay outside the unit, as intended.
