package mqtt

import (
	"context"
	"crypto/aes"
	"crypto/md5" //nolint:gosec // Roborock's mandated account authentication uses MD5.
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"net"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// Open establishes a device connection without reconnecting or retrying commands.
// The context governs establishment. Callers own the returned session until Close.
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

	if conn == nil {
		return nil, invalid("connect", "dialer returned no connection")
	}

	session, username, password, err := newSession(conn, config)
	if err != nil {
		_ = conn.Close()

		return nil, err
	}

	stop := context.AfterFunc(opening, func() { session.fail(opening.Err()) })
	defer stop()

	err = session.handshake(opening, username, password)
	if err != nil {
		session.fail(err)

		if opening.Err() != nil {
			err = opening.Err()
		}

		return nil, transportError("handshake", err)
	}

	if !stop() {
		session.fail(opening.Err())

		return nil, transportError("handshake", opening.Err())
	}

	owner := context.WithoutCancel(ctx)
	go session.readLoop(owner)
	go session.keepalive(owner)

	return session, nil
}

func newSession(conn net.Conn, config Config) (*Session, string, string, error) {
	var session Session

	session.conn = conn
	session.config = config
	session.done = make(chan struct{})
	session.stopped = make(chan struct{})
	session.keepaliveStopped = make(chan struct{})
	session.writeGate = make(chan struct{}, 1)
	session.a01Gate = make(chan struct{}, 1)
	session.pending = make(map[int64]chan response)

	var nonce [aes.BlockSize]byte

	_, err := rand.Read(nonce[:])
	if err != nil {
		return nil, "", "", transportError("security", err)
	}

	endpointHash := md5.Sum([]byte(config.Key)) //nolint:gosec // Wire security endpoint uses MD5 bytes 8:14.
	session.security = dependencymodels.MQTTRPCSecurity{
		Endpoint: base64.StdEncoding.EncodeToString(endpointHash[8:14]),
		Nonce:    hex.EncodeToString(nonce[:]),
	}
	userHash := md5.Sum([]byte(config.User + ":" + config.Key))     //nolint:gosec // Vendor username derivation.
	secretHash := md5.Sum([]byte(config.Secret + ":" + config.Key)) //nolint:gosec // Vendor password derivation.
	username := hex.EncodeToString(userHash[:])[2:10]
	password := hex.EncodeToString(secretHash[:])[16:]
	session.publishTopic = protocol.MQTTPublishTopicPrefix + config.User + "/" + username + "/" + config.DeviceID
	session.subscribeTopic = protocol.MQTTSubscribeTopicPrefix + config.User + "/" + username + "/" + config.DeviceID

	return &session, username, password, nil
}

func defaultDial(ctx context.Context, network, address string) (net.Conn, error) {
	var config tls.Config

	config.MinVersion = tls.VersionTLS12
	dialer := tls.Dialer{NetDialer: nil, Config: &config}

	conn, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, transportError("connect", err)
	}

	return conn, nil
}

func (s *Session) handshake(ctx context.Context, username, password string) error {
	var client [8]byte

	_, err := rand.Read(client[:])
	if err != nil {
		return transportError("client ID", err)
	}

	err = s.write(ctx, connectPacket(hex.EncodeToString(client[:]), username, password))
	if err != nil {
		return err
	}

	header, body, err := readPacket(s.conn)
	if err != nil {
		return err
	}

	err = checkConnAck(header, body)
	if err != nil {
		return err
	}

	err = s.write(ctx, subscribePacket(s.subscribeTopic))
	if err != nil {
		return err
	}

	header, body, err = readPacket(s.conn)
	if err != nil {
		return err
	}

	return checkSubAck(header, body)
}

func checkConnAck(header byte, body []byte) error {
	if header != protocol.MQTTConnAck || len(body) != protocol.MQTTConnAckLength {
		return errBrokerRejectedMQTTConnection
	}

	if body[1] == protocol.MQTTBadUsernamePassword || body[1] == protocol.MQTTNotAuthorized {
		return roborockerrors.New(roborockerrors.Unauthorized, "mqtt handshake", "broker rejected credentials", nil)
	}

	if body[0] != 0 || body[1] != 0 {
		return errBrokerRejectedMQTTConnection
	}

	return nil
}

func checkSubAck(header byte, body []byte) error {
	if header != protocol.MQTTSubAck || len(body) != protocol.MQTTSubAckLength {
		return errBrokerRejectedMQTTSubscription
	}

	identifier := binary.BigEndian.Uint16(body[:protocol.MQTTUint16Size])
	if identifier != protocol.MQTTSubscribeIdentifier || body[protocol.MQTTUint16Size] != protocol.MQTTQoSAtMostOnce {
		return errBrokerRejectedMQTTSubscription
	}

	return nil
}
