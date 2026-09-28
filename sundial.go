package sundial

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/sundayfun/sundial/codec"
)

// Client manages one typed configuration document and its in-memory state.
type Client[T any] struct {
	provider Provider
	codec    codec.Codec
	logger   *slog.Logger
	clone    func(T) T

	writeMu  sync.Mutex
	snapshot atomic.Pointer[snapshot[T]]
}

// Entry pairs a detached configuration value with its revision.
type Entry[T any] struct {
	// Value is a detached copy of the configuration document.
	Value T
	// Revision describes the immutable revision paired with Value.
	Revision Revision
}

// New loads an existing configuration and reloads it until ctx is canceled.
// clone must be non-nil and return a deep copy without changing its input.
// It must be safe for concurrent calls and must not encode or decode.
func New[T any](ctx context.Context, provider Provider, clone func(T) T, opts ...Option[T]) (*Client[T], error) {
	normalized := normalizeOptions(opts)
	if clone == nil {
		return nil, ErrCloneRequired
	}
	s := &Client[T]{
		provider: provider,
		codec:    normalized.Codec,
		logger:   normalized.Logger,
		clone:    clone,
		writeMu:  sync.Mutex{},
		snapshot: atomic.Pointer[snapshot[T]]{},
	}

	loaded, err := s.loadSnapshot(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "load configuration", "error", err)
		return nil, err
	}
	s.snapshot.Store(loaded)
	s.logger.DebugContext(ctx, "loaded configuration", "revision_id", loaded.revision.ID)

	go s.watch(ctx, normalized.Reload)

	return s, nil
}

// Get returns the current detached Entry from memory.
func (s *Client[T]) Get() Entry[T] {
	return s.entry(s.snapshot.Load())
}

// Put saves entry when its revision ID is current, then updates memory.
// It returns the codec-decoded saved Entry with its new revision. A stale
// or empty revision ID returns ErrConflict.
func (s *Client[T]) Put(ctx context.Context, entry Entry[T]) (Entry[T], error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	data, err := s.codec.Encode(entry.Value)
	if err != nil {
		putErr := fmt.Errorf("sundial: encode configuration: %w", err)
		s.logger.ErrorContext(ctx, "put configuration", "error", putErr)
		return Entry[T]{}, putErr
	}
	var zeroRevision Revision
	next, err := s.decodeSnapshot(
		data,
		zeroRevision,
	)
	if err != nil {
		s.logger.ErrorContext(ctx, "put configuration", "error", err)
		return Entry[T]{}, err
	}
	revision, err := s.provider.PutIfRevision(ctx, data, entry.Revision.ID)
	if err != nil {
		putErr := fmt.Errorf("sundial: put configuration: %w", err)
		s.logger.ErrorContext(ctx, "put configuration", "error", putErr)
		return Entry[T]{}, putErr
	}

	next.revision = revision
	s.snapshot.Store(next)
	s.logger.DebugContext(ctx, "put configuration", "revision_id", revision.ID)
	return s.entry(next), nil
}

func (s *Client[T]) loadSnapshot(ctx context.Context) (*snapshot[T], error) {
	data, revision, err := s.provider.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("sundial: get configuration: %w", err)
	}

	return s.decodeSnapshot(data, revision)
}
