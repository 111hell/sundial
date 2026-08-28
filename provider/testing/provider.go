// Package providertesting provides Provider implementations for Sundial tests.
package providertesting

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"github.com/sundayfun/sundial"
)

// Provider is a concurrency-safe test Provider without native watch support.
type Provider struct {
	mu                 sync.RWMutex
	data               []byte
	exists             bool
	getErr             error
	putErr             error
	putIfRevisionErr   error
	getCount           int
	putCount           int
	putIfRevisionCount int
	revision           uint64
}

// New creates a Provider. A nil document represents a missing configuration.
func New(data []byte) *Provider {
	provider := &Provider{
		data:   cloneBytes(data),
		exists: data != nil,
	}
	if data != nil {
		provider.revision = 1
	}
	return provider
}

// Get returns the current test document.
func (p *Provider) Get(context.Context) ([]byte, sundial.Metadata, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.getCount++
	if p.getErr != nil {
		return nil, sundial.Metadata{}, p.getErr
	}
	if !p.exists {
		return nil, sundial.Metadata{}, sundial.ErrNotFound
	}
	return cloneBytes(p.data), sundial.Metadata{Revision: p.currentRevision()}, nil
}

// Put writes the current test document without checking its revision.
func (p *Provider) Put(_ context.Context, data []byte) (sundial.Metadata, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.putCount++
	if p.putErr != nil {
		return sundial.Metadata{}, p.putErr
	}
	p.revision++
	p.data = cloneBytes(data)
	p.exists = true
	return sundial.Metadata{Revision: p.currentRevision()}, nil
}

// PutIfRevision replaces the current test document when
// expectedMetadata.Revision is current.
func (p *Provider) PutIfRevision(
	_ context.Context,
	data []byte,
	expectedMetadata sundial.Metadata,
) (sundial.Metadata, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.putIfRevisionCount++
	if p.putIfRevisionErr != nil {
		return sundial.Metadata{}, p.putIfRevisionErr
	}
	if expectedMetadata.Revision == "" || expectedMetadata.Revision != p.currentRevision() {
		return sundial.Metadata{}, sundial.ErrConflict
	}
	p.revision++
	p.data = cloneBytes(data)
	p.exists = true
	return sundial.Metadata{Revision: p.currentRevision()}, nil
}

// SetData simulates an external configuration change.
func (p *Provider) SetData(data []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.revision++
	p.data = cloneBytes(data)
	p.exists = data != nil
}

// SetGetError configures Get to fail.
func (p *Provider) SetGetError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.getErr = err
}

// SetPutError configures Put to fail.
func (p *Provider) SetPutError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.putErr = err
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

// PutCount returns the number of Put calls.
func (p *Provider) PutCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.putCount
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
			if err := notify(); err != nil {
				if errors.Is(err, context.Canceled) {
					return err
				}
			}
		}
	}
}

func cloneBytes(data []byte) []byte {
	return append([]byte(nil), data...)
}

func (p *Provider) currentRevision() string {
	if !p.exists {
		return ""
	}
	return strconv.FormatUint(p.revision, 10)
}
