# Sundial

[![Go Reference](https://pkg.go.dev/badge/github.com/sundayfun/sundial.svg)](https://pkg.go.dev/github.com/sundayfun/sundial)

[简体中文](README.zh-CN.md)

Sundial is a lightweight, extensible, type-safe configuration SDK for Go with
in-memory reads, persistent writes, and live updates.

## Why Sundial

- **Type-safe access** — applications read their own configuration struct instead of string paths and `any` values.
- **Fast reads** — `Get` copies an already parsed in-memory value without decoding.
- **Persistent writes** — `Put` conditionally saves one complete typed configuration document.
- **Version history** — browse historical revisions and restore configuration.
- **Live updates** — automatic reload keeps memory synchronized with external changes.
- **Extensible storage and formats** — storage sources implement `Provider`; JSON works by default and other formats use codecs.

One `Client` manages one complete configuration document.

## In action

![Sundial in-memory reads, concurrent write protection, and revision restore](docs/images/sundial-overview.png)

If two callers read revision A, the first successful write creates B; the second
write based on A returns `ErrConflict`. Later, restoring A while C is current
creates a new revision D with A's content, preserving the full history.

## Installation

```sh
go get github.com/sundayfun/sundial
```

## Quick start

This example reads and updates an existing JSON configuration in S3.
For credentials and initial publication, see the [S3 example](examples/s3).

```go
package main

import (
    "context"
    "fmt"
    "log"

    s3provider "github.com/sundayfun/sundial/provider/s3"
)

type Config struct {
    Port int `json:"port"`
}

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    store, err := s3provider.New[Config](ctx, &s3provider.Config{
        Region: "us-east-1",
        StorageConfig: s3provider.StorageConfig{
            Bucket:             "my-config-bucket",
            CurrentRevisionKey: "production/app/metadata.yaml",
            RevisionKeyPrefix:  "production/app/",
        },
    }, func(v Config) Config { return v })
    if err != nil {
        log.Fatal(err)
    }

    entry := store.Get()
    fmt.Println(entry.Value.Port)

    entry.Value.Port = 9090
    if _, err := store.Put(ctx, entry); err != nil {
        log.Fatal(err)
    }
}
```

Always provide a non-nil `clone` function; otherwise `New` returns `ErrCloneRequired`.
For configurations containing only value fields, use `func(v Config) Config { return v }`,
as above. For maps, slices, pointers or other mutable references, copy all referenced data.
The function must be safe for concurrent calls, must not change its input, and must not
encode or decode.

`Get` returns an independent copy with its revision. `Put` saves the complete document;
a stale revision returns `ErrConflict`, without automatic merging or retries.
Canceling the context stops automatic reload.
Failed writes or reloads leave the last valid in-memory configuration unchanged.

## Documentation

- [S3 example](examples/s3) — setup, initial publication, and conditional writes.
- [API reference](https://pkg.go.dev/github.com/sundayfun/sundial) — revision history, codecs, and reload callbacks.

## License

[MIT](LICENSE)
