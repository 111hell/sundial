package main

import (
	"context"
	"errors"

	"github.com/sundayfun/sundial"
)

type Restore struct {
	Revision string `required:"" help:"Historical revision ID to restore."`
	Expected string `required:"" help:"Current revision ID observed before restoring."`
}

func (c *Restore) Validate() error {
	if c.Revision == "" || c.Expected == "" {
		return errors.New("--revision and --expected must not be empty")
	}
	return nil
}

func (c *Restore) Run(ctx context.Context, provider sundial.Provider) error {
	manager, err := revisionManager(provider)
	if err != nil {
		return err
	}
	data, _, err := manager.GetRevision(ctx, c.Revision)
	if err != nil {
		return err
	}
	revision, err := provider.PutIfRevision(ctx, data, c.Expected)
	if err != nil {
		return err
	}
	return writeJSON(revision)
}
