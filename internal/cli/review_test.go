package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/review"
)

// An `--axis` naming no axis of §1.5 is refused as the malformed invocation it
// is, with §11.2's code 2 and the closed set named, before any state is read.
func TestAnAxisOutsideSection15IsAUsageError(t *testing.T) {
	crHome(t)

	err := runCLI(t, "review", "7", "--repo", "acme/api", "--axis", "style")

	var unknown *unknownAxisFlagError
	require.ErrorAs(t, err, &unknown)
	assert.Equal(t, ExitUsage, exitCodeFor(err))
	assert.Contains(t, err.Error(), "intent, correctness, convention, test")
}

