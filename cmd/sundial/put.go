package main

import (
	"context"

	"github.com/sundayfun/sundial"
)

type Put struct {
	File             []byte `required:"" short:"f" type:"filecontent" help:"Configuration file; - reads stdin."`
	ExpectedRevision string `required:""                              help:"Current revision ID observed before editing."                       xor:"publication"`
	Force            bool   `required:""                              help:"Publish without an expected revision, including first publication." xor:"publication"`
}

func (c *Put) Run(ctx context.Context, provider sundial.Provider) error {
	var revision sundial.Revision
	var err error
	if c.Force {
		revision, err = provider.Put(ctx, c.File)
	} else {
		revision, err = provider.PutIfRevision(ctx, c.File, c.ExpectedRevision)
	}
	if err != nil {
		return err
	}
	return writeJSON(revision)
}
