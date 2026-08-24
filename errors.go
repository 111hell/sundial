package sundial

import "errors"

var (
	// ErrNotFound reports a missing configuration document or path.
	ErrNotFound = errors.New("sundial: not found")
	// ErrAlreadyExists reports an Add targeting an existing path.
	ErrAlreadyExists = errors.New("sundial: already exists")
	// ErrInvalidPath reports an empty path or an empty path segment.
	ErrInvalidPath = errors.New("sundial: invalid path")
	// ErrPathConflict reports traversal through a non-map value.
	ErrPathConflict = errors.New("sundial: path conflict")
	// ErrProviderRequired reports a missing Provider option.
	ErrProviderRequired = errors.New("sundial: provider is required")
)
