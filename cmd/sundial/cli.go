package main

import (
	"encoding/json/v2"
	"errors"
	"os"
	"time"
)

type CLI struct {
	Timeout time.Duration `default:"30s" help:"Operation timeout."`
	S3      S3            `              help:"Manage a configuration document in S3-compatible storage." cmd:"" name:"s3"`
}

// Commands contains provider-independent document operations.
type Commands struct {
	Get     Get     `cmd:"" help:"Read current or historical content; print its revision ID to stderr."`
	Put     Put     `cmd:"" help:"Publish a configuration document."`
	List    List    `cmd:"" help:"List published revisions, newest first, as JSON."`
	Restore Restore `cmd:"" help:"Publish historical content as a new revision."`
}

func (c *CLI) Validate() error {
	if c.Timeout <= 0 {
		return errors.New("--timeout must be positive")
	}
	return nil
}

func writeJSON(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(data, '\n'))
	return err
}
