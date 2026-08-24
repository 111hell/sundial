package sundial

import (
	"time"

	"github.com/sundayfun/sundial/codec"
	jsoncodec "github.com/sundayfun/sundial/codec/json"
)

const defaultWatchInterval = 30 * time.Second

// Options configures one Sundial instance.
type Options struct {
	Provider      Provider
	Codec         codec.Codec
	WatchInterval time.Duration
}

// WatchOptions configures change and error callbacks for Watch.
type WatchOptions struct {
	// OnChange runs after changed Provider content is published to memory.
	OnChange func()
	// OnError reports reload failures and Provider watcher errors.
	OnError func(error)
}

func normalizeOptions(opts Options) (Options, error) {
	if opts.Provider == nil {
		return Options{}, ErrProviderRequired
	}
	if opts.Codec == nil {
		opts.Codec = jsoncodec.New()
	}
	if opts.WatchInterval <= 0 {
		opts.WatchInterval = defaultWatchInterval
	}
	return opts, nil
}
