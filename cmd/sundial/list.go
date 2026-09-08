package main

import (
	"context"
	"errors"

	"github.com/sundayfun/sundial"
)

type List struct {
	Limit  int `default:"256" help:"Maximum number of revisions."`
	Offset int `default:"0"   help:"Number of newest revisions to skip."`
}

func (c *List) Validate() error {
	if c.Limit <= 0 || c.Offset < 0 {
		return errors.New("--limit must be positive and --offset must be nonnegative")
	}
	return nil
}

func (c *List) Run(ctx context.Context, provider sundial.Provider) error {
	manager, err := revisionManager(provider)
	if err != nil {
		return err
	}
	revisions, err := manager.ListRevisions(ctx, sundial.ListRevisionsOptions{Limit: c.Limit, Offset: c.Offset})
	if err != nil {
		return err
	}
	return writeJSON(revisions)
}
