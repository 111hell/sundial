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
	mu        sync.RWMutex
	data      []byte
	exists    bool
	loadErr   error
	putErr    error
	loadCount int
	putCount  int
	revision  uint64
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

// Load returns the current test document.
func (p *Provider) Load(context.Context) ([]byte, sundial.Metadata, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.loadCount++
	if p.loadErr != nil {
		return nil, sundial.Metadata{}, p.loadErr
	}
	if !p.exists {
		return nil, sundial.Metadata{}, sundial.ErrNotFound
	}
	return cloneBytes(p.data), sundial.Metadata{Revision: p.currentRevision()}, nil
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

	p.putCount++
	if p.putErr != nil {
		return sundial.Metadata{}, p.putErr
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

// SetLoadError configures Load to fail.
func (p *Provider) SetLoadError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.loadErr = err
}

// SetPutError configures Put to fail.
func (p *Provider) SetPutError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.putErr = err
}

// Data returns a copy of the current test document.
func (p *Provider) Data() []byte {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return cloneBytes(p.data)
}

// LoadCount returns the number of Load calls.
func (p *Provider) LoadCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.loadCount
}

// PutCount returns the number of Put calls.
func (p *Provider) PutCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.putCount
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
