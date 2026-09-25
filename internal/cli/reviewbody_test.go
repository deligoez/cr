package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// §8.4.3 through `cr post`'s dry run: the review body's framing lines and the
// axis names are built in per render.lang, which defaults to en and is set to
// tr in the repository's own configuration here, and every lens's entry keeps
// its English wording under both. The entries are those of every kind of
// §4.5.4 — a disabled axis, an unavailable axis, both unavailable lens halves,
// and a skipped role.
//
// Through v0.13.0 the body was English under every render.lang, which is what
// this test asserted until v0.14.0's §8.4.3 reversed it.
func TestTheReviewBodyIsFramedInTheRenderLanguage(t *testing.T) {
	bodies := make(map[string]string, 2)
	for _, lang := range []render.Lang{render.LangEN, render.LangTR} {
		layout := draftedHome(t, aCitedRecord("f1"))
		if lang == render.LangTR {
			require.NoError(t, os.MkdirAll(filepath.Dir(layout.RepoConfig(draftOwner, draftRepo)), 0o700))
			require.NoError(t, os.WriteFile(layout.RepoConfig(draftOwner, draftRepo),
				[]byte(`{"render":{"lang":"tr"}}`), 0o600))
		}
		redraft(t)
		recordedGH(t)

		printed, err := runPost(t, draftPR, "--repo", draftSlug)
		require.NoError(t, err)
		var report dryRun
		require.NoError(t, json.Unmarshal([]byte(printed), &report))
		require.NotNil(t, report.Payload)
		bodies[lang.String()] = report.Payload.Body
	}

	english, turkish := strings.Split(bodies["en"], "\n"), strings.Split(bodies["tr"], "\n")
	assert.Equal(t, []string{"**cr — review coverage**", "", "Axes reviewed: correctness, convention", "",
		"Lenses that did not run, and why:"}, english[:5], "§8.1.1: en is the default")
	assert.Equal(t, []string{"**cr — inceleme kapsamı**", "", "İncelenen eksenler: doğruluk, kod kuralları", "",
		"Çalışmayan incelemeler ve nedenleri:"}, turkish[:5], "the repository's render.lang tr frames the body")
	// The last line is the payload hash, which covers the comments too, and
	// since v0.16.0 §8.1.7's evidence region beneath each comment names its
	// fields in render.lang, so the two hashes differ.
	require.Len(t, turkish, len(english))
	assert.Equal(t, english[5:len(english)-1], turkish[5:len(turkish)-1],
		"each lens's reason keeps its wording under either language")

	heads := make([]string, 0, len(english))
	for _, line := range english[5:] {
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
