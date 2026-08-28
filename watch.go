package sundial

import (
	"context"
	"errors"
	"time"
)

// Reload replaces the in-memory state when the Provider content changed.
func (s *Sundial[T]) Reload(ctx context.Context) error {
	_, err := s.reload(ctx)
	return err
}

// Watch blocks until ctx is canceled or a native Provider watcher stops.
func (s *Sundial[T]) Watch(ctx context.Context, optionFunctions ...WatchOption) error {
	opts := normalizeWatchOptions(optionFunctions)

	// Close the gap between New's initial load and watcher registration.
	if err := s.watchReload(ctx, opts); errors.Is(err, context.Canceled) {
		return err
	}

	if watcher, ok := s.provider.(Watcher); ok {
		var reloadErr error
		err := watcher.Watch(ctx, func() error {
			reloadErr = s.watchReload(ctx, opts)
			return reloadErr
		})
		if err != nil && !errors.Is(err, context.Canceled) &&
			!errors.Is(err, reloadErr) && opts.OnError != nil {
			opts.OnError(err)
		}
		return err
	}

	ticker := time.NewTicker(s.watchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.watchReload(ctx, opts); errors.Is(err, context.Canceled) {
				return err
			}
		}
	}
}

func (s *Sundial[T]) watchReload(ctx context.Context, opts watchOptions) error {
	changed, err := s.reload(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		if opts.OnError != nil {
			opts.OnError(err)
		}
		return err
	}
	if changed && opts.OnChange != nil {
		opts.OnChange()
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
