package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/state"
)

// houseRuleID is the rule the §2.6.3.9 fixtures store, and its file stem.
const houseRuleID = "house-style"

// houseRuleJSON is a rule as an agent drafts one from review history: a
// question, carrying the comment it was drawn from.
const houseRuleJSON = `{"id":"` + houseRuleID + `","title":"A test name states the behaviour.",` +
	`"rationale":"The name is what a failing run prints.","class":"test-name","kind":"question",` +
	`"source":["https://github.com/acme/api/pull/7#discussion_r1"]}`

// handedRule writes a rule file outside any repository and returns its path.
func handedRule(t *testing.T, stem, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), stem+".json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// added runs `cr rules add` and returns the document it printed.
func added(t *testing.T, args ...string) (rulesAddResult, error) {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"rules", "add", "--repo", harvestSlug}, args...))
	err := cmd.Execute()
	var printed rulesAddResult
	if err == nil {
		require.NoError(t, json.Unmarshal(out.Bytes(), &printed))
	}
	return printed, err
}

// §2.6.3.9: a valid rule is written, byte for byte, to the per-repository layer
// as `<id>.json`, and the corpus resolves it from there with its `source`.
func TestAValidRuleIsStoredInThePerRepositoryLayer(t *testing.T) {
	layout := state.New(crHome(t))
	handed := handedRule(t, houseRuleID, houseRuleJSON)

	printed, err := added(t, handed)

	require.NoError(t, err)
	stored := layout.RepoRule(harvestOwner, harvestRepo, houseRuleID)
	assert.Equal(t, rulesAddResult{Repo: harvestSlug, ID: houseRuleID, Path: stored}, printed)
	body, err := os.ReadFile(stored)
	require.NoError(t, err)
	assert.Equal(t, houseRuleJSON, string(body), "the bytes written are the bytes handed over")
	corpus, err := rule.Resolve(layout.RepoRulesDir(harvestOwner, harvestRepo), layout.RulesDir(), "", nil)
	require.NoError(t, err)
	require.Len(t, corpus, 1)
	assert.Equal(t, rule.RepoSource, corpus[0].Source)
	assert.Equal(t, []string{"https://github.com/acme/api/pull/7#discussion_r1"}, corpus[0].Rule.Source)
}

// §2.6.3.9: a malformed rule is refused with exit code 1 naming the field, and
// nothing is written. The same file found in the corpus would be §2.6 item 5's
// code 3; handed to the command it is input data.
func TestAMalformedRuleIsRefusedNamingTheField(t *testing.T) {
	layout := state.New(crHome(t))
	handed := handedRule(t, houseRuleID, `{"id":"`+houseRuleID+`","title":"t","class":"test-name"}`)

	_, err := added(t, handed)

	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	var malformed *rule.MalformedError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, "rationale", malformed.Field)
	assert.Contains(t, hintFor(err), "rules add")
	assert.NoFileExists(t, layout.RepoRule(harvestOwner, harvestRepo, houseRuleID))
}

// §2.6.3.9: an id the per-repository layer already holds is refused with exit
// code 4, and the stored file is left as it was.
func TestAnIdTheLayerHoldsIsRefusedWithoutReplace(t *testing.T) {
	layout := state.New(crHome(t))
	_, err := added(t, handedRule(t, houseRuleID, houseRuleJSON))
	require.NoError(t, err)
	second := `{"id":"` + houseRuleID + `","title":"Another standard.","rationale":"r","class":"test-name"}`

	_, err = added(t, handedRule(t, houseRuleID, second))

	require.Error(t, err)
	assert.Equal(t, ExitState, exitCodeFor(err))
	assert.Contains(t, hintFor(err), "--replace")
	body, readErr := os.ReadFile(layout.RepoRule(harvestOwner, harvestRepo, houseRuleID))
	require.NoError(t, readErr)
	assert.Equal(t, houseRuleJSON, string(body))
}

// §2.6.3.9: with --replace the same id is overwritten, and the report says it
// replaced one.
func TestReplaceOverwritesTheStoredRule(t *testing.T) {
	layout := state.New(crHome(t))
	_, err := added(t, handedRule(t, houseRuleID, houseRuleJSON))
	require.NoError(t, err)
	second := `{"id":"` + houseRuleID + `","title":"Another standard.","rationale":"r","class":"test-name"}`

	printed, err := added(t, handedRule(t, houseRuleID, second), "--replace")

	require.NoError(t, err)
	assert.True(t, printed.Replaced)
	body, readErr := os.ReadFile(layout.RepoRule(harvestOwner, harvestRepo, houseRuleID))
	require.NoError(t, readErr)
	assert.Equal(t, second, string(body))
}

// §2.6.3.9: a rule carrying `source` in the finding register is refused with
// exit code 1, and nothing is written.
func TestASourcedRuleInTheFindingRegisterIsRefused(t *testing.T) {
	layout := state.New(crHome(t))
	sourced := `{"id":"` + houseRuleID + `","title":"t","rationale":"r","class":"test-name",` +
		`"kind":"finding","source":["https://github.com/acme/api/pull/7#discussion_r1"]}`

	_, err := added(t, handedRule(t, houseRuleID, sourced))

	var refused *SourcedFindingError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Contains(t, hintFor(err), "question")
	assert.NoFileExists(t, layout.RepoRule(harvestOwner, harvestRepo, houseRuleID))
}

// §2.6's `id` row holds the handed file as it will hold the stored one: an id
// that is not the file's stem is refused with exit code 1, naming `id`.
func TestAnIdThatIsNotTheFileStemIsRefused(t *testing.T) {
	crHome(t)

	_, err := added(t, handedRule(t, "draft", houseRuleJSON))

	var malformed *rule.MalformedError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, "id", malformed.Field)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
}

// An `axis` outside §1.5's set is the loader's *axis.InvalidError, which the
// corpus codes 3; handed to `cr rules add` it is input data, code 1.
func TestAnUnknownAxisInAHandedRuleIsCodedOne(t *testing.T) {
	crHome(t)
	handed := handedRule(t, houseRuleID,
		`{"id":"`+houseRuleID+`","title":"t","rationale":"r","class":"test-name","axis":"style"}`)

	_, err := added(t, handed)

	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
}
