package sundial

import "context"

// Revision is an opaque Provider-generated conditional-write token for one
// configuration document state. Only equality is meaningful; it need not be
// ordered or unique for every write. Its zero value identifies a missing
// document.
type Revision string

// Provider loads and conditionally saves one complete configuration document.
type Provider interface {
	// Load returns the current document and its revision from the same logical
	// read. A missing document returns ErrNotFound and the zero Revision.
	Load(ctx context.Context) ([]byte, Revision, error)
	// Save atomically replaces the document only when expectedRevision matches
	// the current revision. It returns the revision paired with the saved
	// document; a mismatch returns ErrConflict.
	Save(ctx context.Context, data []byte, expectedRevision Revision) (Revision, error)
}

// Watcher is an optional Provider capability for detecting external changes.
// The callback asks Sundial to reload the configuration from the Provider.
type Watcher interface {
	Watch(ctx context.Context, notify func() error) error
}
