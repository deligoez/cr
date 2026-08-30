package state

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// numbered is a record shaped like the one §3.6.1 allocates: its number is a
// count over the records already in the file, which is the read-modify-write
// the context lock has to make exclusive.
type numbered struct {
	N int `json:"n"`
}

// contextRoot returns a layout with the global tree already created.
func contextRoot(t *testing.T) Layout {
	t.Helper()
	l := New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, l.Init())
	return l
}

// A key nobody has recorded against holds no notes, and asking for them is not
// a failure: §3.6's file is created by the first note appended to it, so every
// first `cr note` reads a store that is not there.
func TestAnUnwrittenContextStoreHoldsNoRecords(t *testing.T) {
	l := contextRoot(t)

	held, err := l.LockContext("CR-1")
	require.NoError(t, err)
	defer func() { assert.NoError(t, held.Unlock()) }()

	records, err := ContextRecords[numbered](held)
	require.NoError(t, err)
	assert.Empty(t, records)
	assert.NotNil(t, records, "an absent store is no records, and §12.3 spells that []")
	assert.NoFileExists(t, l.ContextFile("CR-1"), "reading must not create the store")
}

