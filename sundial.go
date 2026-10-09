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
	snapshot atomic.Pointer[snapshot[T]]
}

// Entry pairs a configuration value with its revision.
type Entry[T any] struct {
	// Value is shared and read-only when returned by Get, Update, Put,
	// RestoreRevision or OnChange. Draft returns an independently editable value.
	Value T
	// Revision describes the immutable revision paired with Value.
	Revision Revision
}

// New loads an existing configuration and reloads it until ctx is canceled.
// Get returns shared read-only values; use Update or Draft to prepare changes.
func New[T any](ctx context.Context, provider Provider, opts ...Option[T]) (*Client[T], error) {
	normalized := normalizeOptions(opts)
	s := &Client[T]{
		provider: provider,
		codec:    normalized.Codec,
		logger:   normalized.Logger,
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

// Get returns the current shared read-only Entry without encoding or decoding.
// Callers must not modify its maps, slices, pointers or other referenced data.
func (s *Client[T]) Get() Entry[T] {
	return s.entry(s.snapshot.Load())
}

// Draft decodes the cached document into an independently editable value and
// preserves the revision of that document. It does not read from the Provider.
func (s *Client[T]) Draft() (Entry[T], error) {
	current := s.snapshot.Load()
	value, err := decodeConfig[T](s.codec, current.data)
	if err != nil {
		return Entry[T]{}, fmt.Errorf("sundial: decode draft: %w", err)
	}
	return Entry[T]{Value: value, Revision: current.revision}, nil
}

// Put saves entry when its revision ID is current, then updates memory.
// It returns the shared read-only saved Entry with its new revision. A stale
// or empty revision ID returns ErrConflict. Do not modify entry during the call.
func (s *Client[T]) Put(ctx context.Context, entry Entry[T]) (Entry[T], error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.put(ctx, entry)
}

// Update decodes an independent draft of the current snapshot, applies modify,
// and publishes it using that snapshot's revision. Local writes and reloads are
// serialized for the entire operation; storage still enforces revision CAS.
// If decoding, modify or publication fails, the accepted snapshot is preserved.
// The returned Entry is shared and read-only. modify must not call Update, Put,
// Reload or RestoreRevision on this Client because they acquire the same lock.
func (s *Client[T]) Update(ctx context.Context, modify func(*T) error) (Entry[T], error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	draft, err := s.Draft()
	if err != nil {
		s.logger.ErrorContext(ctx, "update configuration", "error", err)
		return Entry[T]{}, err
	}
	if err := modify(&draft.Value); err != nil {
		return Entry[T]{}, err
	}
	return s.put(ctx, draft)
}

// put requires writeMu to be held by the caller.
func (s *Client[T]) put(ctx context.Context, entry Entry[T]) (Entry[T], error) {
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
