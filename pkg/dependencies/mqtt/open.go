package mqtt

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
	"net"
)

func Open(ctx context.Context, config Config, dial DialFunc) (*Session, error) {
	endpoint, err := validateConfig(config)
	if err != nil {
		return nil, transportError("open", err)
	}

	if config.Protocol == "" {
		config.Protocol = protocol.MQTTVersionV1
	}

	if dial == nil {
		dial = defaultDial
	}

	opening, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	conn, err := dial(opening, "tcp", endpoint.Host)
	if err != nil {
		return nil, transportError("connect", err)
	}

	stop := context.AfterFunc(opening, func() { _ = conn.Close() })
	defer stop()

	userHash := md5.Sum([]byte(config.User + ":" + config.Key))
	secretHash := md5.Sum([]byte(config.Secret + ":" + config.Key))
	username := hex.EncodeToString(userHash[:])[2:10]
	password := hex.EncodeToString(secretHash[:])[16:]
	session := &Session{conn: conn, config: config, done: make(chan struct{}), stopped: make(chan struct{}), keepaliveStopped: make(chan struct{}), writeGate: make(chan struct{}, 1), a01Gate: make(chan struct{}, 1), pending: make(map[int64]chan response)}

	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		_ = conn.Close()

		return nil, transportError("security", err)
	}

	endpointHash := md5.Sum([]byte(config.Key))
	session.security = dependencymodels.MQTTRPCSecurity{Endpoint: base64.StdEncoding.EncodeToString(endpointHash[8:14]), Nonce: hex.EncodeToString(nonce[:])}
	session.publishTopic = protocol.MQTTPublishTopicPrefix + config.User + "/" + username + "/" + config.DeviceID

	session.subscribeTopic = protocol.MQTTSubscribeTopicPrefix + config.User + "/" + username + "/" + config.DeviceID

	if err = session.handshake(opening, username, password); err != nil {
		_ = conn.Close()

		if opening.Err() != nil {
			err = opening.Err()
		}

		return nil, transportError("handshake", err)
	}

	if !stop() {
		_ = conn.Close()

		return nil, transportError("handshake", opening.Err())
	}

	go session.readLoop()
	go session.keepalive()

	return session, nil
}

func defaultDial(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := tls.Dialer{Config: &tls.Config{MinVersion: tls.VersionTLS12}}

	return dialer.DialContext(ctx, network, address)
}

func (s *Session) handshake(ctx context.Context, username, password string) error {
	var client [8]byte
	if _, err := rand.Read(client[:]); err != nil {
		return err
	}

	if err := s.write(ctx, connectPacket(hex.EncodeToString(client[:]), username, password)); err != nil {
		return err
	}

	header, body, err := readPacket(s.conn)
	if err != nil {
		return err
	}

	if header != protocol.MQTTConnAck || len(body) != 2 || body[0] > 1 || body[1] != 0 {
		if header == protocol.MQTTConnAck && len(body) == 2 && (body[1] == 4 || body[1] == 5) {
			return roborockerrors.New(roborockerrors.Unauthorized, "mqtt handshake", "broker rejected credentials", nil)
		}

		return errBrokerRejectedMQTTConnection
	}

	if err = s.write(ctx, subscribePacket(s.subscribeTopic)); err != nil {
		return err
	}

	header, body, err = readPacket(s.conn)
	if err != nil {
		return err
	}

	if header != protocol.MQTTSubAck || len(body) != 3 || binary.BigEndian.Uint16(body[:2]) != 1 || body[2] != 0 {
		return errBrokerRejectedMQTTSubscription
	}

	return nil
}
