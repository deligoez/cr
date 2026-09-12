// Package draft renders spec/0.1.0.md §7.1's editable draft.
//
// §7.1 has `cr draft <pr>` render every queued record into a single Markdown
// file at `rounds/<n>/draft.md`. That one file is the whole of P4's surface: cr
// drafts, the human edits, and §8.1.2 makes editing it the only input path for
// reader-facing prose. So what is written here is what the author eventually
// reads, and what §7.2 reads back as triage.
//
// Only queued records are rendered, and the state is asked of the record rather
// than of the caller. §9.1's table gives `cr draft` exactly one row into
// `queued`, from `draft`, so a record in any other state has already been
// decided about: a suppressed duplicate speaks through its representative
// (§6.4.3), a thread-suppressed record through the thread that already covers
// it (§3.5.4), a discard through the waiver it wrote (§7.2), and a stale record
// belongs to a head that has moved on (§9.3.4). Rendering any of them would put
// a decided record back in front of the reviewer as if it were open.
//
// Nothing here reads §6.1's table, computes a grade, or forms an opinion. It is
// handed the records the command settled and turns them into text.
package draft

import (
	"fmt"
	"strings"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// Render is §7.1's draft: every queued record as one block, in the order the
// records arrive, and nothing else. lang is §8.1.1's `render.lang`, which the
// §8.1.4 label of every question is built in for.
//
// The order is the caller's. §7.1 asks for every queued record and says nothing
// about arrangement, and findings.ndjson is in the order `cr record` stored it
// — which is `cr merge`'s output order, which is the role files' own. A sort
// invented here would be a reading order cr chose for the reviewer with nothing
// in the spec behind it.
//
// A round with nothing queued renders the empty string. That is a real answer
// rather than a missing one: the file is written either way, so a reviewer who
// opens it sees a draft with no blocks rather than a stale draft from the round
// before.
//
// A record whose initial body §8.1.3 refuses stops the whole draft, naming the
// record, and nothing is rendered. The body is drawn from fields the agent
// wrote, and a draft carrying a reserved sequence would be misread when it is
// read back — as a second marker, or as a region cr discards — so the refusal
// is made while the fault is still one field of one record. A question the
// label table has no row for stops it the same way.
//
// sources is what §8.1.6's provenance region needs from outside the records,
// and may be nil when the caller holds none: a record resting on no note and
// citing no rule's hit needs nothing from it.
//
// preserved are the agent regions §7.1.6 keeps from the draft being
// regenerated, keyed by record id, and may be nil on a first rendering. A
// record found there carries that region as its body instead of the one cr
// would render, and it is held to §8.1.3 exactly as a rendered one is.
func Render(
	queued []*finding.Finding, lang render.Lang, sources *Provenances, preserved map[string]string,
) (string, error) {
	blocks := make([]string, 0, len(queued))
	for _, record := range queued {
		rendered, err := block(record, lang, sources, preserved)
		if err != nil {
			return "", err
		}
		blocks = append(blocks, rendered)
	}
	return strings.Join(blocks, "\n"), nil
}

// block is one record's rendering: §7.1.1's marker introducing it, and §8.1.3's
// comment beneath, separated by a blank line so the marker reads as an
// introduction rather than as part of the prose.
func block(
	record *finding.Finding, lang render.Lang, sources *Provenances, preserved map[string]string,
) (string, error) {
	comment, err := commentOf(record, lang, sources, preserved)
	if err != nil {
		return "", err
	}
	return markerOf(record).String() + "\n\n" + comment.String() + "\n", nil
}

// commentOf is one record's §8.1.3 comment: the agent region cr renders from
// the record, and the owned regions that apply to it.
//
// The §8.1.4 label is asked of the record's kind as it stands here, after
// §6.3's forcing ran over it, and of nothing else. It opens every question —
// one the agent wrote as a question and one cr forced alike — because the
// reader cannot tell the two apart and the line is how they learn the register
// and the grade either way. A finding carries none.
//
// The §8.1.6 provenance region is asked of the record and of sources alike,
// for a finding and a question both: weak provenance is disclosed whatever
// register the record reaches the author in. So is §8.1.7's evidence region,
// which is asked of the grade rather than of the kind: a `probed` or `cited`
// record asserts on what it rests on, whichever register it is written in.
//
// The agent region is the preserved one when §7.1.6 kept it, and the rendered
// one otherwise. Only that region is taken from the draft: the owned regions
// are built here from the record every time, per §8.1.3.
func commentOf(
	record *finding.Finding, lang render.Lang, sources *Provenances, preserved map[string]string,
) (render.Comment, error) {
	agent, kept := preserved[record.ID]
	if !kept {
		agent = body(record)
	}
	comment := render.Comment{Body: agent}
	if err := render.ValidateBody(record.ID, comment.Body); err != nil {
		return render.Comment{}, err
	}
	disclosed, err := sources.of(record)
	if err != nil {
		return render.Comment{}, err
	}
	provenance, err := render.ProvenanceRegion(record.ID, disclosed)
	if err != nil {
		return render.Comment{}, err
	}
	comment.Provenance = provenance
	evidence, err := sources.evidence(record)
	if err != nil {
		return render.Comment{}, err
	}
	comment.Evidence = evidence
	if record.Kind != finding.KindQuestion {
		return comment, nil
	}
	label, err := render.QuestionLabelRegion(lang, record.Grade)
	if err != nil {
		return render.Comment{}, fmt.Errorf("record %s: %w", record.ID, err)
	}
	comment.Label = label
	return comment, nil
}

// body is the free-form Markdown region of §7.1.2, which the user may rewrite
// entirely, holding §8.1.2's initial body.
//
// It is free-form in the sense that matters: nothing downstream parses it, and
// §7.2's triage reads the marker rather than the prose, so a reviewer may
// replace every word of it without changing what the block means to cr.
//
// §8.1.2 has cr render it from the record's `summary` and `evidence`, in
// English, and then forbids cr to compose. Both hold because the two fields
// are placed rather than written: each is carried verbatim as its own
// paragraph, in the order §8.1.2 names them, with no word of cr's between or
// around them. §6.1.1 already keeps both in English, so the body is English
// without cr translating anything, and every sentence in it is one the agent
// wrote into the record. The rewrite into `render.lang` is the agent's, made
// by editing `draft.md` — the one input path §8.1.2 leaves for reader-facing
// prose.
//
// An empty field contributes no paragraph, so a record carrying no evidence
// renders its summary alone rather than a summary trailed by a blank line.
func body(record *finding.Finding) string {
	paragraphs := make([]string, 0, 3)
	for _, paragraph := range []string{record.Summary, record.Evidence} {
		if paragraph != "" {
			paragraphs = append(paragraphs, paragraph)
		}
	}
	if record.Suggestion != "" {
		paragraphs = append(paragraphs, suggestion(record))
	}
	return strings.Join(paragraphs, "\n\n")
}

// machineGenerated is §7.1.3's label for a suggestion `suggestion_origin: rule`
// marks, and it says what cr does not know rather than only where the text came
// from.
//
// §2.6.2.4 is the whole sentence: cr cannot establish that a generated
// replacement compiles, parses, or preserves behaviour. A label reading only
// "machine generated" leaves the reviewer to supply that themselves, and the
// register of this draft is P2's — what cr cannot establish is said, not
// implied. It is English here for the reason §6.1.1's summary is: §8.1.2 has
// the agent rewrite the body into `render.lang` before the author reads it.
const machineGenerated = "Machine generated from the rule's fix. `cr` cannot establish that this " +
	"replacement compiles, parses, or preserves behaviour."

// suggestion is §7.1.3's fenced `suggestion` block, labelled when the record
// carries `suggestion_origin: rule`.
//
// The fence is GitHub's, and the language tag is what makes the block a
// one-click replacement of the anchored lines rather than a quotation of them.
// §8.2 is what decides those lines are the right ones; nothing here re-reads
// them.
//
// The label is absent for a suggestion the agent wrote, and that asymmetry is
// the point. §2.6.2.4 asks for it of a machine-generated replacement alone, and
// a label on every suggestion would say nothing about any of them — the
// reviewer would stop reading it, which is the same as not printing it.
//
// It sits inside the agent's own region rather than in a `cr`-owned one, which
// §8.1.3 delimits and regenerates. That is §7.1's arrangement and not an
// oversight: the body is free-form, the agent rewrites it in `render.lang`, and
// the label travels with the block it describes. §8.1.6's provenance region is
// the enforced disclosure, and it is generated at render and post time from the
// same field this reads.
func suggestion(record *finding.Finding) string {
	block := "```suggestion\n" + record.Suggestion + "\n```"
	if record.SuggestionOrigin != finding.OriginRule {
		return block
	}
	return machineGenerated + "\n\n" + block
}
