package sundial

import (
	"context"
	"fmt"
)

// Add inserts a value at path and fails when the path already exists.
func (s *Sundial) Add(ctx context.Context, path string, value any) error {
	parts, err := splitPath(path)
	if err != nil {
		return err
	}
	return s.persist(ctx, func(values map[string]any) error {
		return setPath(values, parts, value, true)
	})
}

// Set inserts or replaces a value at path.
func (s *Sundial) Set(ctx context.Context, path string, value any) error {
	parts, err := splitPath(path)
	if err != nil {
		return err
	}
	return s.persist(ctx, func(values map[string]any) error {
		return setPath(values, parts, value, false)
	})
}

// Delete removes a value at path.
func (s *Sundial) Delete(ctx context.Context, path string) error {
	parts, err := splitPath(path)
	if err != nil {
		return err
	}
	return s.persist(ctx, func(values map[string]any) error {
		return deletePath(values, parts)
	})
}

func (s *Sundial) persist(ctx context.Context, mutate func(map[string]any) error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	values := cloneMap(s.snapshot.Load().values)
	if err := mutate(values); err != nil {
		return err
	}

	data, err := s.codec.Encode(values)
	if err != nil {
		return fmt.Errorf("sundial: encode configuration: %w", err)
	}
	next, err := decodeSnapshot(s.codec, data)
	if err != nil {
		return err
	}
	if err := s.provider.Save(ctx, data); err != nil {
		return fmt.Errorf("sundial: save configuration: %w", err)
	}

	s.snapshot.Store(next)
	return nil
}
