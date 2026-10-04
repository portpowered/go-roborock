package mqtt

import (
	"context"
	"fmt"
	"github.com/portpowered/go-roborock/internal/protocol"
	"io"
	"time"
)

func (s *Session) write(ctx context.Context, data []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	select {
	case s.writeGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return s.closedError()
	}

	defer func() { <-s.writeGate }()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	select {
	case <-s.done:
		return s.closedError()
	default:
	}

	deadline, _ := ctx.Deadline()
	err := s.conn.SetWriteDeadline(deadline)
	if err != nil {
		return err
	}

	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = s.conn.SetWriteDeadline(time.Now()); close(interrupted) })

	defer func() {
		if !stop() {
			<-interrupted
			s.fail(ctx.Err())
		}
	}()

	for len(data) > 0 {
		count, err := s.conn.Write(data)
		if err != nil {
			s.fail(err)

			if ctx.Err() != nil {
				return ctx.Err()
			}

			return err
		}

		if count == 0 {
			s.fail(io.ErrNoProgress)

			return io.ErrNoProgress
		}

		data = data[count:]
	}

	return nil
}

func (s *Session) keepalive() {
	defer close(s.keepaliveStopped)

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), handshakeTimeout)
			err := s.write(ctx, packet(protocol.MQTTPingReq, nil))

			cancel()

			if err != nil {
				s.fail(err)

				return
			}
		}
	}
}

func (s *Session) readLoop() {
	defer close(s.stopped)

	for {
		if err := s.conn.SetReadDeadline(time.Now().Add(responseTimeout)); err != nil {
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

		if header>>4 != 3 {
			s.fail(fmt.Errorf("unexpected MQTT packet %d", header))

			return
		}

		if err = s.receivePublish(header, body); err != nil {
			s.fail(err)

			return
		}
	}
}
