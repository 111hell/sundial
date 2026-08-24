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

// Sundial manages one configuration document and its in-memory state.
type Sundial struct {
	provider      Provider
	codec         codec.Codec
	watchInterval time.Duration

	writeMu  sync.Mutex
	snapshot atomic.Pointer[snapshot]
}

// New creates a Sundial instance and loads its initial configuration.
// A missing configuration document starts as an empty configuration.
func New(ctx context.Context, opts Options) (*Sundial, error) {
	normalized, err := normalizeOptions(opts)
	if err != nil {
		return nil, err
	}

	s := &Sundial{
		provider:      normalized.Provider,
		codec:         normalized.Codec,
		watchInterval: normalized.WatchInterval,
		writeMu:       sync.Mutex{},
		snapshot:      atomic.Pointer[snapshot]{},
	}

	data, err := s.provider.Load(ctx)
	if errors.Is(err, ErrNotFound) {
		s.snapshot.Store(emptySnapshot())
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sundial: load configuration: %w", err)
	}

	loaded, err := decodeSnapshot(s.codec, data)
	if err != nil {
		return nil, err
	}
	s.snapshot.Store(loaded)
	return s, nil
}

// Get returns a detached value from the in-memory configuration.
func (s *Sundial) Get(path string) any {
	value, ok := lookupPath(s.snapshot.Load().values, path)
	if !ok {
		return nil
	}
	return cloneValue(value)
}

// Raw returns a detached copy of the complete in-memory configuration.
func (s *Sundial) Raw() map[string]any {
	return cloneMap(s.snapshot.Load().values)
}

// Exists reports whether a configuration path exists in memory.
func (s *Sundial) Exists(path string) bool {
	_, ok := lookupPath(s.snapshot.Load().values, path)
	return ok
}

// String returns a string value or the zero value when the path is absent or has another type.
func (s *Sundial) String(path string) string {
	value, _ := lookupPath(s.snapshot.Load().values, path)
	result, _ := value.(string)
	return result
}

// Bool returns a bool value or the zero value when the path is absent or has another type.
func (s *Sundial) Bool(path string) bool {
	value, _ := lookupPath(s.snapshot.Load().values, path)
	result, _ := value.(bool)
	return result
}

// Int returns an integer value or the zero value when conversion is not lossless.
func (s *Sundial) Int(path string) int {
	value, _ := lookupPath(s.snapshot.Load().values, path)
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		converted := int(typed)
		if float64(converted) == typed {
			return converted
		}
	}
	return 0
}

// Unmarshal decodes a configuration subtree into dest.
func (s *Sundial) Unmarshal(path string, dest any) error {
	value, ok := lookupPath(s.snapshot.Load().values, path)
	if !ok {
		return fmt.Errorf("%w: %q", ErrNotFound, path)
	}

	data, err := s.codec.Encode(value)
	if err != nil {
		return fmt.Errorf("sundial: encode %q: %w", path, err)
	}
	if err := s.codec.Decode(data, dest); err != nil {
		return fmt.Errorf("sundial: decode %q: %w", path, err)
	}
	return nil
}
