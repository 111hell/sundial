package sundial

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Reload replaces the in-memory state when the Provider content changed.
func (s *Sundial) Reload(ctx context.Context) error {
	_, err := s.reload(ctx)
	return err
}

// Watch blocks until ctx is canceled or a native Provider watcher stops.
func (s *Sundial) Watch(ctx context.Context, opts WatchOptions) error {
	// Close the gap between New's initial load and watcher registration.
	if err := s.watchReload(ctx, opts); err != nil {
		return err
	}

	if watcher, ok := s.provider.(Watcher); ok {
		err := watcher.Watch(ctx, func() error {
			return s.watchReload(ctx, opts)
		})
		if err != nil && !errors.Is(err, context.Canceled) && opts.OnError != nil {
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
			_ = s.watchReload(ctx, opts)
		}
	}
}

func (s *Sundial) watchReload(ctx context.Context, opts WatchOptions) error {
	changed, err := s.reload(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		if opts.OnError != nil {
			opts.OnError(err)
		}
		return nil
	}
	if changed && opts.OnChange != nil {
		opts.OnChange()
	}
	return nil
}

func (s *Sundial) reload(ctx context.Context) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	data, err := s.provider.Load(ctx)
	if errors.Is(err, ErrNotFound) {
		data = nil
	} else if err != nil {
		return false, fmt.Errorf("sundial: load configuration: %w", err)
	}

	next, err := decodeSnapshot(s.codec, data)
	if err != nil {
		return false, err
	}
	if next.hash == s.snapshot.Load().hash {
		return false, nil
	}

	s.snapshot.Store(next)
	return true, nil
}
