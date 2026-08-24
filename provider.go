package sundial

import "context"

// Provider loads and saves one complete configuration document.
type Provider interface {
	Load(ctx context.Context) ([]byte, error)
	Save(ctx context.Context, data []byte) error
}

// Watcher is an optional Provider capability for detecting external changes.
// The callback asks Sundial to reload the configuration from the Provider.
type Watcher interface {
	Watch(ctx context.Context, notify func() error) error
}
