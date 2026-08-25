package sundial

import "context"

// Version is an opaque Provider-generated conditional-write token for one
// configuration document state. Only equality is meaningful; it need not be
// ordered or unique for every write. Its zero value identifies a missing
// document.
type Version string

// Provider loads and conditionally saves one complete configuration document.
type Provider interface {
	// Load returns the current document and its version from the same logical
	// read. A missing document returns ErrNotFound and the zero Version.
	Load(ctx context.Context) ([]byte, Version, error)
	// Save atomically replaces the document only when expectedVersion matches
	// the current version. It returns the version paired with the saved document;
	// a mismatch returns ErrConflict.
	Save(ctx context.Context, data []byte, expectedVersion Version) (Version, error)
}

// Watcher is an optional Provider capability for detecting external changes.
// The callback asks Sundial to reload the configuration from the Provider.
type Watcher interface {
	Watch(ctx context.Context, notify func() error) error
}
