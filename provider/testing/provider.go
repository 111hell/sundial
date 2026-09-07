// Package providertesting provides Provider implementations for Sundial tests.
package providertesting

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/sundayfun/sundial"
)

// Provider is a concurrency-safe test Provider without native watch support.
type Provider struct {
	mu                 sync.RWMutex
	data               []byte
	exists             bool
	putIfRevisionErr   error
	getCount           int
	putIfRevisionCount int
	revisionNumber     uint64
	revision           sundial.Revision
}

// New creates a Provider. A nil document represents a missing configuration.
func New(data []byte) *Provider {
	provider := &Provider{}
	if data != nil {
		provider.store(data, true)
	}
	return provider
}

// Get returns the current test document.
func (p *Provider) Get(context.Context) ([]byte, sundial.Revision, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.getCount++
	if !p.exists {
		return nil, sundial.Revision{}, sundial.ErrNotFound
	}
	return cloneBytes(p.data), p.revision, nil
}

// Put writes the current test document without checking its revision.
func (p *Provider) Put(_ context.Context, data []byte) (sundial.Revision, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.store(data, true)
	return p.revision, nil
}

// PutIfRevision replaces the current test document when
// expectedRevisionID is current.
func (p *Provider) PutIfRevision(
	_ context.Context,
	data []byte,
	expectedRevisionID string,
) (sundial.Revision, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.putIfRevisionCount++
	if p.putIfRevisionErr != nil {
		return sundial.Revision{}, p.putIfRevisionErr
	}
	if expectedRevisionID == "" || expectedRevisionID != p.revision.ID || !p.exists {
		return sundial.Revision{}, sundial.ErrConflict
	}
	p.store(data, true)
	return p.revision, nil
}

// SetData simulates an external configuration change.
func (p *Provider) SetData(data []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.store(data, data != nil)
}

// SetPutIfRevisionError configures PutIfRevision to fail.
func (p *Provider) SetPutIfRevisionError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.putIfRevisionErr = err
}

// Data returns a copy of the current test document.
func (p *Provider) Data() []byte {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return cloneBytes(p.data)
}

// GetCount returns the number of Get calls.
func (p *Provider) GetCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.getCount
}

// PutIfRevisionCount returns the number of PutIfRevision calls.
func (p *Provider) PutIfRevisionCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.putIfRevisionCount
}

// WatchProvider adds native watch support to Provider.
type WatchProvider struct {
	*Provider

	changes chan struct{}
}

// NewWatcher creates a Provider with native watch support.
func NewWatcher(data []byte) *WatchProvider {
	return &WatchProvider{
		Provider: New(data),
		changes:  make(chan struct{}, 1),
	}
}

// Change simulates an external change and notifies Watch.
func (p *WatchProvider) Change(data []byte) {
	p.SetData(data)
	select {
	case p.changes <- struct{}{}:
	default:
	}
}

// Watch waits for simulated changes.
func (p *WatchProvider) Watch(ctx context.Context, notify func() error) error {
	if err := notify(); errors.Is(err, context.Canceled) {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.changes:
			if err := notify(); errors.Is(err, context.Canceled) {
				return err
			}
		}
	}
}

func cloneBytes(data []byte) []byte {
	return append([]byte(nil), data...)
}

func (p *Provider) store(data []byte, exists bool) {
	parentID := ""
	if p.exists {
		parentID = p.revision.ID
	}
	p.revisionNumber++
	p.revision = sundial.Revision{
		ID:        strconv.FormatUint(p.revisionNumber, 10),
		ParentID:  parentID,
		CreatedAt: time.Now().UTC(),
	}
	p.data = cloneBytes(data)
	p.exists = exists
}
