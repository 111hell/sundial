package sundial

import "context"

// Metadata describes the Provider state paired with a configuration document.
type Metadata struct {
	Revision string
}

// Provider reads and writes one complete configuration document.
type Provider interface {
	// Get returns the current document and its revision from the same logical
	// read. A missing document returns ErrNotFound and zero Metadata.
	Get(ctx context.Context) ([]byte, Metadata, error)
	// Put writes the document without checking the current revision.
	Put(ctx context.Context, data []byte) (Metadata, error)
	// PutIfRevision atomically replaces an existing document only when the non-empty
	// expectedMetadata.Revision matches the current revision. It returns the
	// metadata paired with the saved document; a mismatch returns ErrConflict.
	PutIfRevision(ctx context.Context, data []byte, expectedMetadata Metadata) (Metadata, error)
}

// Watcher is an optional Provider capability for detecting external changes.
// The callback asks Sundial to reload the configuration from the Provider. A
// callback error means the change was not applied.
type Watcher interface {
	Watch(ctx context.Context, notify func() error) error
}
