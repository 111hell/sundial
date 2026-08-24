package sundial

import (
	"context"
	"errors"
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

// New creates a Sundial instance backed by provider and loads its initial configuration.
// A missing configuration document starts with the zero value of T.
func New[T any](ctx context.Context, provider Provider, opts ...Option) (*Sundial[T], error) {
	normalized := normalizeOptions(opts)
	s := &Sundial[T]{
		provider:      provider,
		codec:         normalized.Codec,
		watchInterval: normalized.WatchInterval,
		writeMu:       sync.Mutex{},
		snapshot:      atomic.Pointer[snapshot]{},
	}

	data, err := s.provider.Load(ctx)
	if errors.Is(err, ErrNotFound) {
		data = nil
	} else if err != nil {
		return nil, fmt.Errorf("sundial: load configuration: %w", err)
	}

	loaded, err := decodeSnapshot[T](s.codec, data)
	if err != nil {
		return nil, err
	}
	s.snapshot.Store(loaded)
	return s, nil
}

// Get returns a detached copy of the complete in-memory configuration.
func (s *Sundial[T]) Get() (T, error) {
	config, err := decodeConfig[T](s.codec, s.snapshot.Load().data)
	if err != nil {
		return config, fmt.Errorf("sundial: decode configuration: %w", err)
	}
	return config, nil
}

// Put persists a complete configuration document and then publishes it to
// memory. A failed save leaves the current in-memory configuration unchanged.
func (s *Sundial[T]) Put(ctx context.Context, config T) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	data, err := s.codec.Encode(config)
	if err != nil {
		return fmt.Errorf("sundial: encode configuration: %w", err)
	}
	next, err := decodeSnapshot[T](s.codec, data)
	if err != nil {
		return err
	}
	if err := s.provider.Save(ctx, data); err != nil {
		return fmt.Errorf("sundial: save configuration: %w", err)
	}

	s.snapshot.Store(next)
	return nil
}
