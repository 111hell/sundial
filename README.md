# Sundial

[English](README.md) | [简体中文](README.zh-CN.md)

Sundial is a lightweight, extensible, type-safe configuration SDK for Go with in-memory reads and persistent writes.

## Why Sundial

- **Type-safe access** — applications read their own configuration struct instead of string paths and `any` values.
- **Fast reads** — `Get` reads only from an in-memory snapshot.
- **Persistent writes** — `Put` saves one complete typed configuration document.
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

Create an instance with functional options. `New` loads and validates the initial document:

```go
ctx := context.Background()

configStore, err := sundial.New[Config](
	ctx,
	sundial.WithProvider(source),
)
if err != nil {
	log.Fatal(err)
}
```

### Read

`Get` returns a detached typed copy from memory and does not call the Provider:

```go
config, err := configStore.Get()
if err != nil {
	log.Fatal(err)
}

fmt.Println(config.Server.Port)
```

Changing the returned value does not change Sundial's in-memory state until `Put` succeeds.

### Write

Modify the typed configuration and persist the complete document:

```go
config, err := configStore.Get()
if err != nil {
	log.Fatal(err)
}

config.Server.Port = 9090
if err := configStore.Put(ctx, config); err != nil {
	log.Fatal(err)
}
```

`Put` saves to the Provider before publishing the new in-memory snapshot. A failed save leaves the previous snapshot unchanged.

## Watch for changes

`Watch` blocks until its context is canceled or the Provider watcher stops:

```go
go func() {
	err := configStore.Watch(
		ctx,
		sundial.WithOnChange(func() {
			config, err := configStore.Get()
			if err != nil {
				log.Printf("read configuration: %v", err)
				return
			}
			log.Printf("configuration updated: port=%d", config.Server.Port)
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

Providers may implement native change notifications through `Watcher`. Otherwise, Sundial polls the Provider using the interval configured by `WithWatchInterval`.

External content is decoded as the application's configuration type before publication. A failed reload keeps the last valid snapshot and is reported through `WithOnError`.

## Configuration formats

JSON is the default. Use the provided YAML codec when the source stores YAML:

```go
import yamlcodec "github.com/sundayfun/sundial/codec/yaml"

configStore, err := sundial.New[Config](
	ctx,
	sundial.WithProvider(source),
	sundial.WithCodec(yamlcodec.New()),
)
```

Custom formats can implement `codec.Codec`.

## Build a provider

A Provider loads and saves one complete configuration document:

```go
type Provider interface {
	Load(ctx context.Context) ([]byte, error)
	Save(ctx context.Context, data []byte) error
}
```

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
- `Get` returns a detached configuration; maps, slices, and pointers are not shared with the snapshot.
- A failed `Put` leaves the current in-memory snapshot unchanged.
- A failed reload keeps the last valid snapshot.
- `Put` calls on the same instance are serialized; `Get` is safe to use concurrently.
- Concurrent `Put` calls use last-write-wins semantics, including calls from the same instance.

## Development

The repository pins its golangci-lint version. Run the local quality gate with [Just](https://just.systems/):

```sh
just lint # Check without modifying files.
just fmt  # Apply the configured formatters.
just test # Run lint and race-enabled tests.
```

## License

[MIT](LICENSE)
