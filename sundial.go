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

	loaded, err := s.loadSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	s.snapshot.Store(loaded)
	return s, nil
}

// Get returns a detached copy of the complete in-memory configuration and its
// Provider version.
func (s *Sundial[T]) Get() (T, Version, error) {
	current := s.snapshot.Load()
	config, err := decodeConfig[T](s.codec, current.data)
	if err != nil {
		return config, "", fmt.Errorf("sundial: decode configuration: %w", err)
	}
	return config, current.version, nil
}

// Put conditionally persists config and then publishes it to memory. A stale
// expectedVersion returns ErrConflict; failed saves leave memory unchanged.
func (s *Sundial[T]) Put(ctx context.Context, config T, expectedVersion Version) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	data, err := s.codec.Encode(config)
	if err != nil {
		return fmt.Errorf("sundial: encode configuration: %w", err)
	}
	next, err := decodeSnapshot[T](s.codec, data, "")
	if err != nil {
		return err
	}
	version, err := s.provider.Save(ctx, data, expectedVersion)
	if err != nil {
		return fmt.Errorf("sundial: save configuration: %w", err)
	}

	next.version = version
	s.snapshot.Store(next)
	return nil
}

func (s *Sundial[T]) loadSnapshot(ctx context.Context) (*snapshot, error) {
	data, version, err := s.provider.Load(ctx)
	if errors.Is(err, ErrNotFound) {
		data = nil
		version = ""
	} else if err != nil {
		return nil, fmt.Errorf("sundial: load configuration: %w", err)
	}

	return decodeSnapshot[T](s.codec, data, version)
}
