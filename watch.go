package sundial

import (
	"context"
	"errors"
	"time"
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

func (s *Sundial[T]) runWatch(ctx context.Context, opts reloadOptions) {
	for {
		err := s.watch(ctx, opts)
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return
		}

		timer := time.NewTimer(opts.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Sundial[T]) watch(ctx context.Context, opts reloadOptions) error {
	if watcher, ok := s.provider.(Watcher); ok {
		var reloadErr error
		err := watcher.Watch(ctx, func() error {
			reloadErr = s.watchReload(ctx, opts)
			return reloadErr
		})
		if err != nil && !errors.Is(err, context.Canceled) &&
			!errors.Is(err, reloadErr) {
			s.logger.ErrorContext(
				ctx,
				"automatic reload failed",
				"operation",
				"watch provider",
				"error",
				err,
			)
			if opts.OnError != nil {
				opts.OnError(err)
			}
		}
		return err
	}

	ticker := time.NewTicker(opts.Interval)
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

func (s *Sundial[T]) watchReload(ctx context.Context, opts reloadOptions) error {
	changed, err := s.reload(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		s.logger.ErrorContext(
			ctx,
			"automatic reload failed",
			"operation",
			"reload configuration",
			"error",
			err,
		)
		if opts.OnError != nil {
			opts.OnError(err)
		}
		return err
	}
	if changed && opts.OnChange != nil {
		opts.OnChange()
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
