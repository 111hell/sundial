package main

import (
	"errors"

	"github.com/sundayfun/sundial"
)

func revisionManager(provider sundial.Provider) (sundial.RevisionManager, error) {
	manager, ok := provider.(sundial.RevisionManager)
	if !ok {
		return nil, errors.New("selected provider does not support revision history")
	}
	return manager, nil
}
