package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"sync/atomic"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const inputQueue = "product.find.requested.v1"
const outputQueue = "product.information.found.v1"

// Run reconnects after connection failure and leaves unconfirmed deliveries
// unacknowledged. Invalid messages are rejected to the configured dead letter.
func Run(ctx context.Context, brokerURL string, logger *slog.Logger, ready *atomic.Bool) {
	for ctx.Err() == nil {
		if err := consume(ctx, brokerURL, ready); err != nil && ctx.Err() == nil {
			logger.Warn("finder consumer disconnected", "error", err)
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
func consume(ctx context.Context, brokerURL string, ready *atomic.Bool) error {
	parsed, err := url.Parse(brokerURL)
	if err != nil || parsed.User == nil || parsed.User.Username() == "" {
		return errors.New("broker publisher identity required")
	}
	identity := parsed.User.Username()
	connection, err := amqp.Dial(brokerURL)
	if err != nil {
		return err
	}
	defer func() { _ = connection.Close() }()
	channel, err := connection.Channel()
	if err != nil {
		return err
	}
	defer func() { _ = channel.Close() }()
	if err = channel.ExchangeDeclare("price.dead-letter", "direct", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err = channel.QueueDeclare(inputQueue+".dead", true, false, false, false, nil); err != nil {
		return err
	}
	if err = channel.QueueBind(inputQueue+".dead", inputQueue+".dead", "price.dead-letter", false, nil); err != nil {
		return err
	}
	if _, err = channel.QueueDeclare(inputQueue, true, false, false, false, amqp.Table{"x-dead-letter-exchange": "price.dead-letter", "x-dead-letter-routing-key": inputQueue + ".dead"}); err != nil {
		return err
	}
	if err = channel.Qos(1, 0, false); err != nil {
		return err
	}
	if err = channel.Confirm(false); err != nil {
		return err
	}
	confirms := channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	returns := channel.NotifyReturn(make(chan amqp.Return, 1))
	deliveries, err := channel.Consume(inputQueue, "data-finder", false, false, false, false, nil)
	if err != nil {
		return err
	}
	ready.Store(true)
	defer ready.Store(false)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("finder queue closed")
			}
			result, err := Process(delivery.Body)
			if err != nil {
				if err = delivery.Reject(false); err != nil {
					return err
				}
				continue
			}
			body, err := json.Marshal(result)
			if err != nil {
				return err
			}
			bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
			err = channel.PublishWithContext(bounded, "", outputQueue, true, false, amqp.Publishing{ContentType: "application/json", UserId: identity, DeliveryMode: amqp.Persistent, MessageId: result.MessageID, CorrelationId: result.CorrelationID, Type: result.MessageType, Body: body})
			if err != nil {
				cancel()
				return err
			}
			select {
			case confirmation, ok := <-confirms:
				if !ok || !confirmation.Ack {
					cancel()
					return errors.New("finder result not confirmed")
				}
			case <-bounded.Done():
				cancel()
				return bounded.Err()
			}
			cancel()
			select {
			case <-returns:
				return errors.New("finder result unroutable")
			default:
			}
			if err = delivery.Ack(false); err != nil {
				return err
			}
		}
	}
}
