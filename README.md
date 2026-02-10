# beat-http

[![Go Reference](https://pkg.go.dev/badge/github.com/uniyakcom/beat-http.svg)](https://pkg.go.dev/github.com/uniyakcom/beat-http)

[beat](https://github.com/uniyakcom/beat) 消息框架的 **HTTP** 适配器。

零外部依赖（纯 `net/http`），适用于 **Webhook 回调**、**跨服务 HTTP 事件推送**、**Serverless 函数触发** 等场景。

## 安装

```bash
go get github.com/uniyakcom/beat-http
```

## 依赖

无外部依赖。仅使用 Go 标准库 `net/http`。

## 快速开始

### 独立服务模式

```go
package main

import (
    "context"
    "fmt"
    beathttp "github.com/uniyakcom/beat-http"
    "github.com/uniyakcom/beat/message"
)

func main() {
    // 订阅端：启动 HTTP 服务器
    sub, _ := beathttp.NewSubscriber(beathttp.SubscriberConfig{
        ListenAddr: ":8080",
    })
    defer sub.Close()

    ctx := context.Background()
    msgCh, _ := sub.Subscribe(ctx, "order.created")

    go func() {
        for msg := range msgCh {
            fmt.Printf("收到: %s\n", string(msg.Payload))
            msg.Ack()
        }
    }()

    // 发布端：通过 HTTP POST 推送
    pub, _ := beathttp.NewPublisher(beathttp.PublisherConfig{
        EndpointURL: "http://localhost:8080",
    })
    pub.Publish("order.created", message.NewMessage("", []byte(`{"id":1}`)))
}
```

### 集成到已有 HTTP 服务器

```go
sub, _ := beathttp.NewSubscriber(beathttp.SubscriberConfig{})  // 不设置 ListenAddr

mux := http.NewServeMux()
mux.Handle("/events/", http.StripPrefix("/events", sub.Handler()))
mux.HandleFunc("/health", healthHandler)

http.ListenAndServe(":8080", mux)
```

### Webhook 接收

```go
// 接收第三方 Webhook（如支付回调）
sub, _ := beathttp.NewSubscriber(beathttp.SubscriberConfig{
    ListenAddr: ":9090",
})

msgCh, _ := sub.Subscribe(ctx, "payment.callback")

for msg := range msgCh {
    handlePaymentCallback(msg.Payload)
    msg.Ack()
}
```

### 与 Router 配合

```go
r := router.NewRouter()

sub, _ := beathttp.NewSubscriber(beathttp.SubscriberConfig{
    ListenAddr: ":8080",
})

r.On("webhook", "payment.callback", sub, func(msg *message.Message) error {
    return handlePaymentCallback(msg.Payload)
})

r.Run(ctx)
```

## 协议格式

### 请求

```
POST /{topic} HTTP/1.1
Content-Type: application/octet-stream
Beat-UUID: <message-uuid>
Beat-Meta-<key>: <value>

<marshaled message body>
```

### 响应

| 状态码 | 含义 |
|--------|------|
| 202 | 消息已接受 |
| 400 | 请求体解析失败 |
| 404 | 无该 topic 的订阅者 |
| 405 | 非 POST 方法 |
| 503 | 订阅者繁忙（通道满） |

## 特性

- **零外部依赖**: 纯 `net/http` 标准库实现
- **独立/集成双模式**: 自动启动 HTTP 服务器，或集成到已有服务器
- **Header 传播**: message.Metadata ↔ HTTP Header 双向映射
- **Webhook 兼容**: 可用作 Webhook 接收端
- **自定义序列化**: 默认 JSON，可替换
- **优雅关闭**: `Close()` 自动 `server.Shutdown()`

## 许可证

MIT License
