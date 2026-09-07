package sundial

import (
	"context"
	"fmt"
)

// GetRevision returns a detached historical configuration value and its revision.
func (s *Client[T]) GetRevision(ctx context.Context, revisionID string) (T, Revision, error) {
	var zero T
	provider, ok := s.provider.(RevisionManager)
	if !ok {
		return zero, Revision{}, ErrUnsupported
	}
	data, info, err := provider.GetRevision(ctx, revisionID)
	if err != nil {
		return zero, Revision{}, fmt.Errorf("sundial: get revision: %w", err)
	}
	value, err := decodeConfig[T](s.codec, data)
	if err != nil {
		return zero, Revision{}, fmt.Errorf("sundial: decode revision: %w", err)
	}
	return value, info, nil
}

// ListRevisions returns immutable revisions in newest-first order.
func (s *Client[T]) ListRevisions(
	ctx context.Context,
	opts ListRevisionsOptions,
) ([]Revision, error) {
	provider, ok := s.provider.(RevisionManager)
	if !ok {
		return nil, ErrUnsupported
	}
	if opts.Limit <= 0 {
		opts.Limit = DefaultListRevisionsLimit
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	revisions, err := provider.ListRevisions(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("sundial: list revisions: %w", err)
	}
	return revisions, nil
}

// RestoreRevision publishes a new revision containing a historical value.
// It decodes the historical content before writing and preserves its original bytes.
func (s *Client[T]) RestoreRevision(
	ctx context.Context,
	revisionID string,
) (Entry[T], error) {
	provider, ok := s.provider.(RevisionManager)
	if !ok {
		return Entry[T]{}, ErrUnsupported
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	current := s.snapshot.Load()
	data, _, err := provider.GetRevision(ctx, revisionID)
	if err != nil {
		return Entry[T]{}, fmt.Errorf("sundial: restore revision: %w", err)
	}
	var zeroRevision Revision
	next, value, err := decodeSnapshot[T](s.codec, data, zeroRevision)
	if err != nil {
		return Entry[T]{}, err
	}
	revision, err := s.provider.PutIfRevision(ctx, next.data, current.revision.ID)
	if err != nil {
		return Entry[T]{}, fmt.Errorf("sundial: restore revision: %w", err)
	}
	next.revision = revision
	s.snapshot.Store(next)
	return Entry[T]{Value: value, Revision: revision}, nil
}
