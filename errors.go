package sundial

import "errors"

// ErrNotFound reports a missing configuration document.
var ErrNotFound = errors.New("sundial: not found")

// ErrConflict reports that a write was based on a stale configuration revision.
var ErrConflict = errors.New("sundial: conflict")
