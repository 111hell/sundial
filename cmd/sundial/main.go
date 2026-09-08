// Command sundial manages versioned configuration documents.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sundial:", err)
		os.Exit(1)
	}
}

func run() error {
	var cli CLI
	parser, err := kong.New(&cli,
		kong.Name("sundial"),
		kong.Description("Manage versioned configuration documents."),
		kong.UsageOnError(),
	)
	if err != nil {
		return err
	}
	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"--help"}
	}
	command, err := parser.Parse(args)
	if err != nil {
		return err
	}

	command.BindTo(context.Background(), (*context.Context)(nil))
	return command.Run()
}
