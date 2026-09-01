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

// Watcher detects Provider changes.
// Watch must notify once after registration and stop when ctx is canceled.
// A notify error means the change was not applied.
type Watcher interface {
	Watch(ctx context.Context, notify func() error) error
}
