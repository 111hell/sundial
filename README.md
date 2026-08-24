# Sundial

[English](README.md) | [简体中文](README.zh-CN.md)

Sundial is a lightweight, extensible configuration SDK for Go with in-memory reads and persistent writes.

## Why Sundial

- **Fast reads** — getters read only from an in-memory snapshot.
- **Persistent writes** — `Add`, `Set`, and `Delete` save changes to the configured source.
- **Live updates** — `Watch` keeps memory synchronized with external changes.
- **Extensible storage** — files, S3, databases, and other sources can implement `Provider`.
- **Flexible formats** — JSON works out of the box; other formats use ready-made codecs.

One Sundial instance manages one complete configuration document.

## Installation

```sh
go get github.com/sundayfun/sundial
```

## Quick start

Assume the provider already contains `{"server":{"port":8080}}`:

```go
ctx := context.Background()

config, err := sundial.New(ctx, sundial.Options{
	Provider: source,
})
if err != nil {
	log.Fatal(err)
}
```

### Read-only

`Get` reads from memory and does not call `Provider.Save`:

```go
port := config.Get("server.port")
fmt.Println(port) // 8080
```

The same instance can use `Get`, `Exists`, `Unmarshal`, and `Watch` without performing any writes.

### Read and write

`Set` saves the change to the provider and then updates memory. A following `Get` returns the new value:

```go
if err := config.Set(ctx, "server.port", 9090); err != nil {
	log.Fatal(err)
}

port := config.Get("server.port")
fmt.Println(port) // 9090
```

Configuration keys and their value types are defined by the application. `Get` returns the value currently stored in memory; use `Unmarshal` when a typed configuration struct is needed.

Available operations:

```go
value := config.Get("server.host")
exists := config.Exists("server.port")

err := config.Add(ctx, "features.search", true) // Fails if the path exists.
err = config.Set(ctx, "server.port", 9090)      // Adds or replaces a value.
err = config.Delete(ctx, "features.legacy")    // Removes a value.
```

## Watch for changes

`Watch` blocks until its context is canceled or the provider watcher stops.

```go
go func() {
	err := config.Watch(ctx, sundial.WatchOptions{
		OnChange: func() { log.Println("configuration updated") },
		OnError:  func(err error) { log.Printf("watch error: %v", err) },
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("watch stopped: %v", err)
	}
}()
```

`OnChange` runs after the new in-memory snapshot is available. `OnError` reports reload or provider-watcher failures. A failed reload keeps the last valid snapshot, and polling continues.

Providers may implement native change notifications through `Watcher`. Otherwise, Sundial polls the provider using `WatchInterval`.

## Configuration formats

JSON is the default and requires no configuration. Use the provided YAML codec when the source stores YAML:

```go
import yamlcodec "github.com/sundayfun/sundial/codec/yaml"

config, err := sundial.New(ctx, sundial.Options{
	Provider: source,
	Codec:    yamlcodec.New(),
})
```

Custom formats can implement the `codec.Codec` interface.

## Build a provider

A provider loads and saves one complete configuration document:

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

- Configuration paths use dot notation, such as `server.port`.
- A failed write leaves the current in-memory snapshot unchanged.
- A failed reload keeps the last valid snapshot.
- Writes on the same instance are serialized; reads are safe to use concurrently.
- Concurrent writers across multiple instances use last-write-wins semantics.

## License

[MIT](LICENSE)
