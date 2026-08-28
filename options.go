package sundial

import (
	"log/slog"
	"time"

	"github.com/sundayfun/sundial/codec"
	jsoncodec "github.com/sundayfun/sundial/codec/json"
)

const defaultReloadInterval = 30 * time.Second

type options struct {
	Codec  codec.Codec
	Logger *slog.Logger
	Reload reloadOptions
}

type reloadOptions struct {
	Interval time.Duration
	OnChange func()
	OnError  func(error)
}

// Option configures a Sundial instance.
type Option func(*options)

// WithCodec configures the document codec. JSON is used by default.
func WithCodec(value codec.Codec) Option {
	return func(opts *options) {
		if value != nil {
			opts.Codec = value
		}
	}
}

// WithLogger configures structured debug and automatic reload error logging.
func WithLogger(logger *slog.Logger) Option {
	return func(opts *options) {
		if logger != nil {
			opts.Logger = logger
		}
	}
}

// WithReloadInterval sets the polling and watcher retry interval.
func WithReloadInterval(interval time.Duration) Option {
	return func(opts *options) {
		if interval > 0 {
			opts.Reload.Interval = interval
		}
	}
}

// WithOnChange sets the callback run after a changed configuration is published.
func WithOnChange(callback func()) Option {
	return func(opts *options) {
		opts.Reload.OnChange = callback
	}
}

// WithOnError sets the automatic reload error callback.
func WithOnError(callback func(error)) Option {
	return func(opts *options) {
		opts.Reload.OnError = callback
	}
}

func normalizeOptions(optionFunctions []Option) options {
	normalized := options{
		Codec:  jsoncodec.New(),
		Logger: slog.Default(),
		Reload: reloadOptions{
			Interval: defaultReloadInterval,
			OnChange: nil,
			OnError:  nil,
		},
	}
	for _, option := range optionFunctions {
		if option != nil {
			option(&normalized)
		}
	}

	return normalized
}
