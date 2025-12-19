package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Producer struct {
	channel *amqp.Channel
}

func NewProducer(channel *amqp.Channel) *Producer {
	return &Producer{channel: channel}
}

// PublishJSON публикует сообщение в формате JSON
func (p *Producer) PublishJSON(ctx context.Context, routingKey string, message interface{}) error {
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// // Объявляем exchange если не существует
	// err = p.channel.ExchangeDeclare(
	// 	exchange, // name
	// 	"direct", // type
	// 	true,     // durable
	// 	false,    // auto-deleted
	// 	false,    // internal
	// 	false,    // no-wait
	// 	nil,      // arguments
	// )
	// if err != nil {
	// 	return fmt.Errorf("failed to declare exchange: %w", err)
	// }

	// Публикуем сообщение
	err = p.channel.PublishWithContext(ctx,
		"",         // exchange
		routingKey, // routing key
		false,      // mandatory
		false,      // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent, // Сохранять при перезапуске
			Timestamp:    time.Now(),
		},
	)

	return err
}
