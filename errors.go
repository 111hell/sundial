package sundial

import "errors"

// ErrNotFound reports a missing configuration document.
var ErrNotFound = errors.New("sundial: not found")
