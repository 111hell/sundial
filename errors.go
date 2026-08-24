package sundial

import "errors"

var (
	// ErrNotFound reports a missing configuration document.
	ErrNotFound = errors.New("sundial: not found")
	// ErrProviderRequired reports a missing Provider option.
	ErrProviderRequired = errors.New("sundial: provider is required")
)
