package store

import "errors"

// ErrConflict is the package-neutral spelling for optimistic revision conflicts.
var ErrConflict = errors.New("store: revision conflict")
