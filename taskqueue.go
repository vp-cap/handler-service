package main

import (
	"log"
	"time"

	"github.com/streadway/amqp"
)

const (
	MaxRetryCount = 20
	SleepDuration = 10
)

func getChannelForMessages(conn *amqp.Connection) (<- chan amqp.Delivery, error) {
	ch, err := conn.Channel()
	if err != nil {
		log.Println(err)
		return nil, err
	}
	q, err := ch.QueueDeclare(
		"task_queue", // name
		true,         // durable
		false,        // delete when unused
		false,        // exclusive
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	err = ch.Qos(
		1,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
}

func getTaskQueueConnection() (*amqp.Connection, error) {
	conn, err := amqp.Dial(configs.Services.RabbitMq)
	for retry := 0; retry < MaxRetryCount && err != nil; retry++ {
		time.Sleep(SleepDuration * time.Second)
		conn, err = amqp.Dial(configs.Services.RabbitMq)
	}
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return conn, nil
}