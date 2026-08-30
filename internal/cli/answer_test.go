package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// The pull request `cr answer` is exercised against here.
const (
	answeredOwner = "acme"
	answeredRepo  = "web"
	answeredSlug  = answeredOwner + "/" + answeredRepo
	answeredPR    = "42"
	answeredPRNum = 42
)

// briefedHome puts a state root behind CR_HOME holding one pull request whose
// metadata names issueKey, which is what `cr brief` leaves behind and what the
// command reads the key out of. An empty issueKey is §3.2's fallback.
func briefedHome(t *testing.T, issueKey string) state.Layout {
	t.Helper()
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsurePR(answeredOwner, answeredRepo, answeredPRNum))

	held, err := layout.LockPR(answeredOwner, answeredRepo, answeredPRNum)
	require.NoError(t, err)
	recorded := state.Meta{
		Owner: answeredOwner, Repo: answeredRepo, PR: answeredPRNum, IssueKey: issueKey,
	}
	require.NoError(t, held.WriteMeta(&recorded))
	require.NoError(t, held.Unlock())
	return layout
}

// runAnswer runs `cr answer` with args against whatever CR_HOME points at, and
// returns what it printed and what it refused.
func runAnswer(t *testing.T, args ...string) (printed string, err error) {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"answer"}, args...))
	// Executed first and read after: a return statement evaluates its
	// operands left to right, so reading the buffer in it would read what
	// the command printed before it ran.
	err = cmd.Execute()
	return out.String(), err
}

// §3.6.2 through the command: the answer lands in §2.2's context store under
// the issue key the pull request resolved to, carrying the record it answers.
//
// The key is never typed. `cr answer` is addressed by pull request because §11
// scopes a record id to one, and the key comes from that pull request's §2.3
// metadata, so the answer is filed where §3.6.4 will load it from rather than
// under whatever a second typing produced.
func TestAnswerRecordsAgainstThePullRequestsIssueKey(t *testing.T) {
	layout := briefedHome(t, "CR-7")

	out, err := runAnswer(t,
		answeredPR, "f3", "the retry is deliberate", "--source", "thread", "--repo", answeredSlug)
	require.NoError(t, err)

	var printed struct {
		Note note.Note `json:"note"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &printed))
	assert.Equal(t, "CR-7#n1", printed.Note.ID)
	assert.Equal(t, "f3", printed.Note.Record)
	assert.Equal(t, answeredPRNum, printed.Note.PR)
	assert.Equal(t, note.SourceThread, printed.Note.Source)
	assert.False(t, printed.Note.RecordedAt.IsZero(), "§3.6.1 requires a timestamp")

	stored, err := os.ReadFile(layout.ContextFile("CR-7"))
	require.NoError(t, err)
	assert.Contains(t, string(stored), "the retry is deliberate")
	assert.Contains(t, string(stored), `"record":"f3"`)
}

// §12.1's other shape for this command. A terminal reader gets the id, the
// record answered, and the source: §3.6.6 makes the note revocable by id,
// §3.6.2 makes the record the thing this note is about, and §8.1.6 will
// disclose the source in every posted body resting on it.
func TestATerminalAnswerNamesTheIdTheRecordAndTheSource(t *testing.T) {
	briefedHome(t, "CR-7")

	out := throughATerminal(t,
		"answer", answeredPR, "f3", "the retry is deliberate", "--source", "thread", "--repo", answeredSlug)

	assert.Contains(t, out, "recorded ")
	assert.Contains(t, out, "\x1b[36mCR-7#n1\x1b[0m",
		"the id is accented, as every terminal rendering accents its answer")
	assert.Contains(t, out, "answering f3")
	assert.Contains(t, out, " from thread")
}

// §3.6.2's negative, observed rather than argued: the record the answer names
// is byte-identical before and after.
//
// v0.1 ends at posting per §9, and whether an answer settles a question is a
// judgement v0.2 makes, so an answer moves no record out of `posted` and edits
// nothing else about it either. The whole file is compared rather than the one
// record's state field, because §9.1's state is not the only thing a run could
// have disturbed.
func TestAnAnswerLeavesTheAnsweredRecordByteIdentical(t *testing.T) {
	layout := briefedHome(t, "CR-7")

	findings := layout.PRFile(answeredOwner, answeredRepo, answeredPRNum, state.FileFindings)
	before := []byte(`{"id":"f3","kind":"question","summary":"why is the retry unbounded?",` +
		`"state":"posted","thread_id":"PRRT_1","head":"0f1e2d3","round":1}` + "\n")
	require.NoError(t, os.WriteFile(findings, before, 0o600))

	_, err := runAnswer(t,
		answeredPR, "f3", "the retry is deliberate", "--source", "thread", "--repo", answeredSlug)
	require.NoError(t, err)

	after, err := os.ReadFile(findings)
	require.NoError(t, err)
	assert.Equal(t, before, after, "§3.6.2: an answer must not change the record's state")

	// The run really did record the answer, so the assertion above is about
	// a command that ran rather than one that refused early.
	recorded, err := os.ReadFile(layout.ContextFile("CR-7"))
	require.NoError(t, err)
	assert.Contains(t, string(recorded), `"record":"f3"`)
}

// Every way `cr answer` can be refused, with the §11.2 code its cause carries.
//
// The three codes are three different faults. A malformed invocation is 2; a
// pull request cr was never briefed on is a file it expected and did not find,
// which is 3; and a pull request that resolved to no issue key is recorded
// state under §3.2's fallback with nowhere to keep the answer, which is 1.
func TestAnswerRefusesWithTheCodeItsCauseCarries(t *testing.T) {
	for name, refusal := range map[string]struct {
		issueKey string
		briefed  bool
		args     []string
		code     int
	}{
		"a record id §6.1 does not spell": {
			issueKey: "CR-7", briefed: true,
			args: []string{answeredPR, "f03", "answered", "--source", "chat", "--repo", answeredSlug},
			code: ExitUsage,
		},
		"no record id at all": {
			issueKey: "CR-7", briefed: true,
			args: []string{answeredPR, "answered", "--source", "chat", "--repo", answeredSlug},
			code: ExitUsage,
		},
		"no source": {
			issueKey: "CR-7", briefed: true,
			args: []string{answeredPR, "f3", "answered", "--repo", answeredSlug},
			code: ExitUsage,
		},
		"an unlisted source": {
			issueKey: "CR-7", briefed: true,
			args: []string{answeredPR, "f3", "answered", "--source", "gossip", "--repo", answeredSlug},
			code: ExitUsage,
		},
		"no repository": {
			issueKey: "CR-7", briefed: true,
			args: []string{answeredPR, "f3", "answered", "--source", "chat"},
			code: ExitUsage,
		},
		"a pull request that is not one": {
			issueKey: "CR-7", briefed: true,
			args: []string{"zero", "f3", "answered", "--source", "chat", "--repo", answeredSlug},
			code: ExitUsage,
		},
		"a pull request cr has never briefed": {
			briefed: false,
			args:    []string{answeredPR, "f3", "answered", "--source", "chat", "--repo", answeredSlug},
			code:    ExitFile,
		},
		"a pull request that resolved to no issue key": {
			issueKey: "", briefed: true,
			args: []string{answeredPR, "f3", "answered", "--source", "chat", "--repo", answeredSlug},
			code: ExitValidation,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var layout state.Layout
			if refusal.briefed {
				layout = briefedHome(t, refusal.issueKey)
			} else {
				layout = state.New(crHome(t))
				require.NoError(t, layout.Init())
			}

			out, err := runAnswer(t, refusal.args...)
			require.Error(t, err)
			assert.Equal(t, refusal.code, exitCodeFor(err))
			assert.Empty(t, out, "a refused run prints no note")
			assert.NoFileExists(t, layout.ContextFile("CR-7"))
		})
	}
}

// answerSource are the non-test files `cr answer` is made of: the command
// itself and the package that owns §3.6's store. Everything they call below
// that is shared infrastructure whose per-PR write door is a method on the
// exclusive lock, so a caller that never takes the lock reaches none of it.
//
// Tests are excluded on purpose. This says what the command does when it runs;
// the tests beside it are what put the record on disk for it to leave alone,
// and they must be free to write the very state the command may not.
func answerSource(t *testing.T) []string {
	t.Helper()
	root := moduleRoot(t)

	files := []string{filepath.Join(root, "internal", "cli", "answer.go")}
	matched, err := filepath.Glob(filepath.Join(root, "internal", "note", "*.go"))
	require.NoError(t, err)
	for _, path := range matched {
		if !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
	}
	require.Greater(t, len(files), 3, "only %d files were scanned, so this guard proved nothing", len(files))
	return files
}

// stateWriters are the names that can reach a record's state: the constructor
// of the exclusive per-PR lock, the three writers that are methods on it, and
// the two §2.3 files a record's state and its history live in.
//
// They are searched for as text, which is what a file that never mentions one
// cannot be doing anything with. Nothing here is reachable by another spelling:
// the lock's fields are unexported, so its constructor is the only way to hold
// one, and every write to §2.3's files goes through a method that demands it.
var stateWriters = []string{
	"LockPR", "WriteStamped", "WriteRecords", "WriteMeta", "FileFindings", "FileTransitions",
}

// §3.6.2 forbids an answer to change the record's state, and the way that is
// kept is structural: `cr answer` has no route to a record's state at all.
//
// TestAnAnswerLeavesTheAnsweredRecordByteIdentical is the observation, and this
// is the reason it will keep holding. A run that happens not to write today
// could start writing tomorrow with nothing failing; a path that cannot take
// the per-PR lock cannot write any of §2.3's files whatever it later does. The
// two record files are named as well, so even a read of them — the step towards
// resolving the id that §9.3.5 argues against — has to be a deliberate change
// here rather than something a call site does by existing.
func TestAnswerHasNoRouteToARecordsState(t *testing.T) {
	root := moduleRoot(t)

	var found []string
	for _, path := range answerSource(t) {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		rel, err := filepath.Rel(root, path)
		require.NoError(t, err)
		for _, writer := range stateWriters {
			if strings.Contains(string(raw), writer) {
				found = append(found, rel+" names "+writer)
			}
		}
	}

	assert.Empty(t, found,
		"§3.6.2: an answer may not change a record's state, so its path never reaches one")
}
