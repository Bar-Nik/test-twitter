package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	MessageQueue = "message"
)

// RabbitMQClient обертка для работы с RabbitMQ
type RabbitMQClient struct {
	Conn *amqp.Connection
	Ch   *amqp.Channel
}

// NewRabbitMQClient создает нового клиента RabbitMQ

func NewRabbitMQClient(host string, port string, username string, password string, vHost string) (*RabbitMQClient, error) {

	url := fmt.Sprintf("amqp://%s:%s@%s:%s/%s",
		username,
		password,
		host,
		port,
		vHost,
	)

	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}

	// Объявляем очередь для сообщений
	_, err = ch.QueueDeclare(
		MessageQueue,
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, err
	}

	return &RabbitMQClient{Conn: conn, Ch: ch}, nil
}

// Close закрывает соединение с RabbitMQ
func (c *RabbitMQClient) Close() {
	if c.Ch != nil {
		c.Ch.Close()
	}
	if c.Conn != nil {
		c.Conn.Close()
	}
}
