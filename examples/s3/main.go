package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/sundayfun/sundial"
	s3provider "github.com/sundayfun/sundial/provider/s3"
)

type config struct {
	Server serverConfig `json:"server"`
	Debug  bool         `json:"debug"`
}

type serverConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	port := flag.Int("port", -1, "update the server port; negative is read-only")
	watch := flag.Bool("watch", false, "wait and print external changes")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := s3provider.New[config](
		ctx,
		&s3provider.Config{
			Region:       os.Getenv("AWS_REGION"),
			Bucket:       os.Getenv("SUNDIAL_S3_BUCKET"),
			PathPrefix:   os.Getenv("SUNDIAL_S3_PATH_PREFIX"),
			Key:          os.Getenv("SUNDIAL_S3_KEY"),
			Endpoint:     "",
			UsePathStyle: false,
			// Zero uses the default 30-second interval.
			WatchInterval: 0,
		},
		// Optional: observe automatic reload changes and errors.
		sundial.WithOnChange(func() {
			log.Print("configuration reloaded")
		}),
		sundial.WithOnError(func(reloadErr error) {
			log.Printf("automatic reload: %v", reloadErr)
		}),
	)
	if err != nil {
		return err
	}

	entry, err := store.Get()
	if err != nil {
		return fmt.Errorf("get loaded configuration: %w", err)
	}
	printEntry("loaded", entry)

	if *port >= 0 {
		entry.Value.Server.Port = *port
		putErr := store.Put(ctx, entry)
		if putErr != nil {
			if sundial.IsConflict(putErr) {
				return errors.New("configuration changed before it could be saved")
			}
			return fmt.Errorf("put configuration: %w", putErr)
		}

		entry, err = store.Get()
		if err != nil {
			return fmt.Errorf("get updated configuration: %w", err)
		}
		printEntry("updated", entry)
	}

	if !*watch {
		return nil
	}

	// Keep automatic reload running until SIGINT or SIGTERM cancels ctx.
	<-ctx.Done()
	return nil
}

func printEntry(event string, entry sundial.Entry[config]) {
	log.Printf(
		"%s: host=%s port=%d debug=%t revision=%s",
		event,
		entry.Value.Server.Host,
		entry.Value.Server.Port,
		entry.Value.Debug,
		entry.Metadata.Revision,
	)
}
