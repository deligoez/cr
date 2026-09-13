package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// section103Counts are the counts §10.3 requires every round to record, spelled
// here as the keys a reader of summary.json finds them under, and in the order
// §10.3 lists them — plus round 8's missing-summary-count, §9.3.6's
// already-posted drop.
//
// They are written out rather than read from summaryOwners, because the test
// is whether that table covers the section: a list derived from the table would
// agree with it by construction and prove nothing.
var section103Counts = []string{
	"raised", "deduplicated", "suppressed_by_thread", "waived", "forced_to_question",
	"drafted", "posted", "discarded_not_here", "discarded_wrong",
	"comments", "probe_cap", "payload_hash",
	"already_posted",
}

// strictly decodes one section into the shape given, refusing a field the shape
// does not name, so a section holding more than the shape says fails as surely
// as one holding less.
func strictly[T any](raw json.RawMessage) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var held T
	return decoder.Decode(&held)
}

// payloadHashShape is §8.3.3's payload hash as §1.4's normalised hash writes
// it: sixteen lowercase hexadecimal digits.
var payloadHashShape = regexp.MustCompile(`^[0-9a-f]{16}$`)

// summaryShapes is the JSON shape each section of summary.json is stored in,
// stated here independently of the types the writers encode.
//
// A shape read back through the writer's own struct would agree with the
// writer by construction, and the file is what §10.3 promises a reader can
// reconstruct the review's history from — so the reader's view is the one the
// assertion takes.
func summaryShapes() map[string]func(json.RawMessage) error {
	type cap struct {
		Count int `json:"count"`
		Max   int `json:"max"`
	}
	count := func(raw json.RawMessage) error { return strictly[int](raw) }
	return map[string]func(json.RawMessage) error{
		"raised":               count,
		"deduplicated":         count,
		"suppressed_by_thread": count,
		"drafted":              count,
		"discarded_not_here":   count,
		"discarded_wrong":      count,
		"posted":               count,
		"waived": func(raw json.RawMessage) error {
			return strictly[struct {
				Dropped int      `json:"dropped"`
				Waivers []string `json:"waivers"`
			}](raw)
		},
		"already_posted": func(raw json.RawMessage) error {
			return strictly[struct {
				Dropped int      `json:"dropped"`
				Posted  []string `json:"posted"`
			}](raw)
		},
		"forced_to_question": func(raw json.RawMessage) error {
			return strictly[[]struct {
				Class string `json:"class"`
				Count int    `json:"count"`
			}](raw)
		},
		"confirm_given": func(raw json.RawMessage) error {
			if string(raw) != "true" {
				return fmt.Errorf("%s is not true: §8.5.4's fact is written only once --confirm was given", raw)
			}
			return nil
		},
		"new_classes": func(raw json.RawMessage) error { return strictly[[]string](raw) },
		"comments":    func(raw json.RawMessage) error { return strictly[cap](raw) },
		"probe_cap":   func(raw json.RawMessage) error { return strictly[cap](raw) },
		"payload_hash": func(raw json.RawMessage) error {
			var hash string
			if err := json.Unmarshal(raw, &hash); err != nil {
				return err
			}
			if !payloadHashShape.MatchString(hash) {
				return fmt.Errorf("%q is not a §1.4 normalised hash", hash)
			}
			return nil
		},
		"merged_hash": func(raw json.RawMessage) error {
			var hash string
			if err := json.Unmarshal(raw, &hash); err != nil {
				return err
			}
			if !payloadHashShape.MatchString(hash) {
				return fmt.Errorf("%q is not a §1.4 normalised hash", hash)
			}
			return nil
		},
	}
}

// assertSummaryShape holds one round's summary.json to its shape: every
// section it holds is one summaryOwners names and decodes into the shape stated
// for it, no section holds null, and every section the given writers own is
// present. It returns the document for the caller's own assertions.
func assertSummaryShape(t *testing.T, body []byte, ran ...summaryOwner) map[string]json.RawMessage {
	t.Helper()
	var document map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &document), "summary.json is one JSON object")
	shapes := summaryShapes()
	for key, raw := range document {
		_, owned := summaryOwners[key]
		assert.Truef(t, owned, "summary.json holds %q, which no writer of §10.3 owns", key)
		shape, stated := shapes[key]
		if !assert.Truef(t, stated, "summary.json holds %q, whose shape nothing states", key) {
			continue
		}
		assert.NotEqualf(t, "null", string(raw), "%q is written as null", key)
		assert.NoErrorf(t, shape(raw), "%q does not hold the shape stated for it", key)
	}
	for key, owner := range summaryOwners {
		if slices.Contains(ran, owner) {
			assert.Containsf(t, document, key, "%s ran, and %q is one of its counts", owner, key)
		}
	}
	return document
}

// §10.3's writer list, as round 8's unassigned-writer and missing-summary-count
// left it: every count the section requires has a named writer, and the two the
// round found unowned or merged are where the findings put them.
func TestEverySection103CountHasANamedWriter(t *testing.T) {
	writers := []summaryOwner{ownerMerge, ownerRecord, ownerDraft, ownerPost}
	for _, key := range section103Counts {
		owner, named := summaryOwners[key]
		if assert.Truef(t, named, "§10.3 requires %q and no writer is named for it", key) {
			assert.Containsf(t, writers, owner, "%q is given to %q, which is not one of §10.3's writers", key, owner)
		}
		assert.Containsf(t, summaryShapes(), key, "%q has a writer and no stated shape", key)
	}

	assert.Equal(t, ownerRecord, summaryOwners["deduplicated"],
		"round 8's unassigned-writer: `cr record` stamps `duplicate` from `duplicate_of`")
	assert.Equal(t, ownerRecord, summaryOwners["suppressed_by_thread"],
		"round 8's unassigned-writer: `cr record` writes §3.5.4's `suppressed`")
	assert.NotEqual(t, summaryWaived, summaryAlreadyPosted,
		"round 8's missing-summary-count: §9.3.6's drop is a key apart from §6.4.4's")
	assert.Equal(t, ownerMerge, summaryOwners["already_posted"])
	assert.Equal(t, ownerPost, summaryOwners["payload_hash"])

	for key := range summaryOwners {
		assert.Containsf(t, summaryShapes(), key, "summaryOwners lists %q with no stated shape", key)
	}
}

// A writer is refused a count it was not given, one the table does not list,
// and the same count twice — and a refused write touches nothing.
func TestTheSummaryWriterTableIsEnforcedAtTheWrite(t *testing.T) {
	layout := draftedHome(t)
	held, err := layout.LockPR(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	defer func() { require.NoError(t, held.Unlock()) }()

	for name, attempt := range map[string]struct {
		owner  summaryOwner
		counts []summaryCount
		says   string
	}{
		"another writer's count": {
			ownerDraft, []summaryCount{{key: summaryPosted, value: 1}}, "gives \"posted\" to cr post",
		},
		"a count §10.3 does not list": {
			ownerMerge, []summaryCount{{key: "reviewed", value: 1}}, "not one of §10.3's",
		},
		"one count twice": {
			ownerRecord,
			[]summaryCount{{key: summaryDeduplicated, value: 1}, {key: summaryDeduplicated, value: 2}},
			"twice",
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := writeSummary(held, draftRound, attempt.owner, attempt.counts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), attempt.says)
		})
	}

	_, statErr := os.Stat(layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileSummary))
	assert.ErrorIs(t, statErr, os.ErrNotExist, "every refusal comes before anything is written")
}

// §10.3 end to end over the three writers that run before posting: `cr merge`
// creates summary.json, `cr record` and `cr draft` each add their own counts,
// and the document that results holds every one of their sections in the shape
// stated for it — with the values the fixture makes checkable.
//
// The two role records share an anchor and a class, so §6.4.3 retires one as a
// duplicate, and a waiver takes a third out before anything is recorded: every
// count on the way from raised to drafted moves.
func TestTheRoundSummaryCarriesEveryPrePostWritersCounts(t *testing.T) {
	layout := gradedHome(t)
	// The hash cr stamps on aGradedRecord's anchor: line 3 of app.go at the
	// fixture's head. A waiver keyed on the hash the role typed matches nothing.
	stamped, err := finding.AnchorContentHash([]string{"func Retry() { backoff() }"})
	require.NoError(t, err)
	waiver := finding.Waiver{
		WaiverKey: finding.WaiverKey{
			Path: "app.go", Side: "RIGHT", Class: "long-function", ContentHash: stamped,
		},
		Disposition: finding.DispositionWrong,
	}
	_, err = finding.Waive(layout, fixtureOwner, fixtureProject, &waiver,
		finding.WaiverProvenance{Round: 2, PR: fixturePRNumber, Head: "0a1b2c3"})
	require.NoError(t, err)

	waived := aGradedRecord("f3")
	waived["class"] = "long-function"
	dir := t.TempDir()
	file := writeFanOut(t, dir, "correctness", aGradedRecord("f1"), aGradedRecord("f2"), waived)
	out := filepath.Join(dir, "merged.ndjson")

	_, err = runCLIPrinting(t, "merge", file, "-o", out, "--repo", fixtureSlug, "--pr", fixturePR)
	require.NoError(t, err)
	_, err = runRecord(t, fixturePR, out, "--repo", fixtureSlug)
	require.NoError(t, err)
	_, err = runDraft(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)

	body, err := layout.ReadRound(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileSummary)
	require.NoError(t, err)
	document := assertSummaryShape(t, body, ownerMerge, ownerRecord, ownerDraft)

	for key, want := range map[string]string{
		"raised":               "3",
		"deduplicated":         "1",
		"suppressed_by_thread": "0",
		"drafted":              "1",
		"discarded_not_here":   "0",
		"discarded_wrong":      "0",
	} {
		assert.JSONEqf(t, want, string(document[key]), "%q", key)
	}
	assert.JSONEq(t, `{"dropped":1,"waivers":["`+waivedID(t, layout)+`"]}`, string(document["waived"]))
	assert.JSONEq(t, `{"count":1,"max":20}`, string(document["comments"]),
		"§1.6.2's count against post.max_comments, whose built-in default is 20")
	assert.JSONEq(t, `{"count":1,"max":10}`, string(document["probe_cap"]),
		"§5.6.4's cap over the round: gradedHome records one probe in round 2, and the default cap is 10")
}

// waivedID is the id of the one repository-wide waiver the fixture wrote.
func waivedID(t *testing.T, layout state.Layout) string {
	t.Helper()
	active, err := finding.ActiveWaivers(layout, fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.Len(t, active, 1)
	return active[0].ID
}

// §10.3's finalisation: `cr post --confirm` writes the posted count and §8.3.3's
// payload hash, and the hash is the one the review that was sent embeds.
func TestConfirmFinalisesTheRoundSummary(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"), aCitedRecord("f2"))
	redraft(t)
	payload := builtPayload(t)
	ghShimming(t, payload)

	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)

	body, err := layout.ReadRound(draftOwner, draftRepo, draftPRNum, draftRound, state.FileSummary)
	require.NoError(t, err)
	document := assertSummaryShape(t, body, ownerDraft, ownerPost)
	assert.JSONEq(t, "2", string(document["posted"]), "§9.1 moved both queued records to posted")
	hash, err := payload.Hash()
	require.NoError(t, err)
	assert.JSONEq(t, `"`+hash+`"`, string(document["payload_hash"]),
		"§10.3 records the §8.3.3 hash of the payload that was sent")
	assert.JSONEq(t, "true", string(document["confirm_given"]),
		"§8.5.4: the round summary records that --confirm was given")
}

// Round 8's non-idempotent-accumulation at the write: a writer that leaves one
// of its own counts out is refused, because a section is replaced whole on
// every run and the count it left out would keep an earlier run's value.
func TestAWriterMustReplaceItsWholeSection(t *testing.T) {
	layout := draftedHome(t)
	held, err := layout.LockPR(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	defer func() { require.NoError(t, held.Unlock()) }()

	err = writeSummary(held, draftRound, ownerRecord, []summaryCount{{key: summaryDeduplicated, value: 1}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"suppressed_by_thread"`, "the refusal names the count left out")
	assert.Contains(t, err.Error(), "replaced whole")

	_, statErr := os.Stat(layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileSummary))
	assert.ErrorIs(t, statErr, os.ErrNotExist, "the refusal comes before anything is written")
}

// preparedRound runs the three pre-post writers over gradedHome's round, each
// writing command `times` times: `cr merge`, then `cr record` once — it
// appends, and is not one of the writers round 8's finding re-runs — then
// `cr draft`, a reviewer deleting one block, and `cr draft` again to ingest the
// deletion. It returns the round summary the run leaves.
func preparedRound(t *testing.T, times int) []byte {
	t.Helper()
	layout := gradedHome(t)
	other := aGradedRecord("f2")
	other["class"] = "missing-test"
	dir := t.TempDir()
	file := writeFanOut(t, dir, "correctness", aGradedRecord("f1"), other)
	out := filepath.Join(dir, "merged.ndjson")

	for range times {
		_, err := runCLIPrinting(t, "merge", file, "-o", out, "--repo", fixtureSlug, "--pr", fixturePR)
		require.NoError(t, err)
	}
	_, err := runRecord(t, fixturePR, out, "--repo", fixtureSlug)
	require.NoError(t, err)
	for range times {
		_, err := runDraft(t, fixturePR, "--repo", fixtureSlug)
		require.NoError(t, err)
	}
	drafted := layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileDraft)
	body, err := os.ReadFile(drafted)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(drafted, []byte(deleteBlock(t, string(body), "f2")), 0o600))
	for range times {
		_, err := runDraft(t, fixturePR, "--repo", fixtureSlug)
		require.NoError(t, err)
	}

	summary, err := layout.ReadRound(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileSummary)
	require.NoError(t, err)
	return summary
}

// postedRound runs §8.5.1's dry run `times` times over draftedHome's round and
// then posts it once with `--confirm`, returning the summary before the
// confirmation and after it.
func postedRound(t *testing.T, times int) (dry, confirmed []byte) {
	t.Helper()
	layout := draftedHome(t, aCitedRecord("f1"), aCitedRecord("f2"))
	redraft(t)
	for range times {
		_, err := runPost(t, draftPR, "--repo", draftSlug)
		require.NoError(t, err)
	}
	dry, err := layout.ReadRound(draftOwner, draftRepo, draftPRNum, draftRound, state.FileSummary)
	require.NoError(t, err)

	ghShimming(t, builtPayload(t))
	_, err = runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)
	confirmed, err = layout.ReadRound(draftOwner, draftRepo, draftPRNum, draftRound, state.FileSummary)
	require.NoError(t, err)
	return dry, confirmed
}

// Round 8's non-idempotent-accumulation end to end: `cr merge` twice, `cr draft`
// twice at each of its two moments, and `cr post` twice leave every count
// exactly where one run of each left it.
//
// The deletion is what gives the draft half its teeth. The second rendering
// after it reads a draft the deleted block is already gone from, so its triage
// discards nothing; a count of what the run discarded would fall from one to
// zero, and a count added to on each run would climb. The count read off the
// round's records stays at one either way.
//
// The dry runs are compared byte for byte, before the confirmation: §8.5.1's run
// writes nothing, so no number of them may move the document.
func TestRerunningEveryWriterLeavesTheRoundSummaryUnchanged(t *testing.T) {
	var once, twice []byte
	t.Run("once", func(t *testing.T) { once = preparedRound(t, 1) })
	t.Run("twice", func(t *testing.T) { twice = preparedRound(t, 2) })
	require.NotEmpty(t, once)
	assert.JSONEq(t, string(once), string(twice),
		"re-running cr merge and cr draft replaces their sections rather than adding to them")
	document := assertSummaryShape(t, twice, ownerMerge, ownerRecord, ownerDraft)
	assert.JSONEq(t, "2", string(document["raised"]))
	assert.JSONEq(t, "1", string(document["drafted"]))
	assert.JSONEq(t, "1", string(document["discarded_not_here"]),
		"the deleted block is counted once, however many renderings read the draft after it")

	var dryOnce, postedOnce, dryTwice, postedTwice []byte
	t.Run("one dry run", func(t *testing.T) { dryOnce, postedOnce = postedRound(t, 1) })
	t.Run("two dry runs", func(t *testing.T) { dryTwice, postedTwice = postedRound(t, 2) })
	require.NotEmpty(t, dryOnce)
	assert.Equal(t, string(dryOnce), string(dryTwice), "§8.5.1's dry run writes nothing, however often it runs")
	assert.JSONEq(t, string(postedOnce), string(postedTwice),
		"the confirmation after two dry runs records what it records after one")
}

// Round 8's unpostable-round-summary: a round that never sent a payload has no
// payload hash in its summary — the key is absent, not null and not the dry
// run's hash — and the summary it has is a valid one.
//
// The round is taken through both ways of not posting. §8.5.1's dry run builds
// a payload and prints the hash it would embed, which is exactly the value a
// careless writer would record; and a `--confirm` GitHub refuses per §8.4.2
// built one too and sent nothing that reached the author. Neither finalises the
// round, and after both the document holds `cr draft`'s counts in their stated
// shape and nothing of `cr post`'s.
func TestAnUnpostedRoundSummaryOmitsThePayloadHashAndStillValidates(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"))
	redraft(t)
	dryRunHash, err := builtPayload(t).Hash()
	require.NoError(t, err)
	_, err = runPost(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)

	rejectingShim(t)
	_, err = runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.Error(t, err, "§8.4.2: the refused call is this round's only confirmation")

	body, err := layout.ReadRound(draftOwner, draftRepo, draftPRNum, draftRound, state.FileSummary)
	require.NoError(t, err)
	document := assertSummaryShape(t, body, ownerDraft)
	assert.NotContains(t, document, "payload_hash",
		"an unposted round's summary omits the key entirely, rather than holding null")
	assert.NotContains(t, document, "posted", "and holds no posted count either: nothing finalised it")
	assert.NotContains(t, document, "confirm_given", "nor §8.5.4's confirmation, which is written with the other two")
	assert.NotContains(t, string(body), dryRunHash, "the dry run's hash names a review nobody received")
}
