package testutil

import (
	"testing"

	"github.com/usepackyard/packyard/internal/store"
)

// NewStores returns a Stores backed by a fresh in-memory database.
func NewStores(t *testing.T) *store.Stores {
	t.Helper()
	return store.NewStores(NewDB(t))
}
