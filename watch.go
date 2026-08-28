package sundial

import (
	"context"
	"errors"
	"time"
)

func (s *Sundial[T]) watch(ctx context.Context, opts reloadOptions) {
	watcher, native := s.provider.(Watcher)
	for {
		var err error
		if native {
			err = s.runWatcher(ctx, watcher, opts)
		} else {
			err = s.poll(ctx, opts)
		}
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

func (s *Sundial[T]) runWatcher(ctx context.Context, watcher Watcher, opts reloadOptions) error {
	var reloadErr error
	err := watcher.Watch(ctx, func() error {
		reloadErr = s.autoReload(ctx, opts)
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

func (s *Sundial[T]) poll(ctx context.Context, opts reloadOptions) error {
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.autoReload(ctx, opts); errors.Is(err, context.Canceled) {
				return err
			}
		}
	}
}

func (s *Sundial[T]) autoReload(ctx context.Context, opts reloadOptions) error {
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
