# Sundial

[English](README.md) | [简体中文](README.zh-CN.md)

Sundial 是一个轻量、可扩展的 Go 配置 SDK，提供内存读取和持久化写入能力。

## 为什么选择 Sundial

- **快速读取**：所有 getter 只读取内存快照。
- **持久化写入**：`Add`、`Set`、`Delete` 会将变更保存到配置源。
- **实时更新**：`Watch` 将外部变更同步到内存。
- **存储可扩展**：文件、S3、数据库等配置源均可实现 `Provider`。
- **格式可扩展**：默认支持 JSON，其他格式使用现成 Codec。

一个 Sundial 实例管理一份完整配置文档。

## 安装

```sh
go get github.com/sundayfun/sundial
```

## 快速开始

假设 Provider 中已有配置 `{"server":{"port":8080}}`：

```go
ctx := context.Background()

config, err := sundial.New(ctx, sundial.Options{
	Provider: source,
})
if err != nil {
	log.Fatal(err)
}
```

### 只读使用

`Get` 只读取内存，不会调用 `Provider.Save`：

```go
port := config.Get("server.port")
fmt.Println(port) // 8080
```

同一个实例可以只使用 `Get`、`Exists`、`Unmarshal` 和 `Watch`，不执行任何写入。

### 读写使用

`Set` 先将变更保存到 Provider，成功后更新内存。随后调用 `Get` 会得到新值：

```go
if err := config.Set(ctx, "server.port", 9090); err != nil {
	log.Fatal(err)
}

port := config.Get("server.port")
fmt.Println(port) // 9090
```

配置键及其值类型由使用方定义。`Get` 返回当前内存中的配置值；需要业务结构体时使用 `Unmarshal`。

可用操作：

```go
value := config.Get("server.host")
exists := config.Exists("server.port")

err := config.Add(ctx, "features.search", true) // 路径已存在时失败。
err = config.Set(ctx, "server.port", 9090)      // 新增或覆盖配置。
err = config.Delete(ctx, "features.legacy")    // 删除配置。
```

## 监听变化

`Watch` 会阻塞运行，直到 context 被取消或 Provider 的监听结束。

```go
go func() {
	err := config.Watch(ctx, sundial.WatchOptions{
		OnChange: func() { log.Println("配置已更新") },
		OnError:  func(err error) { log.Printf("监听错误: %v", err) },
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("监听已停止: %v", err)
	}
}()
```

新内存快照生效后会调用 `OnChange`；重新加载失败或 Provider 监听出错时会调用 `OnError`。重新加载失败不会污染当前配置，轮询会继续运行。

Provider 可以通过实现 `Watcher` 提供原生变化通知；否则 Sundial 会按照 `WatchInterval` 轮询配置源。

## 配置格式

JSON 是默认格式，不需要额外配置。当配置源存储 YAML 时，使用项目提供的 YAML Codec：

```go
import yamlcodec "github.com/sundayfun/sundial/codec/yaml"

config, err := sundial.New(ctx, sundial.Options{
	Provider: source,
	Codec:    yamlcodec.New(),
})
```

其他格式可以自行实现 `codec.Codec` 接口。

## 实现 Provider

Provider 负责加载和保存一份完整配置文档：

```go
type Provider interface {
	Load(ctx context.Context) ([]byte, error)
	Save(ctx context.Context, data []byte) error
}
```

需要原生监听能力时，还可以实现：

```go
type Watcher interface {
	Watch(ctx context.Context, notify func() error) error
}
```

具体存储实现位于 `provider/<source>`，核心包不依赖任何存储 SDK。

## 参考资料

- [koanf](https://github.com/knadh/koanf)
- [Viper](https://github.com/spf13/viper)

## 行为约定

- 配置路径统一使用点号，例如 `server.port`。
- 写入失败时，当前内存快照保持不变。
- 重新加载失败时，保留上一份有效配置。
- 同一实例的写操作串行执行，读操作支持并发使用。
- 多实例并发写入采用 last-write-wins 语义。

## 许可证

[MIT](LICENSE)
