package sundial

import (
	"context"
	"time"
)

// DefaultListRevisionsLimit bounds history traversal when no positive limit is provided.
const DefaultListRevisionsLimit = 256

// Revision describes one immutable configuration revision.
type Revision struct {
	// ID uniquely identifies the immutable revision.
	ID string
	// ParentID identifies the previously active revision.
	ParentID string
	// CreatedAt is the revision creation time.
	CreatedAt time.Time
}

// ListRevisionsOptions controls history queries.
type ListRevisionsOptions struct {
	// Limit caps the number of newest-first results. Non-positive uses the default limit.
	Limit int
	// Offset skips this many newest revisions. Non-positive starts from the newest.
	Offset int
}

// Provider reads and writes one complete configuration document.
type Provider interface {
	// Get returns the current document and its revision from the same logical read.
	Get(ctx context.Context) ([]byte, Revision, error)
	// Put writes the document without requiring a caller-supplied revision.
	// Providers may use internal concurrency checks and return ErrConflict
	// when concurrent writes prevent publication.
	Put(ctx context.Context, data []byte) (Revision, error)
	// PutIfRevision atomically replaces an existing document only when the non-empty
	// expectedRevisionID matches the current revision ID. It returns the saved
	// revision; a mismatch returns ErrConflict.
	PutIfRevision(ctx context.Context, data []byte, expectedRevisionID string) (Revision, error)
}

// RevisionManager provides optional configuration history operations.
type RevisionManager interface {
	// GetRevision returns an immutable revision's content and descriptor.
	GetRevision(ctx context.Context, revisionID string) ([]byte, Revision, error)
	// ListRevisions returns immutable revisions in newest-first order.
	ListRevisions(ctx context.Context, opts ListRevisionsOptions) ([]Revision, error)
	// RestoreRevision conditionally copies a historical revision into a new current revision.
	RestoreRevision(
		ctx context.Context,
		revisionID string,
		expectedRevisionID string,
	) ([]byte, Revision, error)
}

// Watcher detects Provider changes.
// Watch must notify once after registration and stop when ctx is canceled.
// A notify error means the change was not applied.
type Watcher interface {
	Watch(ctx context.Context, notify func() error) error
}
