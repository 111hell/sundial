# Sundial

[English](README.md) | [简体中文](README.zh-CN.md)

Sundial is a lightweight, extensible, type-safe configuration SDK for Go with in-memory reads and persistent writes.

## Why Sundial

- **Type-safe access** — applications read their own configuration struct instead of string paths and `any` values.
- **Fast reads** — `Get` reads only from an in-memory snapshot.
- **Persistent writes** — `Put` conditionally saves one complete typed configuration document.
- **Live updates** — `Watch` keeps memory synchronized with external changes.
- **Extensible storage and formats** — storage sources implement `Provider`; JSON works by default and other formats use codecs.

One Sundial instance manages one complete configuration document.

## Installation

```sh
go get github.com/sundayfun/sundial
```

## Quick start

Define the configuration owned by the application:

```go
type Config struct {
	Server struct {
		Host string `json:"host" yaml:"host"`
		Port int    `json:"port" yaml:"port"`
	} `json:"server" yaml:"server"`
	Debug bool `json:"debug" yaml:"debug"`
}
```

Create a Sundial with an initialized `Provider`. `New` loads and validates the
initial configuration:

```go
ctx := context.Background()

configStore, err := sundial.New[Config](
	ctx,
	provider,
)
if err != nil {
	log.Fatal(err)
}
```

### S3 provider

```go
import s3provider "github.com/sundayfun/sundial/provider/s3"

provider, err := s3provider.New(ctx, s3provider.Config{
	Region: "us-east-1",
	Bucket: "my-config-bucket",
	Key:    "production/app.json",
})
if err != nil {
	log.Fatal(err)
}

configStore, err := sundial.New[Config](ctx, provider)
if err != nil {
	log.Fatal(err)
}
```

### Read

`Get` returns an `Entry` from memory without calling the Provider. Its `Value`
is detached, and its `Metadata.Revision` belongs to the same snapshot:

```go
entry, err := configStore.Get()
if err != nil {
	log.Fatal(err)
}

fmt.Println(entry.Value.Server.Port)
```

### Write

Modify `entry.Value`, then pass the entry back to `Put` for a conditional save:

```go
entry, err := configStore.Get()
if err != nil {
	log.Fatal(err)
}

entry.Value.Server.Port = 9090
if err := configStore.Put(ctx, entry); err != nil {
	if errors.Is(err, sundial.ErrConflict) {
		// Reload, read the new revision, reapply the change, and retry if appropriate.
		log.Print("configuration changed before it could be saved")
		return
	}
	log.Fatal(err)
}
```

`Put` uses `entry.Metadata.Revision` and returns `ErrConflict` if another writer
wins. It does not merge or retry automatically.

## Watch for changes

`Watch` blocks until its context is canceled or the Provider watcher stops:

```go
go func() {
	err := configStore.Watch(
		ctx,
		sundial.WithOnChange(func() {
			entry, err := configStore.Get()
			if err != nil {
				log.Printf("read configuration: %v", err)
				return
			}
			log.Printf("configuration updated: port=%d", entry.Value.Server.Port)
		}),
		sundial.WithOnError(func(err error) {
			log.Printf("watch error: %v", err)
		}),
	)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("watch stopped: %v", err)
	}
}()
```

Providers may implement native change notifications through `Watcher`. Otherwise, Sundial polls the Provider every 30 seconds by default; use `WithWatchInterval` to change the interval.

The S3 Provider implements `Watcher` by polling object metadata. It calls
`HeadObject` on its `Config.WatchInterval`; after startup synchronization, it
downloads the object only when the ETag changes. The interval defaults to 30 seconds.

External content is decoded as the application's configuration type before publication. A failed reload keeps the last valid snapshot and is reported through `WithOnError`.

## Configuration formats

JSON is the default. Use the provided YAML codec when the source stores YAML:

```go
import yamlcodec "github.com/sundayfun/sundial/codec/yaml"

configStore, err := sundial.New[Config](
	ctx,
	provider,
	sundial.WithCodec(yamlcodec.New()),
)
```

Custom formats can implement `codec.Codec`.

## Build a provider

A Provider loads and conditionally saves one complete configuration document:

```go
type Provider interface {
	Load(ctx context.Context) ([]byte, Metadata, error)
	Save(ctx context.Context, data []byte, expectedMetadata Metadata) (Metadata, error)
}
```

The data and `Metadata` returned by `Load` must belong to the same configuration
state. `Save` must replace the configuration only when
`expectedMetadata.Revision` matches the current revision and return
`ErrConflict` otherwise. The Provider must enforce this check atomically.

For native change notifications, it can also implement:

```go
type Watcher interface {
	Watch(ctx context.Context, notify func() error) error
}
```

Concrete storage implementations live under `provider/<source>`. The core package does not depend on any storage SDK.

## References

- [koanf](https://github.com/knadh/koanf)
- [Viper](https://github.com/spf13/viper)

## Behavior

- A missing document starts with the zero value of the application's configuration type.
- A failed or conflicting `Put` leaves the current in-memory snapshot unchanged.
- A failed reload keeps the last valid snapshot.
- `Get` is safe for concurrent use. `Put` calls are serialized per instance, and stale revisions return `ErrConflict`.

## Development

The repository pins its golangci-lint version. Run the local quality gate with [Just](https://just.systems/):

```sh
just lint # Check without modifying files.
just fmt  # Apply the configured formatters.
just test # Run lint and race-enabled tests.
```

## License

[MIT](LICENSE)
