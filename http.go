// Package http 提供基于 HTTP 的 beat Publisher/Subscriber 实现。
//
// 零外部依赖（纯 net/http），适用于 Webhook、跨服务 HTTP 事件推送等场景。
//
// Publisher 通过 HTTP POST 发送消息到远端 URL。
// Subscriber 启动 HTTP 服务端接收来自 Publisher 的消息。
package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/uniyakcom/beat/marshal"
	"github.com/uniyakcom/beat/message"
)

// PublisherConfig 发布者配置
type PublisherConfig struct {
	// EndpointURL 远端接收 URL（必填）。
	// 消息将通过 POST 发送到 {EndpointURL}/{topic}。
	EndpointURL string

	// Marshaler 消息序列化器。为 nil 时使用 JSON。
	Marshaler marshal.Codec

	// Client 自定义 HTTP 客户端。为 nil 时使用默认客户端（10s 超时）。
	Client *http.Client
}

func (c *PublisherConfig) defaults() {
	if c.Marshaler == nil {
		c.Marshaler = marshal.JSON{}
	}
	if c.Client == nil {
		c.Client = &http.Client{Timeout: 10 * time.Second}
	}
}

// Publisher HTTP 发布者
type Publisher struct {
	endpointURL string
	marshaler   marshal.Codec
	client      *http.Client
}

// NewPublisher 创建 HTTP 发布者。
func NewPublisher(cfg PublisherConfig) (*Publisher, error) {
	if cfg.EndpointURL == "" {
		return nil, fmt.Errorf("beat-http: EndpointURL is required")
	}
	cfg.defaults()
	return &Publisher{
		endpointURL: cfg.EndpointURL,
		marshaler:   cfg.Marshaler,
		client:      cfg.Client,
	}, nil
}

// Publish 通过 HTTP POST 发布消息。
//
// 请求格式：
//
//	POST {EndpointURL}/{topic}
//	Content-Type: application/octet-stream
//	Beat-UUID: <uuid>
//	Beat-Meta-<key>: <value>
//	Body: marshaled message
func (p *Publisher) Publish(ctx context.Context, topic string, messages ...*message.Message) error {
	for _, msg := range messages {
		data, err := p.marshaler.Marshal(topic, msg)
		if err != nil {
			return err
		}

		url := p.endpointURL + "/" + topic
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return err
		}

		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Beat-UUID", msg.UUID)
		for k, v := range msg.Metadata {
			req.Header.Set("Beat-Meta-"+k, v)
		}

		resp, err := p.client.Do(req)
		if err != nil {
			return fmt.Errorf("beat-http: publish to %s failed: %w", url, err)
		}
		resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("beat-http: publish to %s returned %d", url, resp.StatusCode)
		}
	}
	return nil
}

// Close 关闭 HTTP 发布者（无操作）。
func (p *Publisher) Close() error {
	return nil
}

// SubscriberConfig 订阅者配置
type SubscriberConfig struct {
	// ListenAddr HTTP 监听地址（例如 ":8080"）。
	// 为空时不自动启动服务器，需外部调用 Handler() 集成到已有服务器。
	ListenAddr string

	// Marshaler 消息反序列化器。为 nil 时使用 JSON。
	Marshaler marshal.Codec
}

func (c *SubscriberConfig) defaults() {
	if c.Marshaler == nil {
		c.Marshaler = marshal.JSON{}
	}
}

// Subscriber HTTP 订阅者
type Subscriber struct {
	addr      string
	marshaler marshal.Codec
	server    *http.Server
	topics    map[string]chan *message.Message
	done      chan struct{}
	once      sync.Once
	mu        sync.RWMutex
}

// NewSubscriber 创建 HTTP 订阅者。
func NewSubscriber(cfg SubscriberConfig) (*Subscriber, error) {
	cfg.defaults()
	s := &Subscriber{
		addr:      cfg.ListenAddr,
		marshaler: cfg.Marshaler,
		topics:    make(map[string]chan *message.Message),
		done:      make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleMessage)

	if s.addr != "" {
		s.server = &http.Server{
			Addr:    s.addr,
			Handler: mux,
		}
		go func() {
			if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
				// 仅在非正常关闭时记录
				_ = err
			}
		}()
	}

	return s, nil
}

// Handler 返回 HTTP handler，用于集成到已有服务器。
//
//	sub, _ := http.NewSubscriber(http.SubscriberConfig{}) // 不设置 ListenAddr
//	mux.Handle("/beat/", http.StripPrefix("/beat", sub.Handler()))
func (s *Subscriber) Handler() http.Handler {
	return http.HandlerFunc(s.handleMessage)
}

// Subscribe 订阅 topic，返回消息通道。
// topic 对应 HTTP 路径（POST /{topic}）。
func (s *Subscriber) Subscribe(ctx context.Context, topic string) (<-chan *message.Message, error) {
	s.mu.Lock()
	ch := make(chan *message.Message, 256)
	s.topics[topic] = ch
	s.mu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
		case <-s.done:
		}
		s.mu.Lock()
		delete(s.topics, topic)
		close(ch)
		s.mu.Unlock()
	}()

	return ch, nil
}

// Close 关闭 HTTP 订阅者。
func (s *Subscriber) Close() error {
	s.once.Do(func() { close(s.done) })
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}
	return nil
}

// handleMessage 处理收到的 HTTP 消息
func (s *Subscriber) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// 从路径提取 topic: /order.created → order.created
	topic := r.URL.Path
	if len(topic) > 0 && topic[0] == '/' {
		topic = topic[1:]
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Read Body Failed", http.StatusBadRequest)
		return
	}

	msg, err := s.marshaler.Unmarshal(topic, body)
	if err != nil {
		http.Error(w, "Unmarshal Failed", http.StatusBadRequest)
		return
	}

	// 恢复 HTTP headers 到 metadata
	for key, vals := range r.Header {
		if len(key) > 10 && key[:10] == "Beat-Meta-" {
			if len(vals) > 0 {
				msg.Metadata.Set(key[10:], vals[0])
			}
		}
	}

	s.mu.RLock()
	ch, ok := s.topics[topic]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "No Subscriber", http.StatusNotFound)
		return
	}

	select {
	case ch <- msg:
		w.WriteHeader(http.StatusAccepted)
	default:
		http.Error(w, "Subscriber Overloaded", http.StatusServiceUnavailable)
	}
}
