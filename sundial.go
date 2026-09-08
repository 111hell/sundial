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

	writeMu  sync.Mutex
	snapshot atomic.Pointer[snapshot]
}

// Entry pairs a detached configuration value with its revision.
type Entry[T any] struct {
	// Value is a detached copy of the configuration document.
	Value T
	// Revision describes the immutable revision paired with Value.
	Revision Revision
}

// New loads an existing configuration and reloads it until ctx is canceled.
func New[T any](ctx context.Context, provider Provider, opts ...Option[T]) (*Client[T], error) {
	normalized := normalizeOptions(opts)
	s := &Client[T]{
		provider: provider,
		codec:    normalized.Codec,
		logger:   normalized.Logger,
		writeMu:  sync.Mutex{},
		snapshot: atomic.Pointer[snapshot]{},
	}

	loaded, _, err := s.loadSnapshot(ctx)
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
func (s *Client[T]) Get() (Entry[T], error) {
	current := s.snapshot.Load()
	config, err := decodeConfig[T](s.codec, current.data)
	if err != nil {
		var zeroRevision Revision
		return Entry[T]{Value: config, Revision: zeroRevision},
			fmt.Errorf("sundial: decode configuration: %w", err)
	}
	return Entry[T]{Value: config, Revision: current.revision}, nil
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
	next, savedValue, err := decodeSnapshot[T](
		s.codec,
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
	return Entry[T]{Value: savedValue, Revision: revision}, nil
}

func (s *Client[T]) loadSnapshot(ctx context.Context) (*snapshot, Entry[T], error) {
	data, revision, err := s.provider.Get(ctx)
	if err != nil {
		return nil, Entry[T]{}, fmt.Errorf("sundial: get configuration: %w", err)
	}

	next, config, err := decodeSnapshot[T](s.codec, data, revision)
	if err != nil {
		return nil, Entry[T]{}, err
	}
	return next, Entry[T]{Value: config, Revision: revision}, nil
}
