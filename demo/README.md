# beat-http 示例

## 示例

### webhook/
演示 HTTP Webhook 模式的消息发布与接收。

```bash
cd webhook && go run .
```

## 测试

```bash
# 基准测试
go test -bench=. -benchmem

# 压力测试
go test -v -run TestStress
```

## 前置要求

- Go 1.21+
- 无外部依赖（纯 net/http）

## 说明

beat-http 使用 HTTP POST 发送消息：
- **Publisher**: 发送 `POST {EndpointURL}/{topic}` 请求
- **Subscriber**: 启动 HTTP 服务器监听消息
- Metadata 通过 `Beat-Meta-*` 头传递
