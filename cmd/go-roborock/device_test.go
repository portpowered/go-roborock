package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/go-roborock/pkg/roborock"
)

// TestDeviceFailureClosesConnection pairs a synthetic MQTT establishment
// transcript with an unsupported appliance operation and requires socket EOF.
func TestDeviceFailureClosesConnection(t *testing.T) {
	t.Parallel()

	connection, server := net.Pipe()

	t.Cleanup(func() { _ = connection.Close(); _ = server.Close() })

	completed := make(chan error, 1)

	go func() {
		_ = server.SetDeadline(time.Now().Add(3 * time.Second))
		completed <- replayEstablishment(server)
	}()

	client, err := roborock.NewClient(roborock.WithMQTTDial(func(_ context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != "example.invalid:8883" {
			return nil, errors.New("dial mismatch")
		}

		return connection, nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer

	input := `{"auth":{"mqtt":{"brokerUrl":"ssl://example.invalid:8883","user":"synthetic-user","secret":"synthetic-secret","key":"synthetic-key"}},"deviceId":"synthetic-device","localKey":"0123456789abcdef","protocol":"1.0","dyadProperties":[1]}`

	err = run(context.Background(), []string{"dyad"}, strings.NewReader(input), &stdout, &stderr, noEnvironment, client)
	if err == nil || stdout.Len() != 0 {
		t.Fatal("unsupported device operation succeeded")
	}

	select {
	case replayErr := <-completed:
		if replayErr != nil {
			t.Fatal(replayErr)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("device connection was not closed")
	}
}

func replayEstablishment(connection net.Conn, exchanges ...func(net.Conn) error) error {
	header, body, err := readSmallPacket(connection)
	if err != nil {
		return err
	}

	if header != 16 || len(body) != 56 || !bytes.Equal(body[:10], []byte{0, 4, 'M', 'Q', 'T', 'T', 4, 194, 0, 45}) || !bytes.Equal(body[10:12], []byte{0, 16}) {
		return errors.New("CONNECT protocol mismatch")
	}

	_, err = hex.DecodeString(string(body[12:28]))
	if err != nil {
		return errors.New("CONNECT volatile identity must be 16 hexadecimal characters")
	}

	if !bytes.Equal(body[28:], append([]byte{0, 8}, append([]byte("f8cd4bde"), append([]byte{0, 16}, []byte("98cd006946fbb310")...)...)...)) {
		return errors.New("CONNECT credentials mismatch")
	}

	_, err = connection.Write([]byte{32, 2, 0, 0})
	if err != nil {
		return err
	}

	header, body, err = readSmallPacket(connection)
	if err != nil {
		return err
	}

	topic := fixtureOutputTopic

	expected := append([]byte{0, 1, 0, byte(len(topic) & 255)}, append([]byte(topic), 0)...)
	if header != 130 || !bytes.Equal(body, expected) {
		return errors.New("SUBSCRIBE account topic mismatch")
	}

	_, err = connection.Write([]byte{144, 3, 0, 1, 0})
	if err != nil {
		return err
	}

	for _, exchange := range exchanges {
		err := exchange(connection)
		if err != nil {
			return err
		}
	}

	var extra [1]byte

	_, err = connection.Read(extra[:])
	if !errors.Is(err, io.EOF) {
		return errors.New("expected connection close after unsupported operation")
	}

	return nil
}

func readSmallPacket(reader io.Reader) (byte, []byte, error) {
	var prefix [2]byte

	_, err := io.ReadFull(reader, prefix[:])
	if err != nil {
		return 0, nil, err
	}

	if prefix[1] >= 128 {
		return 0, nil, errors.New("unexpected oversized packet")
	}

	body := make([]byte, int(prefix[1]))
	_, err = io.ReadFull(reader, body)

	return prefix[0], body, err
}
