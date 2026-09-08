package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sundayfun/sundial"
)

type Get struct {
	Revision string `help:"Historical revision ID; omit to read the current revision."`
}

func (c *Get) Run(ctx context.Context, provider sundial.Provider) error {
	var data []byte
	var revision sundial.Revision
	var err error
	if c.Revision == "" {
		data, revision, err = provider.Get(ctx)
	} else {
		manager, capabilityErr := revisionManager(provider)
		if capabilityErr != nil {
			return capabilityErr
		}
		data, revision, err = manager.GetRevision(ctx, c.Revision)
	}
	if err != nil {
		return err
	}
	if _, err = os.Stdout.Write(data); err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, revision.ID)
	return err
}
