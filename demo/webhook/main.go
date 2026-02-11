package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	beathttp "github.com/uniyakcom/beat-http"
	"github.com/uniyakcom/beat/message"
)

type OrderEvent struct {
	OrderID    string  `json:"order_id"`
	CustomerID string  `json:"customer_id"`
	Amount     float64 `json:"amount"`
	Status     string  `json:"status"`
	Timestamp  int64   `json:"timestamp"`
}

func main() {
	// 创建 HTTP Subscriber（监听 :8088）
	sub, err := beathttp.NewSubscriber(beathttp.SubscriberConfig{
		ListenAddr: ":8088",
	})
	if err != nil {
		log.Fatalf("创建 Subscriber 失败: %v", err)
	}
	defer sub.Close()
	fmt.Println("✓ HTTP Subscriber 启动于 :8088")

	// 创建 HTTP Publisher（发送到 Subscriber 地址）
	pub, err := beathttp.NewPublisher(beathttp.PublisherConfig{
		EndpointURL: "http://localhost:8088",
	})
	if err != nil {
		log.Fatalf("创建 Publisher 失败: %v", err)
	}
	defer pub.Close()
	fmt.Println("✓ HTTP Publisher 准备就绪")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 订阅 webhook topic
	msgCh, err := sub.Subscribe(ctx, "webhook.orders")
	if err != nil {
		log.Fatalf("订阅失败: %v", err)
	}

	received := 0
	go func() {
		for msg := range msgCh {
			var event OrderEvent
			if err := json.Unmarshal(msg.Payload, &event); err != nil {
				continue
			}
			received++
			fmt.Printf("📩 [Webhook] 收到: %s ¥%.2f (UUID: %s)\n", event.OrderID, event.Amount, msg.UUID[:8])
		}
	}()

	time.Sleep(300 * time.Millisecond) // 等待服务就绪

	fmt.Println("\n📤 通过 HTTP 发布消息...")
	events := []OrderEvent{
		{OrderID: "ORD-001", CustomerID: "CUST-001", Amount: 128.00, Status: "created", Timestamp: time.Now().Unix()},
		{OrderID: "ORD-002", CustomerID: "CUST-002", Amount: 256.50, Status: "created", Timestamp: time.Now().Unix()},
		{OrderID: "ORD-003", CustomerID: "CUST-003", Amount: 512.99, Status: "created", Timestamp: time.Now().Unix()},
	}

	for _, event := range events {
		payload, _ := json.Marshal(event)
		msg := message.New("", payload)
		msg.Metadata.Set("source", "demo")
		msg.Metadata.Set("env", "development")

		if err := pub.Publish(context.Background(), "webhook.orders", msg); err != nil {
			log.Printf("发布失败: %v", err)
		} else {
			fmt.Printf("  ✓ POST /webhook.orders → %s (¥%.2f)\n", event.OrderID, event.Amount)
		}
		time.Sleep(200 * time.Millisecond)
	}

	time.Sleep(2 * time.Second)
	fmt.Printf("\n✓ 示例完成，共收到 %d 条消息\n", received)
}
