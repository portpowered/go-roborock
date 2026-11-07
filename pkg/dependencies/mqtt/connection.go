package mqtt

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"time"

	"github.com/portpowered/go-roborock/internal/protocol"
)

// connection is one authenticated broker socket. It carries every device
// subscription of its account and ends on the first transport failure.
type connection struct {
	account          *account
	conn             net.Conn
	done             chan struct{}
	stopped          chan struct{}
	keepaliveStopped chan struct{}
	closeOnce        sync.Once
	writeGate        chan struct{}
	mu               sync.Mutex
	failure          error
	acks             map[uint16]chan error
	packetID         uint16
}

func newConnection(owner *account, conn net.Conn) *connection {
	return &connection{
		account: owner, conn: conn,
		done:             make(chan struct{}),
		stopped:          make(chan struct{}),
		keepaliveStopped: make(chan struct{}),
		closeOnce:        sync.Once{},
		writeGate:        make(chan struct{}, 1),
		mu:               sync.Mutex{},
		failure:          nil,
		acks:             make(map[uint16]chan error),
		packetID:         0,
	}
}

func (c *connection) closedError() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return transportError("session", c.failure)
}

func (c *connection) fail(err error) {
	closeSocket := false

	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.failure = err
		c.mu.Unlock()
		close(c.done)

		closeSocket = true
	})

	if closeSocket {
		_ = c.conn.Close()
	}
}

func (c *connection) write(ctx context.Context, data []byte) error {
	err := c.acquireWriter(ctx)
	if err != nil {
		return err
	}

	defer func() { <-c.writeGate }()

	deadline, _ := ctx.Deadline()

	err = c.conn.SetWriteDeadline(deadline)
	if err != nil {
		return transportError("write deadline", err)
	}

	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = c.conn.SetWriteDeadline(time.Now()); close(interrupted) })

	defer func() {
		if !stop() {
			<-interrupted
			c.fail(ctx.Err())
		}
	}()

	return c.writeAll(ctx, data)
}

func (c *connection) acquireWriter(ctx context.Context) error {
	if ctx.Err() != nil {
		return transportError("write", ctx.Err())
	}

	select {
	case c.writeGate <- struct{}{}:
	case <-ctx.Done():
		return transportError("write", ctx.Err())
	case <-c.done:
		return c.closedError()
	}

	if ctx.Err() != nil {
		<-c.writeGate

		return transportError("write", ctx.Err())
	}

	select {
	case <-c.done:
		<-c.writeGate

		return c.closedError()
	default:
		return nil
	}
}

func (c *connection) writeAll(ctx context.Context, data []byte) error {
	for len(data) > 0 {
		count, err := c.conn.Write(data)
		if err != nil {
			c.fail(err)

			if ctx.Err() != nil {
				return transportError("write", ctx.Err())
			}

			return transportError("write", err)
		}

		if count == 0 {
			c.fail(io.ErrNoProgress)

			return io.ErrNoProgress
		}

		data = data[count:]
	}

	return nil
}

func (c *connection) keepalive(owner context.Context) {
	ticker := time.NewTicker(c.account.hub.timing.ping)
	defer ticker.Stop()

	c.keepaliveTicks(owner, ticker.C)
}

func (c *connection) keepaliveTicks(owner context.Context, ticks <-chan time.Time) {
	defer close(c.keepaliveStopped)

	for {
		select {
		case <-c.done:
			return
		case <-ticks:
			ctx, cancel := context.WithTimeout(owner, handshakeTimeout)
			err := c.write(ctx, packet(protocol.MQTTPingReq, nil))

			cancel()

			if err != nil {
				c.fail(err)

				return
			}
		}
	}
}

// readLoop routes every incoming packet and always reports the connection's end
// to its account before it signals stopped.
func (c *connection) readLoop(owner context.Context) {
	defer close(c.stopped)
	defer c.account.lost(c)

	for {
		err := c.conn.SetReadDeadline(time.Now().Add(responseTimeout))
		if err != nil {
			c.fail(err)

			return
		}

		header, body, err := readPacket(c.conn)
		if err != nil {
			c.fail(err)

			return
		}

		err = c.dispatch(owner, header, body)
		if err != nil {
			c.fail(err)

			return
		}
	}
}

func (c *connection) dispatch(owner context.Context, header byte, body []byte) error {
	switch header {
	case protocol.MQTTPingResp:
		if len(body) == 0 {
			return nil
		}
	case protocol.MQTTSubAck, protocol.MQTTUnsubAck:
		c.acknowledge(header, body)

		return nil
	}

	if header>>protocol.MQTTKindShift != protocol.MQTTKindPublish {
		return errUnexpectedMQTTPacket
	}

	return c.receivePublish(owner, header, body)
}

// expectAck reserves a packet identifier whose acknowledgement the caller awaits.
func (c *connection) expectAck() (uint16, chan error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.packetID == 0 {
		c.packetID = protocol.MQTTSubscribeIdentifier
	} else {
		c.packetID++
		if c.packetID == 0 {
			c.packetID = protocol.MQTTSubscribeIdentifier
		}
	}

	reply := make(chan error, 1)
	c.acks[c.packetID] = reply

	return c.packetID, reply
}

func (c *connection) forgetAck(identifier uint16) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.acks, identifier)
}

func (c *connection) acknowledge(header byte, body []byte) {
	if len(body) < protocol.MQTTUint16Size {
		return
	}

	identifier := binary.BigEndian.Uint16(body[:protocol.MQTTUint16Size])

	c.mu.Lock()
	reply := c.acks[identifier]
	c.mu.Unlock()

	if reply == nil {
		return
	}

	var err error
	if header == protocol.MQTTSubAck {
		err = checkSubAck(header, body, identifier)
	}

	select {
	case reply <- err:
	default:
	}
}

// subscribe asks the broker for one device topic and waits for its SUBACK.
func (c *connection) subscribe(ctx context.Context, topic string) error {
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	identifier, reply := c.expectAck()
	defer c.forgetAck(identifier)

	err := c.write(ctx, subscribePacket(identifier, topic))
	if err != nil {
		return err
	}

	select {
	case err = <-reply:
		return err
	case <-ctx.Done():
		return transportError("subscribe", ctx.Err())
	case <-c.done:
		// A grant that arrived just before the connection ended still counts: the
		// device sees the loss on its first call rather than the opener retrying.
		select {
		case err = <-reply:
			return err
		default:
			return c.closedError()
		}
	}
}

// unsubscribe is best effort: the topic is already unrouted locally, and the
// connection ends when its last topic goes.
func (c *connection) unsubscribe(topic string) {
	ctx, cancel := context.WithTimeout(context.Background(), unsubscribeTimeout)
	defer cancel()

	identifier, _ := c.expectAck()
	defer c.forgetAck(identifier)

	_ = c.write(ctx, unsubscribePacket(identifier, topic))
}

func (c *connection) receivePublish(owner context.Context, header byte, body []byte) error {
	if len(body) < protocol.MQTTUint16Size {
		return errTruncatedMQTTTopic
	}

	topicLength := int(binary.BigEndian.Uint16(body[:protocol.MQTTUint16Size]))
	if topicLength > len(body)-protocol.MQTTUint16Size {
		return errTruncatedMQTTTopic
	}

	topic := string(body[protocol.MQTTUint16Size : protocol.MQTTUint16Size+topicLength])

	payload, err := c.acknowledgePublish(owner, header, body[protocol.MQTTUint16Size+topicLength:])
	if err != nil {
		return err
	}

	target := c.account.deviceFor(c, topic)
	if target == nil {
		return nil
	}

	err = target.receiveDeviceFrames(payload)
	if err != nil {
		// One device's malformed traffic ends only that device's subscription.
		c.account.retire(target, err)
	}

	return nil
}

func (c *connection) acknowledgePublish(owner context.Context, header byte, payload []byte) ([]byte, error) {
	qos := (header >> protocol.MQTTQoSShift) & protocol.MQTTQoSMask
	if qos > protocol.MQTTQoSAtLeastOnce {
		return nil, errUnsupportedIncomingMQTTQoS
	}

	if qos == protocol.MQTTQoSAtMostOnce {
		return payload, nil
	}

	if len(payload) < protocol.MQTTUint16Size {
		return nil, errTruncatedMQTTPacketIdentifier
	}

	ack := packet(protocol.MQTTPubAck, payload[:protocol.MQTTUint16Size])

	ctx, cancel := context.WithTimeout(owner, handshakeTimeout)
	defer cancel()

	err := c.write(ctx, ack)
	if err != nil {
		return nil, err
	}

	return payload[protocol.MQTTUint16Size:], nil
}
