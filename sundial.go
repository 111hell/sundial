package sundial

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sundayfun/sundial/codec"
)

// Sundial manages one typed configuration document and its in-memory state.
type Sundial[T any] struct {
	provider      Provider
	codec         codec.Codec
	watchInterval time.Duration

	writeMu  sync.Mutex
	snapshot atomic.Pointer[snapshot]
}

// Entry pairs a detached configuration value with the Provider metadata from
// the same in-memory snapshot.
type Entry[T any] struct {
	Value    T
	Metadata Metadata
}

// New creates a Sundial instance backed by provider and loads its initial configuration.
// A missing configuration document returns ErrNotFound.
func New[T any](ctx context.Context, provider Provider, opts ...Option) (*Sundial[T], error) {
	normalized := normalizeOptions(opts)
	s := &Sundial[T]{
		provider:      provider,
		codec:         normalized.Codec,
		watchInterval: normalized.WatchInterval,
		writeMu:       sync.Mutex{},
		snapshot:      atomic.Pointer[snapshot]{},
	}

	loaded, err := s.loadSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	s.snapshot.Store(loaded)
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
		return fmt.Errorf("sundial: encode configuration: %w", err)
	}
	next, err := decodeSnapshot[T](s.codec, data, Metadata{Revision: ""})
	if err != nil {
		return err
	}
	metadata, err := s.provider.PutIfRevision(ctx, data, entry.Metadata)
	if err != nil {
		return fmt.Errorf("sundial: put configuration: %w", err)
	}

	next.metadata = metadata
	s.snapshot.Store(next)
	return nil
}

func (s *Sundial[T]) loadSnapshot(ctx context.Context) (*snapshot, error) {
	data, metadata, err := s.provider.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("sundial: load configuration: %w", err)
	}

	return decodeSnapshot[T](s.codec, data, metadata)
}
