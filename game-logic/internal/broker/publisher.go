// Package broker publica en RabbitMQ el resultado final de cada partida.
//
// Game Logic no conoce a quién le interesa el resultado: publica game.ended en
// el exchange bololo.events y cada interesado (hoy el Lobby Manager) enlaza su
// propia cola. El contrato está en docs/game-logic-design.md.
package broker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	Exchange   = "bololo.events"
	RoutingKey = "game.ended"
)

// Publisher mantiene una conexión perezosa: se abre en la primera publicación
// y se vuelve a abrir después de cualquier fallo.
type Publisher struct {
	url         string
	dialTimeout time.Duration

	mu      sync.Mutex
	conn    *amqp.Connection
	channel *amqp.Channel
	returns chan amqp.Return
}

func NewPublisher(url string) *Publisher {
	return &Publisher{url: url, dialTimeout: 5 * time.Second}
}

// PublishResult retorna nil solo cuando RabbitMQ confirmó el mensaje y lo dejó
// en al menos una cola. Si nadie tiene una cola enlazada, el broker lo
// devuelve (mandatory) y cuenta como fallo, para que el llamador reintente.
func (p *Publisher) PublishResult(ctx context.Context, gameID string, body []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.connect(); err != nil {
		return err
	}
	p.drainReturns()
	confirmation, err := p.channel.PublishWithDeferredConfirmWithContext(ctx, Exchange, RoutingKey, true, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    gameID,
		Timestamp:    time.Now().UTC(),
		Body:         body,
	})
	if err != nil {
		p.reset()
		return fmt.Errorf("publish %s: %w", RoutingKey, err)
	}
	acked, err := confirmation.WaitContext(ctx)
	if err != nil {
		p.reset()
		return fmt.Errorf("wait for RabbitMQ confirmation: %w", err)
	}
	if !acked {
		return errors.New("RabbitMQ rejected the message")
	}
	// RabbitMQ envía basic.return antes del ack, así que aquí ya habría llegado.
	select {
	case returned := <-p.returns:
		return fmt.Errorf("no queue is bound to %s: %s", RoutingKey, returned.ReplyText)
	default:
		return nil
	}
}

func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reset()
	return nil
}

func (p *Publisher) connect() error {
	if p.channel != nil && !p.channel.IsClosed() && !p.conn.IsClosed() {
		return nil
	}
	p.reset()
	conn, err := amqp.DialConfig(p.url, amqp.Config{Dial: amqp.DefaultDial(p.dialTimeout)})
	if err != nil {
		return fmt.Errorf("connect to RabbitMQ: %w", err)
	}
	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("open RabbitMQ channel: %w", err)
	}
	if err := channel.ExchangeDeclare(Exchange, amqp.ExchangeTopic, true, false, false, false, nil); err != nil {
		_ = conn.Close()
		return fmt.Errorf("declare exchange %s: %w", Exchange, err)
	}
	if err := channel.Confirm(false); err != nil {
		_ = conn.Close()
		return fmt.Errorf("enable publisher confirms: %w", err)
	}
	p.returns = channel.NotifyReturn(make(chan amqp.Return, 8))
	p.conn, p.channel = conn, channel
	return nil
}

func (p *Publisher) drainReturns() {
	for {
		select {
		case <-p.returns:
		default:
			return
		}
	}
}

func (p *Publisher) reset() {
	if p.channel != nil {
		_ = p.channel.Close()
	}
	if p.conn != nil {
		_ = p.conn.Close()
	}
	p.conn, p.channel, p.returns = nil, nil, nil
}
