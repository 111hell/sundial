package sundial

import "errors"

var (
	// ErrNotFound reports a missing configuration document.
	ErrNotFound = errors.New("sundial: not found")
	// ErrConflict reports that a write was based on a stale configuration revision.
	ErrConflict = errors.New("sundial: conflict")
)

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

func IsConflict(err error) bool {
	return errors.Is(err, ErrConflict)
}
