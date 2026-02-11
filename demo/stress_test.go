package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	beathttp "github.com/uniyakcom/beat-http"
	"github.com/uniyakcom/beat/message"
)

// TestStressConcurrentPublish 压力测试：并发 HTTP 发布
func TestStressConcurrentPublish(t *testing.T) {
	sub, err := beathttp.NewSubscriber(beathttp.SubscriberConfig{ListenAddr: ":18091"})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	pub, err := beathttp.NewPublisher(beathttp.PublisherConfig{EndpointURL: "http://localhost:18091"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgCh, _ := sub.Subscribe(ctx, "stress.concurrent")
	go func() {
		for range msgCh {
		}
	}()

	time.Sleep(200 * time.Millisecond)

	const (
		numGoroutines = 10
		messagesPerGo = 50
	)

	payload, _ := json.Marshal(OrderEvent{
		OrderID: "ORD-001", CustomerID: "CUST-001",
		Amount: 99.99, Status: "created", Timestamp: time.Now().Unix(),
	})

	var wg sync.WaitGroup
	var success, errors int64
	start := time.Now()

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < messagesPerGo; i++ {
				if err := pub.Publish(context.Background(), "stress.concurrent", message.New("", payload)); err != nil {
					atomic.AddInt64(&errors, 1)
				} else {
					atomic.AddInt64(&success, 1)
				}
			}
		}()
	}

	wg.Wait()
	dur := time.Since(start)
	total := numGoroutines * messagesPerGo

	t.Logf("goroutines=%d total=%d success=%d errors=%d elapsed=%v throughput=%.0f msg/s",
		numGoroutines, total, success, errors, dur, float64(total)/dur.Seconds())
}

// TestStressEndToEnd 压力测试：端到端消息传递
func TestStressEndToEnd(t *testing.T) {
	sub, err := beathttp.NewSubscriber(beathttp.SubscriberConfig{ListenAddr: ":18092"})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	pub, err := beathttp.NewPublisher(beathttp.PublisherConfig{EndpointURL: "http://localhost:18092"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	msgCh, _ := sub.Subscribe(ctx, "stress.e2e")

	var received int64
	const count = 200

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case _, ok := <-msgCh:
				if !ok {
					return
				}
				atomic.AddInt64(&received, 1)
			case <-ctx.Done():
				return
			}
		}
	}()

	time.Sleep(200 * time.Millisecond)

	payload, _ := json.Marshal(OrderEvent{
		OrderID: "ORD-001", CustomerID: "CUST-001",
		Amount: 99.99, Status: "created", Timestamp: time.Now().Unix(),
	})

	start := time.Now()
	for i := 0; i < count; i++ {
		pub.Publish(context.Background(), "stress.e2e", message.New("", payload))
	}

	deadline := time.After(10 * time.Second)
	for atomic.LoadInt64(&received) < int64(count) {
		select {
		case <-deadline:
			goto report
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

report:
	dur := time.Since(start)
	recv := atomic.LoadInt64(&received)
	cancel()
	<-done

	t.Logf("sent=%d received=%d elapsed=%v throughput=%.0f msg/s",
		count, recv, dur, float64(recv)/dur.Seconds())
}

// TestStressMultiTopic 压力测试：多 topic HTTP 转发
func TestStressMultiTopic(t *testing.T) {
	sub, err := beathttp.NewSubscriber(beathttp.SubscriberConfig{ListenAddr: ":18093"})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	pub, err := beathttp.NewPublisher(beathttp.PublisherConfig{EndpointURL: "http://localhost:18093"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	topics := []string{"stress.t1", "stress.t2", "stress.t3"}
	for _, topic := range topics {
		ch, _ := sub.Subscribe(ctx, topic)
		go func() {
			for range ch {
			}
		}()
	}

	time.Sleep(200 * time.Millisecond)

	const messagesPerTopic = 50

	payload, _ := json.Marshal(OrderEvent{
		OrderID: "ORD-001", CustomerID: "CUST-001",
		Amount: 99.99, Status: "created", Timestamp: time.Now().Unix(),
	})

	var wg sync.WaitGroup
	var errors int64
	start := time.Now()

	for _, topic := range topics {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			for i := 0; i < messagesPerTopic; i++ {
				if err := pub.Publish(context.Background(), t, message.New("", payload)); err != nil {
					atomic.AddInt64(&errors, 1)
				}
			}
		}(topic)
	}

	wg.Wait()
	dur := time.Since(start)
	total := len(topics) * messagesPerTopic

	t.Logf("topics=%d total=%d errors=%d elapsed=%v throughput=%.0f msg/s",
		len(topics), total, errors, dur, float64(total)/dur.Seconds())
}

// TestStressPayloadSizes 压力测试：不同消息大小
func TestStressPayloadSizes(t *testing.T) {
	sub, err := beathttp.NewSubscriber(beathttp.SubscriberConfig{ListenAddr: ":18094"})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	pub, err := beathttp.NewPublisher(beathttp.PublisherConfig{EndpointURL: "http://localhost:18094"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgCh, _ := sub.Subscribe(ctx, "stress.payload")
	go func() {
		for range msgCh {
		}
	}()

	time.Sleep(200 * time.Millisecond)

	sizes := []struct {
		name string
		size int
	}{
		{"64B", 64},
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
	}

	const count = 30
	for _, s := range sizes {
		data := make([]byte, s.size)
		for i := range data {
			data[i] = byte(i % 256)
		}
		start := time.Now()
		var errs int
		for i := 0; i < count; i++ {
			if err := pub.Publish(context.Background(), "stress.payload", message.New("", data)); err != nil {
				errs++
			}
		}
		dur := time.Since(start)
		fmt.Printf("  %-6s: %d msgs in %v (%.0f msg/s, errors=%d)\n",
			s.name, count, dur, float64(count)/dur.Seconds(), errs)
	}
}
