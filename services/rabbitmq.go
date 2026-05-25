package services

import (
	"encoding/json"
	"log"
	"os"

	"github.com/streadway/amqp"
)

var (
	RabbitConn *amqp.Connection
	RabbitCh   *amqp.Channel
)

const ExchangeName = "workflow_events"
const MessageExpiration = "3600000"

func InitRabbitMQ() {
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		url = "amqp://guest:guest@localhost:5672/"
	}

	conn, err := amqp.Dial(url)
	if err != nil {
		log.Printf("Warning: Failed to connect to RabbitMQ: %v", err)
		return
	}

	ch, err := conn.Channel()
	if err != nil {
		log.Printf("Warning: Failed to open a channel: %v", err)
		return
	}

	err = ch.ExchangeDeclare(
		ExchangeName, // name
		"topic",      // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		log.Printf("Warning: Failed to declare exchange: %v", err)
		return
	}

	RabbitConn = conn
	RabbitCh = ch
	log.Println("RabbitMQ initialized and connected")
}

func PublishEvent(routingKey string, payload interface{}) {
	if RabbitCh == nil {
		return // Ignore if not connected
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal rabbitmq payload: %v", err)
		return
	}

	err = RabbitCh.Publish(
		ExchangeName, // exchange
		routingKey,   // routing key
		false,        // mandatory
		false,        // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
			Expiration:  MessageExpiration,
		})
	if err != nil {
		log.Printf("Failed to publish a message: %v", err)
	}
}

func SubscribeEvents(routingKey string) (<-chan amqp.Delivery, func(), error) {
	if RabbitCh == nil {
		return nil, nil, nil // Silently ignore if not connected
	}

	q, err := RabbitCh.QueueDeclare(
		"",    // name - empty means random generated
		false, // durable
		true,  // delete when unused
		true,  // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return nil, nil, err
	}

	err = RabbitCh.QueueBind(
		q.Name,       // queue name
		routingKey,   // routing key
		ExchangeName, // exchange
		false,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	msgs, err := RabbitCh.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		// Because it's an auto-delete and exclusive queue, closing the connection or channel cleans it up.
		// However, we shouldn't close the global channel. We just delete the queue.
		RabbitCh.QueueDelete(q.Name, false, false, false)
	}

	return msgs, cleanup, nil
}
