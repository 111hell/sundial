# Sundial

[![Go Reference](https://pkg.go.dev/badge/github.com/sundayfun/sundial.svg)](https://pkg.go.dev/github.com/sundayfun/sundial)

[English](README.md)

Sundial 是一个轻量、可扩展、类型安全的 Go 配置 SDK，提供内存读取、持久化写入
和实时更新能力。

## 为什么选择 Sundial

- **类型安全访问**：应用直接读取自己定义的配置结构体，不再使用字符串路径和 `any`。
- **快速读取**：`Get` 返回内存中已解析的快照，无需再次解码或深拷贝。
- **持久化写入**：`Put` 有条件地保存完整的强类型配置文档。
- **版本历史**：支持查看历史版本和恢复配置。
- **实时更新**：自动重新加载将外部变化同步到内存。
- **存储和格式可扩展**：配置源实现 `Provider`；默认使用 JSON，其他格式通过 Codec 扩展。

每个 `Client` 管理一份完整配置文档。

## 使用效果

![Sundial 内存读取、并发写入保护与历史恢复示意图](docs/images/sundial-overview.png)

例如，两位调用方都读取了版本 A：第一位保存后生成 B，第二位仍基于 A 写入时会收到
`ErrConflict`。之后从 C 恢复 A 的配置，会生成内容与 A 相同的新版本 D，保留完整历史。

## 安装

```sh
go get github.com/sundayfun/sundial
```

## 快速开始

以下示例读取并更新 S3 中已有的 JSON 配置。
凭据配置和首次发布见 [S3 示例](examples/s3)。

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
    })
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

`Get` 返回共享的只读配置快照及其版本，无 error 返回，也不进行编码、解码或
深拷贝。不要修改返回值，包括其中的 map、slice 和指针。Go 不强制只读，需要
调用方遵守这个约定。`Update`、`Put`、`RestoreRevision` 成功后返回的结果，以及 `OnChange` 回调
收到的配置，同样是共享只读快照。

普通修改直接调用 `Update(ctx, func(*T) error)`，不需要先调用 `Get`。它基于
缓存的原始字节和快照版本创建独立草稿，执行修改回调，再通过版本条件发布。
解码、回调或发布失败时都保留原快照；返回结果是共享只读配置。调用期间不要并发修改回调草稿。
回调内不要对同一 Client 调用 `Update`、`Put`、`Reload` 或 `RestoreRevision`，
因为这些操作会获取同一个写锁。

需要手动发布或显式处理版本时，调用 `Draft() (Entry[T], error)`：从缓存的原始字节解码出独立可修改的
草稿，并保留快照对应的版本。这样读取路径无需解码，草稿修改在发布成功前
不会影响缓存。通过 `Put` 发布期间，不要并发修改草稿。

`New` 无需 clone 函数，也不通过反射限制配置类型；支持哪些类型由所选 Codec
决定。Codec 每次 `Decode` 必须创建独立的可变对象，不能保留或复用这些对象的
引用；`Encode` 不得修改输入。

`Put` 保存完整文档，版本过期时返回 `ErrConflict`，不会自动合并或重试。
取消 context 会停止自动加载。写入或加载失败时，保留内存中上一份有效配置。

## 文档

- [S3 示例](examples/s3)：环境配置、首次发布和条件写入。
- [API 文档](https://pkg.go.dev/github.com/sundayfun/sundial)：版本历史、Codec 和加载回调。

## 许可证

[MIT](LICENSE)
