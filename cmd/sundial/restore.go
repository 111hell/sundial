package main

import (
	"context"

	"github.com/sundayfun/sundial"
)

type Restore struct {
	Revision         string `required:"" help:"Historical revision ID to restore."`
	ExpectedRevision string `required:"" help:"Current revision ID observed before restoring."`
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
	revision, err := provider.PutIfRevision(ctx, data, c.ExpectedRevision)
	if err != nil {
		return err
	}
	return writeJSON(revision)
}
