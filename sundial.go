package sundial

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/sundayfun/sundial/codec"
)

// Sundial manages one typed configuration document and its in-memory state.
type Sundial[T any] struct {
	provider Provider
	codec    codec.Codec
	logger   *slog.Logger

	writeMu  sync.Mutex
	snapshot atomic.Pointer[snapshot]
}

// Entry pairs a detached configuration value with the Provider metadata from
// the same in-memory snapshot.
type Entry[T any] struct {
	Value    T
	Metadata Metadata
}

// New loads the configuration and reloads it until ctx is canceled.
func New[T any](ctx context.Context, provider Provider, opts ...Option) (*Sundial[T], error) {
	normalized := normalizeOptions(opts)
	s := &Sundial[T]{
		provider: provider,
		codec:    normalized.Codec,
		logger:   normalized.Logger,
		writeMu:  sync.Mutex{},
		snapshot: atomic.Pointer[snapshot]{},
	}

	loaded, err := s.loadSnapshot(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "load configuration", "error", err)
		return nil, err
	}
	s.snapshot.Store(loaded)
	s.logger.DebugContext(ctx, "loaded configuration", "revision", loaded.metadata.Revision)

	go s.watch(ctx, normalized.Reload)

	return s, nil
}

// Get returns the current detached Entry from memory.
func (s *Sundial[T]) Get() (Entry[T], error) {
	current := s.snapshot.Load()
	config, err := decodeConfig[T](s.codec, current.data)
	if err != nil {
		return Entry[T]{Value: config, Metadata: Metadata{Revision: ""}},
			fmt.Errorf("sundial: decode configuration: %w", err)
	}
	return Entry[T]{Value: config, Metadata: current.metadata}, nil
}

// Put saves entry when its metadata revision is current, then updates memory.
// A stale revision returns ErrConflict.
func (s *Sundial[T]) Put(ctx context.Context, entry Entry[T]) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	data, err := s.codec.Encode(entry.Value)
	if err != nil {
		putErr := fmt.Errorf("sundial: encode configuration: %w", err)
		s.logger.ErrorContext(ctx, "put configuration", "error", putErr)
		return putErr
	}
	next, err := decodeSnapshot[T](s.codec, data, Metadata{Revision: ""})
	if err != nil {
		s.logger.ErrorContext(ctx, "put configuration", "error", err)
		return err
	}
	metadata, err := s.provider.PutIfRevision(ctx, data, entry.Metadata)
	if err != nil {
		putErr := fmt.Errorf("sundial: put configuration: %w", err)
		s.logger.ErrorContext(ctx, "put configuration", "error", putErr)
		return putErr
	}

	next.metadata = metadata
	s.snapshot.Store(next)
	s.logger.DebugContext(ctx, "put configuration", "revision", metadata.Revision)
	return nil
}

func (s *Sundial[T]) loadSnapshot(ctx context.Context) (*snapshot, error) {
	data, metadata, err := s.provider.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("sundial: get configuration: %w", err)
	}

	return decodeSnapshot[T](s.codec, data, metadata)
}
