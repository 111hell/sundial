# Sundial

[![Go Reference](https://pkg.go.dev/badge/github.com/sundayfun/sundial.svg)](https://pkg.go.dev/github.com/sundayfun/sundial)

[简体中文](README.zh-CN.md)

Sundial is a lightweight, extensible, type-safe configuration SDK for Go with
in-memory reads, persistent writes, and live updates.

## Why Sundial

- **Type-safe access** — applications read their own configuration struct instead of string paths and `any` values.
- **Fast reads** — `Get` returns the already parsed in-memory snapshot without decoding or deep copying.
- **Persistent writes** — `Update` conditionally saves one complete typed configuration document.
- **Version history** — browse historical revisions and restore configuration.
- **Live updates** — automatic reload keeps memory synchronized with external changes.
- **Extensible storage and formats** — storage sources implement `Provider`; JSON works by default and other formats use codecs.

One `Client` manages one complete configuration document.

## In action

![Sundial in-memory reads, concurrent write protection, and revision restore](docs/images/sundial-overview.png)

If two clients cache revision A, the first successful update creates B; the second
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
    }, func(config Config) Config { return config })
    if err != nil {
        log.Fatal(err)
    }

    entry := store.Get()
    fmt.Println(entry.Value.Port)

    if _, err := store.Update(ctx, func(config *Config) error {
        config.Port = 9090
        return nil
    }); err != nil {
        log.Fatal(err)
    }
}
```

`Get` returns a shared read-only configuration snapshot with its revision, without
an error return, encoding, decoding or deep copying. Do not modify its value,
including nested maps, slices and pointers. Go does not enforce this read-only
contract; callers must follow it. Successful `Update` and `RestoreRevision` results and values
passed to `OnChange` have the same shared read-only contract.

For ordinary edits, call `Update(ctx, func(*T) error)` directly; no preceding `Get`
is needed. It clones the current snapshot into an independent draft while retaining its
revision, applies the callback, and publishes with revision protection.
A decode, callback or publication failure leaves the cached snapshot unchanged.
The returned entry is shared read-only. Do not modify the callback's draft
concurrently with the call. The callback must not call `Update`, `Reload`
or `RestoreRevision` on the same client because they acquire the same write lock.

`Draft() (Entry[T], error)` creates an independent editable copy of the current
snapshot while retaining its revision. With a clone function it does not use the
codec; with nil it encodes and decodes the current value and may return an error. Editing this copy does
not change or publish the cached configuration; use `Update` to persist changes.

`New(ctx, provider, clone, opts...)` takes a `func(T) T` clone function for the
root configuration type as a positional argument. Passing nil selects a default
copy through encoding and decoding the current snapshot value, so callers can
use `sundial.New[Config](ctx, provider, nil)` without writing a clone function.
The default copy follows the codec's serialization and decoding semantics.
A supplied clone function must deeply copy all mutable
maps, slices and pointers without modifying its input. Nested types need not be
passed separately; the root clone function owns the complete copy. The quick
start uses a value copy because its configuration contains only an integer.

Before publishing, `Update` encodes and decodes the edited value to validate it
and isolate the stored snapshot from the callback's draft. Each codec `Decode`
call must create independent mutable objects, without retaining or reusing their
references; `Encode` must not modify its input.

`Update` saves the complete document using compare-and-swap (CAS) against the
cached snapshot's revision. If another client publishes a new revision first,
the stale write returns `ErrConflict`, without automatic merging or retries.
Canceling the context stops automatic reload.
Failed writes or reloads leave the last valid in-memory configuration unchanged.

## Documentation

- [S3 example](examples/s3) — setup, initial publication, and conditional writes.
- [API reference](https://pkg.go.dev/github.com/sundayfun/sundial) — revision history, codecs, and reload callbacks.

## License

[MIT](LICENSE)
