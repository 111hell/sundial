# Sundial

[English](README.md) | [简体中文](README.zh-CN.md)

Sundial 是一个轻量、可扩展、类型安全的 Go 配置 SDK，提供内存读取和持久化写入能力。

## 为什么选择 Sundial

- **类型安全访问**：应用直接读取自己定义的配置结构体，不再使用字符串路径和 `any`。
- **快速读取**：`Get` 只读取内存快照。
- **持久化写入**：`Put` 有条件地保存完整的强类型配置文档。
- **实时更新**：`Watch` 将外部变化同步到内存。
- **存储和格式可扩展**：配置源实现 `Provider`；默认使用 JSON，其他格式通过 Codec 扩展。

一个 Sundial 实例管理一份完整配置文档。

## 安装

```sh
go get github.com/sundayfun/sundial
```

## 快速开始

首先定义应用拥有的配置结构：

```go
type Config struct {
	Server struct {
		Host string `json:"host" yaml:"host"`
		Port int    `json:"port" yaml:"port"`
	} `json:"server" yaml:"server"`
	Debug bool `json:"debug" yaml:"debug"`
}
```

使用已初始化的 `Provider` 创建 Sundial。`New` 会加载并验证初始配置：

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

### S3 Provider

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

### 读取

`Get` 从内存返回 `Entry`，不会访问 Provider。`Value` 是独立副本，
`Metadata.Revision` 来自同一快照：

```go
entry, err := configStore.Get()
if err != nil {
	log.Fatal(err)
}

fmt.Println(entry.Value.Server.Port)
```

### 写入

修改 `entry.Value`，然后将 `Entry` 传回 `Put` 进行条件写入：

```go
entry, err := configStore.Get()
if err != nil {
	log.Fatal(err)
}

entry.Value.Server.Port = 9090
if err := configStore.Put(ctx, entry); err != nil {
	if errors.Is(err, sundial.ErrConflict) {
		// 重新加载、读取新 revision、应用本次修改，然后按需重试。
		log.Print("保存前配置已发生变化")
		return
	}
	log.Fatal(err)
}
```

`Put` 使用 `entry.Metadata.Revision`；如果其他写入先成功，则返回
`ErrConflict`。它不会自动合并或重试。

## 监听变化

`Watch` 会阻塞运行，直到 context 被取消或 Provider 的监听结束：

```go
go func() {
	err := configStore.Watch(
		ctx,
		sundial.WithOnChange(func() {
			entry, err := configStore.Get()
			if err != nil {
				log.Printf("读取配置失败: %v", err)
				return
			}
			log.Printf("配置已更新: port=%d", entry.Value.Server.Port)
		}),
		sundial.WithOnError(func(err error) {
			log.Printf("监听错误: %v", err)
		}),
	)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("监听已停止: %v", err)
	}
}()
```

Provider 可以通过实现 `Watcher` 提供原生变化通知；否则 Sundial 默认每 30 秒轮询一次 Provider，可通过 `WithWatchInterval` 修改间隔。

S3 Provider 通过轮询对象元数据实现 `Watcher`：按照
`Config.WatchInterval` 调用 `HeadObject`；完成启动同步后，仅在 ETag
发生变化时下载对象。该间隔默认为 30 秒。

外部内容必须成功解码为应用的配置类型后才会发布。重新加载失败时保留上一份有效快照，并通过 `WithOnError` 报告错误。

## 配置格式

JSON 是默认格式。当配置源存储 YAML 时，使用项目提供的 YAML Codec：

```go
import yamlcodec "github.com/sundayfun/sundial/codec/yaml"

configStore, err := sundial.New[Config](
	ctx,
	provider,
	sundial.WithCodec(yamlcodec.New()),
)
```

其他格式可以自行实现 `codec.Codec`。

## 实现 Provider

Provider 负责加载并有条件地保存一份完整配置：

```go
type Provider interface {
	Load(ctx context.Context) ([]byte, Metadata, error)
	Save(ctx context.Context, data []byte, expectedMetadata Metadata) (Metadata, error)
}
```

`Load` 返回的数据和 `Metadata` 必须对应同一配置状态。只有
`expectedMetadata.Revision` 与当前配置的 Revision 匹配时，`Save` 才能替换配置；
否则返回 `ErrConflict`。Provider 必须原子地完成这项检查。

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

- 配置文档不存在时，从应用配置类型的零值开始。
- `Put` 失败或发生冲突时，当前内存快照保持不变。
- 重新加载失败时，保留上一份有效配置。
- `Get` 支持并发调用。同一实例的 `Put` 会串行执行，陈旧 `Revision` 会返回 `ErrConflict`。

## 开发

仓库固定了 golangci-lint 版本。使用 [Just](https://just.systems/) 运行本地质量门禁：

```sh
just lint # 只检查，不修改文件。
just fmt  # 使用已配置的 formatter 格式化代码。
just test # 运行 lint 和 race test。
```

## 许可证

[MIT](LICENSE)
