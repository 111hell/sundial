package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sundayfun/sundial"
)

type Put struct {
	File     string `required:"" short:"f" help:"Configuration file; - reads stdin."`
	Expected string `required:""           help:"Current revision ID observed before editing."                       xor:"publication"`
	Force    bool   `                      help:"Publish without an expected revision, including first publication." xor:"publication"`
}

func (c *Put) Validate() error {
	if c.File == "" {
		return errors.New("--file must not be empty")
	}
	if !c.Force && c.Expected == "" {
		return errors.New("provide --expected with a nonempty revision ID or --force")
	}
	return nil
}

func (c *Put) Run(ctx context.Context, provider sundial.Provider) error {
	var data []byte
	var err error
	if c.File == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(c.File)
	}
	if err != nil {
		return fmt.Errorf("read configuration: %w", err)
	}
	var revision sundial.Revision
	if c.Force {
		revision, err = provider.Put(ctx, data)
	} else {
		revision, err = provider.PutIfRevision(ctx, data, c.Expected)
	}
	if err != nil {
		return err
	}
	return writeJSON(revision)
}
