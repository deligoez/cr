package cli

import (
	"errors"
	"fmt"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/state"
)

// UnknownProbeError reports a `--rerun` naming no probe the pull request holds,
// which §5.5.4 refuses with exit code 1: the command line is well-formed and
// the id in it is not there, so the id is the agent's to correct.
type UnknownProbeError struct {
	// ID is the id that was given.
	ID string
}

func (e *UnknownProbeError) Error() string {
	return fmt.Sprintf("--rerun %q names no probe this pull request holds; §5.5 stores one per "+
		"line of probes.ndjson, and `cr recheck` names the probe each posted record carries", e.ID)
}

// rerunFlagWhy is the reason every input flag beside `--rerun` is refused.
const rerunFlagWhy = "§5.5.4 runs the stored probe's own kind, input, filter, paths and target, " +
	"so the experiment cr repeats is the one that was recorded"

// rerunRun is §5.5.4's whole invocation: no input flag beside `--rerun`, the
// stored probe loaded into the request as if its inputs had been given on the
// command line, and the kind it names returned for the switch that runs it.
//
// It changes no record's `probe` and no grade. The new probe is its own record
// whose `rerun_of` names the old one, and `cr recheck` reports it beside the
// posted record that names the old one; what the result means is still
// §5.3.5's and §5.4.4's, and the agent's to judge.
func rerunRun(request *probeRequest, flags *probeFlags) (string, error) {
	if err := onlyTheRerun(flags); err != nil {
		return "", err
	}
	held, err := storedProbe(request.owner, request.repo, request.pr, flags.rerun)
	if err != nil {
		return "", err
	}
	if err := fillFromProbe(held, request); err != nil {
		return "", err
	}
	request.rerunOf = held.ID
	return string(held.Kind), nil
}

// onlyTheRerun is §5.5.4's refusal of an input flag beside `--rerun`, with exit
// code 2. `--proposal` is one of them: a proposal carries its own inputs, and
// running both would leave the record's `rerun_of` naming an experiment other
// than the one performed.
func onlyTheRerun(flags *probeFlags) error {
	for _, given := range []struct{ flag, value string }{
		{"--proposal", flags.fromProposal}, {"--kind", flags.kind},
		{"--patch", flags.patchFile}, {"--test", flags.testFile},
		{"--target", flags.target}, {"--filter", flags.filter},
	} {
		if given.value != "" {
			return fmt.Errorf("%s is rejected beside --rerun: %s", given.flag, rerunFlagWhy)
		}
	}
	if len(flags.paths) > 0 {
		return errors.New("--path is rejected beside --rerun: " + rerunFlagWhy)
	}
	return nil
}

// storedProbe is the probe id names, read from every round's probes.ndjson.
//
// The whole file and not the round's lines: §5.5.4 re-runs a posted record's
// probe after a push, and a posted record lives in the round that posted it,
// so the probe it names was recorded in an earlier round than the one the
// re-run is performed in.
func storedProbe(owner, repo string, pr int, id string) (*probe.Record, error) {
	layout, err := state.Default()
	if err != nil {
		return nil, err
	}
	stored, err := state.ReadRecords[probe.Record](layout, owner, repo, pr, state.FileProbes)
	if err != nil {
		return nil, err
	}
	for i := range stored {
		if stored[i].ID == id {
			return &stored[i], nil
		}
	}
	return nil, &UnknownProbeError{ID: id}
}

// fillFromProbe puts the stored probe's inputs where the two kinds read them.
//
// A mutation probe's target is not carried over, because §5.3.2 derives it
// from the patch again; a gap probe's is, because nothing else supplies it.
func fillFromProbe(held *probe.Record, request *probeRequest) error {
	request.filter, request.paths = held.Filter, held.Paths
	switch held.Kind {
	case probe.Mutation:
		files, err := git.ParsePatch(held.Input)
		if err != nil {
			return err
		}
		if len(files) == 0 {
			return &git.MalformedPatchError{
				File: held.ID, Problem: "holds no hunk: §5.3.1's mutation is a unified diff " +
					"against a sandbox file, and §5.5's `input` carries it whole",
			}
		}
		request.kind, request.patch, request.files = probe.Mutation, held.Input, files
		request.patchFile = held.ID
	case probe.Gap:
		request.kind, request.test, request.target = probe.Gap, held.Input, held.Target
	default:
		// The probe writer closes `kind` at the two, so this is
		// reachable only through a hand-edited state file.
		return fmt.Errorf("probe %s names kind %q, and §5.5 admits mutation and gap",
			held.ID, held.Kind)
	}
	return nil
}
