package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"

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
	port := flag.Int("port", -1, "update the server port; negative is read-only")
	watch := flag.Bool("watch", false, "watch for external changes")
	flag.Parse()

	ctx := context.Background()
	provider, err := s3provider.New(ctx, &s3provider.Config{
		Region:        os.Getenv("AWS_REGION"),
		Bucket:        os.Getenv("SUNDIAL_S3_BUCKET"),
		Key:           os.Getenv("SUNDIAL_S3_KEY"),
		Endpoint:      "",
		UsePathStyle:  false,
		WatchInterval: 0,
	})
	if err != nil {
		log.Fatal(err)
	}

	store, err := sundial.New[config](ctx, provider)
	if err != nil {
		log.Fatal(err)
	}

	entry, err := store.Get()
	if err != nil {
		log.Fatal(err)
	}
	printEntry("loaded", entry)

	if *port >= 0 {
		entry.Value.Server.Port = *port
		putErr := store.Put(ctx, entry)
		if putErr != nil {
			if errors.Is(putErr, sundial.ErrConflict) {
				log.Fatal("configuration changed before it could be saved")
			}
			log.Fatal(putErr)
		}

		entry, err = store.Get()
		if err != nil {
			log.Fatal(err)
		}
		printEntry("updated", entry)
	}

	if !*watch {
		return
	}

	watchErr := store.Watch(
		ctx,
		sundial.WithOnChange(func() {
			entry, getErr := store.Get()
			if getErr != nil {
				log.Printf("get changed configuration: %v", getErr)
				return
			}
			printEntry("changed", entry)
		}),
		sundial.WithOnError(func(watchErr error) {
			log.Printf("watch: %v", watchErr)
		}),
	)
	if watchErr != nil {
		log.Fatal(watchErr)
	}
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
