package messaging

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestWorkerBrokerDelivery(t *testing.T) {
	broker := os.Getenv("FINDER_TEST_RABBITMQ_URL")
	if broker == "" {
		t.Skip("set FINDER_TEST_RABBITMQ_URL to a disposable broker")
	}
	var conn *amqp.Connection
	var err error
	for i := 0; i < 100; i++ {
		conn, err = amqp.Dial(broker)
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ch.Close() }()
	if _, err = ch.QueueDeclare(outputQueue, true, false, false, false, nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var ready atomic.Bool
	go func() { defer close(done); Run(ctx, broker, slog.New(slog.NewTextHandler(io.Discard, nil)), &ready) }()
	defer func() { cancel(); <-done }()
	var input amqp.Queue
	for i := 0; i < 50; i++ {
		input, err = ch.QueueDeclarePassive(inputQueue, true, false, false, false, amqp.Table{"x-dead-letter-exchange": "price.dead-letter", "x-dead-letter-routing-key": inputQueue + ".dead"})
		if err == nil {
			break
		}
		_ = ch.Close()
		ch, err = conn.Channel()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil || input.Name == "" {
		t.Fatal("worker queue unavailable")
	}
	id := "00000000-0000-4000-8000-000000000001"
	price := "R$ 12,34"
	cash := "à vista"
	payload, _ := json.Marshal(request{CrawlRequestID: id, PageParsedMessageID: id, Page: Page{CrawlRequestID: id, RequestedURL: "https://store.example/a", FinalURL: "https://store.example/a", Outcome: "partial", ObservedAt: time.Now(), ParserVersion: "1", Fields: []PageField{{Name: "price", RawValue: &price}, {Name: "payment_method", RawValue: &cash}}}})
	msg, _ := json.Marshal(Envelope{MessageID: id, MessageType: "product.find.requested", SchemaVersion: 1, OccurredAt: time.Now(), OrganizationID: id, CorrelationID: id, Payload: payload})
	if err = ch.PublishWithContext(context.Background(), "", inputQueue, false, false, amqp.Publishing{Body: msg}); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
resultLoop:
	for {
		select {
		case <-deadline:
			t.Fatal("result not published")
		default:
			delivery, ok, err := ch.Get(outputQueue, true)
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				var result Envelope
				if err = json.Unmarshal(delivery.Body, &result); err != nil || result.CausationID != id || result.OrganizationID != id {
					t.Fatalf("bad output %+v %v", result, err)
				}
				break resultLoop
			}
			time.Sleep(25 * time.Millisecond)
		}
	}

	invalid, _ := json.Marshal(Envelope{MessageID: id, MessageType: "product.find.requested", SchemaVersion: 2, OccurredAt: time.Now(), OrganizationID: id, CorrelationID: id, Payload: payload})
	if err = ch.PublishWithContext(context.Background(), "", inputQueue, false, false, amqp.Publishing{Body: invalid}); err != nil {
		t.Fatal(err)
	}
	deadline = time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("invalid message not dead-lettered")
		default:
			_, ok, err := ch.Get(inputQueue+".dead", true)
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
}
