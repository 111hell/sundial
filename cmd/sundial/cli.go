package main

import (
	"encoding/json/v2"
	"os"
)

type CLI struct {
	S3 S3 `help:"Manage a configuration document in S3-compatible storage." cmd:"" name:"s3"`
}

// Commands contains provider-independent document operations.
type Commands struct {
	Get     Get     `cmd:"" help:"Read current or historical content; print its revision ID to stderr."`
	Put     Put     `cmd:"" help:"Publish a configuration document."`
	List    List    `cmd:"" help:"List published revisions, newest first, as JSON."`
	Restore Restore `cmd:"" help:"Publish historical content as a new revision."`
}

func writeJSON(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(data, '\n'))
	return err
}
