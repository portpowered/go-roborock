package mqtt

import (
	"context"
	"io"
	"time"

	"github.com/portpowered/go-roborock/internal/protocol"
)

func (s *Session) write(ctx context.Context, data []byte) error {
	err := s.acquireWriter(ctx)
	if err != nil {
		return err
	}

	defer func() { <-s.writeGate }()

	deadline, _ := ctx.Deadline()

	err = s.conn.SetWriteDeadline(deadline)
	if err != nil {
		return transportError("write deadline", err)
	}

	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = s.conn.SetWriteDeadline(time.Now()); close(interrupted) })

	defer func() {
		if !stop() {
			<-interrupted
			s.fail(ctx.Err())
		}
	}()

	return s.writeAll(ctx, data)
}

func (s *Session) acquireWriter(ctx context.Context) error {
	if ctx.Err() != nil {
		return transportError("write", ctx.Err())
	}

	select {
	case s.writeGate <- struct{}{}:
	case <-ctx.Done():
		return transportError("write", ctx.Err())
	case <-s.done:
		return s.closedError()
	}

	if ctx.Err() != nil {
		<-s.writeGate

		return transportError("write", ctx.Err())
	}

	select {
	case <-s.done:
		<-s.writeGate

		return s.closedError()
	default:
		return nil
	}
}

func (s *Session) writeAll(ctx context.Context, data []byte) error {
	for len(data) > 0 {
		count, err := s.conn.Write(data)
		if err != nil {
			s.fail(err)

			if ctx.Err() != nil {
				return transportError("write", ctx.Err())
			}

			return transportError("write", err)
		}

		if count == 0 {
			s.fail(io.ErrNoProgress)

			return io.ErrNoProgress
		}

		data = data[count:]
	}

	return nil
}

func (s *Session) keepalive(owner context.Context) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	s.keepaliveTicks(owner, ticker.C)
}

func (s *Session) keepaliveTicks(owner context.Context, ticks <-chan time.Time) {
	defer close(s.keepaliveStopped)

	for {
		select {
		case <-s.done:
			return
		case <-ticks:
			ctx, cancel := context.WithTimeout(owner, handshakeTimeout)
			err := s.write(ctx, packet(protocol.MQTTPingReq, nil))

			cancel()

			if err != nil {
				s.fail(err)

				return
			}
		}
	}
}

func (s *Session) readLoop(owner context.Context) {
	defer close(s.stopped)

	for {
		err := s.conn.SetReadDeadline(time.Now().Add(responseTimeout))
		if err != nil {
			s.fail(err)

			return
		}

		header, body, err := readPacket(s.conn)
		if err != nil {
			s.fail(err)

			return
		}

		if header == protocol.MQTTPingResp && len(body) == 0 {
			continue
		}

		if header>>protocol.MQTTKindShift != protocol.MQTTKindPublish {
			s.fail(errUnexpectedMQTTPacket)

			return
		}

		err = s.receivePublish(owner, header, body)
		if err != nil {
			s.fail(err)

			return
		}
	}
}
