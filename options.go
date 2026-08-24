package sundial

import (
	"time"

	"github.com/sundayfun/sundial/codec"
	jsoncodec "github.com/sundayfun/sundial/codec/json"
)

const defaultWatchInterval = 30 * time.Second

type options struct {
	Provider      Provider
	Codec         codec.Codec
	WatchInterval time.Duration
}

// Option configures a Sundial instance.
type Option func(*options)

// WithProvider configures the storage provider.
func WithProvider(provider Provider) Option {
	return func(opts *options) {
		opts.Provider = provider
	}
}

// WithCodec configures the document codec. JSON is used by default.
func WithCodec(value codec.Codec) Option {
	return func(opts *options) {
		opts.Codec = value
	}
}

// WithWatchInterval configures the polling interval used when the Provider
// does not implement Watcher.
func WithWatchInterval(interval time.Duration) Option {
	return func(opts *options) {
		opts.WatchInterval = interval
	}
}

type watchOptions struct {
	// OnChange runs after changed Provider content is published to memory.
	OnChange func()
	// OnError reports reload failures and Provider watcher errors.
	OnError func(error)
}

// WatchOption configures Watch callbacks.
type WatchOption func(*watchOptions)

// WithOnChange configures a callback that runs after a changed configuration
// is published to memory.
func WithOnChange(callback func()) WatchOption {
	return func(opts *watchOptions) {
		opts.OnChange = callback
	}
}

// WithOnError configures a callback for reload and Provider watcher errors.
func WithOnError(callback func(error)) WatchOption {
	return func(opts *watchOptions) {
		opts.OnError = callback
	}
}

func normalizeOptions(optionFunctions []Option) (options, error) {
	normalized := options{
		Provider:      nil,
		Codec:         jsoncodec.New(),
		WatchInterval: defaultWatchInterval,
	}
	for _, option := range optionFunctions {
		if option != nil {
			option(&normalized)
		}
	}

	if normalized.Provider == nil {
		return options{}, ErrProviderRequired
	}
	if normalized.Codec == nil {
		normalized.Codec = jsoncodec.New()
	}
	if normalized.WatchInterval <= 0 {
		normalized.WatchInterval = defaultWatchInterval
	}
	return normalized, nil
}

func normalizeWatchOptions(optionFunctions []WatchOption) watchOptions {
	normalized := watchOptions{
		OnChange: nil,
		OnError:  nil,
	}
	for _, option := range optionFunctions {
		if option != nil {
			option(&normalized)
		}
	}
	return normalized
}
