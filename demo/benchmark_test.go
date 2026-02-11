package demo

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	beathttp "github.com/uniyakcom/beat-http"
	"github.com/uniyakcom/beat/message"
)

func setupHTTP(tb testing.TB, port string) (*beathttp.Publisher, *beathttp.Subscriber) {
	sub, err := beathttp.NewSubscriber(beathttp.SubscriberConfig{
		ListenAddr: ":" + port,
	})
	if err != nil {
		tb.Fatal(err)
	}

	pub, err := beathttp.NewPublisher(beathttp.PublisherConfig{
		EndpointURL: "http://localhost:" + port,
	})
	if err != nil {
		sub.Close()
		tb.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond) // 等待 server 启动
	return pub, sub
}

// BenchmarkHTTPPublish HTTP 发布吞吐量
func BenchmarkHTTPPublish(b *testing.B) {
	pub, sub := setupHTTP(b, "18081")
	defer sub.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgCh, _ := sub.Subscribe(ctx, "bench.http")

	// 消费消息避免阻塞
	go func() {
		for range msgCh {
		}
	}()

	payload, _ := json.Marshal(OrderEvent{
		OrderID: "ORD-001", CustomerID: "CUST-001",
		Amount: 99.99, Status: "created", Timestamp: time.Now().Unix(),
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pub.Publish(context.Background(), "bench.http", message.New("", payload))
	}
}

// BenchmarkHTTPConcurrentPublish 并发 HTTP 发布
func BenchmarkHTTPConcurrentPublish(b *testing.B) {
	pub, sub := setupHTTP(b, "18082")
	defer sub.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgCh, _ := sub.Subscribe(ctx, "bench.http.concurrent")
	go func() {
		for range msgCh {
		}
	}()

	payload, _ := json.Marshal(OrderEvent{
		OrderID: "ORD-001", CustomerID: "CUST-001",
		Amount: 99.99, Status: "created", Timestamp: time.Now().Unix(),
	})

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pub.Publish(context.Background(), "bench.http.concurrent", message.New("", payload))
		}
	})
}
