package sundial

import (
	"context"
	"errors"
)

// Reload replaces the in-memory state when the Provider content changed.
func (s *Sundial[T]) Reload(ctx context.Context) error {
	changed, err := s.reload(ctx)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			s.logger.ErrorContext(ctx, "reload configuration", "error", err)
		}
		return err
	}
	if changed {
		current := s.snapshot.Load()
		s.logger.DebugContext(ctx, "reloaded configuration", "revision", current.metadata.Revision)
	}
	return nil
}

func (s *Sundial[T]) reload(ctx context.Context) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	next, err := s.loadSnapshot(ctx)
	if err != nil {
		return false, err
	}
	current := s.snapshot.Load()
	if next.hash == current.hash {
		if next.metadata.Revision != current.metadata.Revision {
			s.snapshot.Store(next)
		}
		return false, nil
	}

	s.snapshot.Store(next)
	return true, nil
}
