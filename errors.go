package sundial

import "errors"

var (
	// ErrNotFound reports a missing configuration document.
	ErrNotFound = errors.New("sundial: not found")
	// ErrConflict reports that a write was based on a stale configuration revision.
	ErrConflict = errors.New("sundial: conflict")
	// ErrEmptyDocument reports an empty configuration document.
	ErrEmptyDocument = errors.New("sundial: empty configuration document")
)

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

func IsConflict(err error) bool {
	return errors.Is(err, ErrConflict)
}
