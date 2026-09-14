package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/render"
	"github.com/deligoez/cr/internal/state"
)

// builtReview assembles the round's payload the way `cr post` does.
func builtReview(t *testing.T, layout state.Layout) *post.Review {
	t.Helper()
	round := state.Meta{
		Owner: draftOwner, Repo: draftRepo, PR: draftPRNum,
		Round: draftRound, Head: draftHead,
	}
	queued, err := roundFindingsOf(layout, draftOwner, draftRepo, draftPRNum, draftRound)
	require.NoError(t, err)
	review, err := buildPayload(
		layout, draftOwner, draftRepo, draftPRNum, &round, queued, map[string]string{})
	require.NoError(t, err)
	return review
}

// §8.4.3 through the payload `cr post` builds: the review's own body carries
// §4.5.4's disclosure, and beneath it the payload hash as an HTML comment.
//
// All four kinds of §4.5.4 are asserted, which is what round 10's finding
// disclosure-scope-drift left open and this task's last criterion restates: an
// unavailable axis, both unavailable lens halves, and a skipped role. The
// fixture reaches them without arranging for any of them — it resolves no
// profile and records no issue key, so the intent axis is unavailable, neither
// half of §4.3.1 and §4.4.1 can be built, and no role of the corpus was active.
func TestTheReviewBodyDisclosesEveryLensThatDidNotRun(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"))
	redraft(t)

	review := builtReview(t, layout)

	lines := strings.Split(review.Body, "\n")
	for kind, entry := range map[string]string{
		"an unavailable axis": "- axis intent did not run: cr found no issue linked to this pull request, " +
			"so it did not check the change against what an issue asks for",
		"§4.3.1's reinvention half": "- lens convention/reinvention did not run: cr could not index the " +
			"repository's existing code, so it did not check whether the change re-implements code the " +
			"repository already has",
		"§4.4.1's symbol half": "- lens test/symbols did not run: cr could not index the repository's " +
			"existing code, so it did not read which code the changed tests exercise",
		"a disabled axis": "- axis test did not run: cr has no command to run this repository's tests, " +
			"so it did not check whether the change is adequately tested",
		"a role skipped per §4.6.4": "- role test-adequacy did not look at this change: axis test did not run",
	} {
		assert.Containsf(t, lines, entry, "§8.4.3 owes the author %s, in words the author can read", kind)
	}
}

// The review body of §8.4.3 is English whatever render.lang says, per the
// user's 2026-09-13 decision: `cr post` run under tr and under en prints the
// same body, byte for byte, and it is the English framing around every kind of
// §4.5.4's entries — a disabled axis, an unavailable axis, both unavailable lens
// halves, and a skipped role.
//
// The run under each language first proves the language took effect, by
// reading the settings `cr post` resolves, so two equal bodies cannot come from
// two runs that both ignored the variable.
func TestTheReviewBodyIsTheSameEnglishUnderEitherRenderLanguage(t *testing.T) {
	bodies := make(map[string]string, 2)
	for _, lang := range []render.Lang{render.LangTR, render.LangEN} {
		t.Setenv("CR_RENDER_LANG", lang.String())
		layout := draftedHome(t, aCitedRecord("f1"))
		redraft(t)
		recordedGH(t)

		settings, err := resolveDraftSettings(layout, draftOwner, draftRepo)
		require.NoError(t, err)
		require.Equal(t, lang, settings.lang, "the run reads render.lang as %s", lang)

		printed, err := runPost(t, draftPR, "--repo", draftSlug)
		require.NoError(t, err)
		var report dryRun
		require.NoError(t, json.Unmarshal([]byte(printed), &report))
		require.NotNil(t, report.Payload)
		bodies[lang.String()] = report.Payload.Body
	}

	assert.Equal(t, bodies["en"], bodies["tr"], "render.lang does not reach §8.4.3's review body")

	lines := strings.Split(bodies["tr"], "\n")
	assert.Equal(t, []string{"**cr — review coverage**", "", "Axes reviewed: correctness, convention", "",
		"Lenses that did not run, and why:"}, lines[:5])
	heads := make([]string, 0, len(lines))
	for _, line := range lines[5:] {
		if entry, ok := strings.CutPrefix(line, "- "); ok {
			head, _, _ := strings.Cut(entry, ":")
			heads = append(heads, head)
		}
	}
	assert.Equal(t, []string{
		"axis test did not run",
		"axis intent did not run",
		"lens convention/reinvention did not run",
		"lens test/symbols did not run",
		"role convention did not look at this change",
		"role correctness did not look at this change",
		"role intent-coverage did not look at this change",
		"role test-adequacy did not look at this change",
	}, heads, "entries of every kind §4.5.4 names, in English")
}

// The hash embedded in the body is the payload hash of §8.3.3, and the body it
// sits in reached no part of computing it.
//
// That is what makes §8.4.3 possible at all rather than circular: §8.3.3 builds
// the pre-image out of the comments alone, so the value can be embedded in a
// body that is not in it. §8.4.4 then matches this line against an already
// posted review to decide between adopting it and posting a second one, so the
// embedded value and the stored one have to be one value by construction.
func TestTheEmbeddedHashIsThePayloadHashOfTheComments(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"), aCitedRecord("f2"))
	redraft(t)

	review := builtReview(t, layout)

	embedded, found := render.PayloadHashIn(review.Body)
	require.True(t, found, "§8.4.3 embeds the hash as an HTML comment")
	computed, err := review.Hash()
	require.NoError(t, err)
	assert.Equal(t, computed, embedded)

	// The body moves and the hash does not, which is the property stated
	// the only way that cannot be satisfied by accident.
	review.Body = "something else entirely"
	moved, err := review.Hash()
	require.NoError(t, err)
	assert.Equal(t, computed, moved, "§8.3.3 excludes the review body from the pre-image")
}

// The review body is not a line comment, so §1.6.1 does not reach it: it
// carries no anchor, it is not one of the payload's comments, and it does not
// count against §1.6.2's cap.
//
// The cap is the half worth asserting. §1.6.2 blocks posting above
// post.max_comments, and a body counted as a comment would make a round of
// exactly the cap refuse — the disclosure §4.5.4 requires would then be the
// thing that stopped the review from being sent.
func TestTheReviewBodyIsNotOneOfTheComments(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"), aCitedRecord("f2"))
	redraft(t)

	review := builtReview(t, layout)

	require.Len(t, review.Comments, 2, "one comment per queued record, and the body is not one")
	for i := range review.Comments {
		assert.NotEqual(t, review.Body, review.Comments[i].Body)
		assert.NotEmpty(t, review.Comments[i].Path, "every comment is anchored, per §1.6.1")
	}
	assert.NotContains(t, review.Body, `"path"`, "the body names no location of its own")
}

// The body reaches the payload the command reports, so a reviewer reading
// `cr post` before confirming sees exactly what the author would receive.
func TestThePostedPayloadCarriesTheBody(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"))
	redraft(t)

	printed, err := runPost(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)
	assert.NotEmpty(t, printed, "the run reported the payload it built")

	payload, err := builtReview(t, layout).Payload()
	require.NoError(t, err)
	var document struct {
		Body     string `json:"body"`
		Comments []struct {
			Body string `json:"body"`
		} `json:"comments"`
	}
	require.NoError(t, json.Unmarshal(payload, &document))
	require.Len(t, document.Comments, 1)

	assert.Contains(t, document.Body, "<!-- cr:payload-hash ")
	assert.Less(t,
		strings.Index(document.Body, "per §4.5.4"),
		strings.Index(document.Body, "<!-- cr:payload-hash "),
		"§8.4.3 puts the disclosure above the hash comment in what is sent")
}

// A record's own body and the review body are rendered apart, so nothing of
// §8.4.3's region can be mistaken for §8.1's.
func TestTheDisclosureIsNotRenderedIntoAComment(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"))
	redraft(t)

	review := builtReview(t, layout)

	for i := range review.Comments {
		assert.NotContains(t, review.Comments[i].Body, "per §4.5.4",
			"§4.5.4's disclosure belongs to the review body, not to a line comment")
		assert.NotContains(t, review.Comments[i].Body, "cr:payload-hash")
	}
}
