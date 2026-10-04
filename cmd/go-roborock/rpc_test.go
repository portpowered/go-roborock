package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/md5" // Vendor-required derivation, independently implemented as a test oracle.
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/portpowered/go-roborock/pkg/roborock"
	"hash/crc32"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

const deviceJSON = `{"auth":{"mqtt":{"brokerUrl":"ssl://example.invalid:8883","user":"synthetic-user","secret":"synthetic-secret","key":"synthetic-key"}},"deviceId":"synthetic-device","localKey":"0123456789abcdef","protocol":"1.0"}`

func TestVacuumReadAndControlCommandsPairedMQTT(t *testing.T) {
	t.Parallel()

	cases := [][4]string{
		{"start", "app_start", `["ok"]`, `"acknowledged":true`},
		{"stop", "app_stop", `["ok"]`, `"acknowledged":true`},
		{"pause", "app_pause", `["ok"]`, `"acknowledged":true`},
		{"dock", "app_charge", `["ok"]`, `"acknowledged":true`},
		{"status", "get_status", `{"battery":85}`, `"battery":85`},
		{"consumables", "get_consumable", `{"main_brush_work_time":3600}`, `"main_brush_work_time":3600`},
		{"summary", "get_clean_summary", `{"clean_time":120}`, `"clean_time":120`},
	}
	for _, testCase := range cases {
		t.Run(testCase[0], func(t *testing.T) {
			t.Parallel()
			client, done := rpcTestClient(t, func(conn net.Conn) error {
				return replyRPC(conn, testCase[1], `[]`, testCase[2], nil)
			})

			var out, errOut bytes.Buffer

			err := run(context.Background(), []string{testCase[0]}, strings.NewReader(deviceJSON), &out, &errOut, noEnvironment, client)
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(out.String(), testCase[3]) {
				t.Fatal("missing public result")
			}

			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestCancellationClosesPendingRPC(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, done := rpcTestClient(t, func(conn net.Conn) error { return replyRPC(conn, "get_status", `[]`, "", cancel) })

	var out, errOut bytes.Buffer

	err := run(ctx, []string{"status"}, strings.NewReader(deviceJSON), &out, &errOut, noEnvironment, client)
	if err == nil || out.Len() != 0 {
		t.Fatal("canceled RPC returned result")
	}

	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCameraLifecycleAndOutput(t *testing.T) {
	t.Parallel()

	for _, success := range []bool{false, true} {
		client, done := rpcTestClient(t, func(conn net.Conn) error {
			pairs := [][3]string{
				{"check_homesec_password", `{"password":"0123456789abcdef0123456789abcdef"}`, `["ok"]`},
				{"start_camera_preview", `{"password":"0123456789abcdef0123456789abcdef","quality":"hd"}`, `["ok"]`},
			}

			if success {
				offer := base64.StdEncoding.EncodeToString([]byte(`{"sdp":"synthetic-offer","type":"offer"}`))
				pairs = append(pairs, [3]string{"get_turn_server", `{}`, `{"url":"turn:example.invalid","username":"user","credential":"secret"}`},
					[3]string{"send_sdp_to_robot", fmt.Sprintf(`{"app_sdp":%q}`, offer), `["ok"]`},
					[3]string{"get_device_sdp", `{}`, `{"sdp":"synthetic-ICE-credential"}`})
			} else {
				pairs = append(pairs, [3]string{"get_turn_server", `{}`, `{"unexpected":"synthetic"}`})
			}

			pairs = append(pairs, [3]string{"stop_camera_preview", `{}`, `["ok"]`})
			for _, pair := range pairs {
				err := replyRPC(conn, pair[0], pair[1], pair[2], nil)
				if err != nil {
					return err
				}
			}

			return nil
		})
		input := strings.TrimSuffix(deviceJSON, "}") + `,"camera":{"pattern_password":"0123456789abcdef0123456789abcdef","sdp_offer":"synthetic-offer"}}`

		var out, errOut bytes.Buffer

		err := run(context.Background(), []string{"camera"}, strings.NewReader(input), &out, &errOut, noEnvironment, client)
		if success && err != nil {
			t.Fatal(err)
		}

		if !success && err == nil {
			t.Fatal("invalid TURN accepted")
		}

		if success && out.String() != "{\"negotiated\":true}\n" {
			t.Fatal("camera credentials disclosed")
		}

		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

func rpcTestClient(t *testing.T, exchange func(net.Conn) error) (*roborock.Client, <-chan error) {
	t.Helper()

	conn, server := net.Pipe()

	t.Cleanup(func() { _ = conn.Close(); _ = server.Close() })

	done := make(chan error, 1)

	go func() {
		_ = server.SetDeadline(time.Now().Add(5 * time.Second))
		done <- replayEstablishment(server, exchange)
	}()

	client, err := roborock.NewClient(roborock.WithMQTTDial(func(_ context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != "example.invalid:8883" {
			return nil, errors.New("dial mismatch")
		}

		return conn, nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	return client, done
}

func replyRPC(conn net.Conn, method, params, result string, cancel context.CancelFunc) error {
	header, body, err := readMQTTPacket(conn)
	if err != nil {
		return err
	}

	topic := "rr/m/i/synthetic-user/f8cd4bde/synthetic-device"

	prefix := append([]byte{0, byte(len(topic))}, []byte(topic)...)
	if header != 48 || !bytes.HasPrefix(body, prefix) {
		return errors.New("RPC topic mismatch")
	}

	frame := body[len(prefix):]

	plain, err := testFrame(frame, nil)
	if err != nil {
		return err
	}

	identifier, err := matchRPCPlain(plain, method, params)
	if err != nil {
		return err
	}
	if cancel != nil {
		cancel()

		return nil
	}

	inner := fmt.Sprintf(`{"id":%d,"result":%s}`, identifier, result)

	quoted, err := json.Marshal(inner)
	if err != nil {
		return err
	}

	response := []byte(fmt.Sprintf(`{"dps":{"102":%s},"t":1700000000}`, quoted))

	encoded, err := testFrame(frame, response)
	if err != nil {
		return err
	}

	topic = "rr/m/o/synthetic-user/f8cd4bde/synthetic-device"
	_, err = conn.Write(mqttTestPacket(48, append(append([]byte{0, byte(len(topic))}, []byte(topic)...), encoded...)))

	return err
}

func equalJSON(left, right []byte) bool {
	var lhs, rhs any
	if json.Unmarshal(left, &lhs) != nil || json.Unmarshal(right, &rhs) != nil {
		return false
	}

	leftJSON, err := json.Marshal(lhs)
	if err != nil {
		return false
	}

	rightJSON, err := json.Marshal(rhs)

	return err == nil && bytes.Equal(leftJSON, rightJSON)
}

// testFrame independently decodes a request or encodes a reply using the pinned V1 wire formula.
func testFrame(frame, payload []byte) ([]byte, error) {
	if len(frame) < 39 || string(frame[:3]) != "1.0" || binary.BigEndian.Uint16(frame[15:17]) != 101 || int(binary.BigEndian.Uint16(frame[17:19])) != len(frame)-23 || binary.BigEndian.Uint32(frame[len(frame)-4:]) != crc32.ChecksumIEEE(frame[:len(frame)-4]) {
		return nil, errors.New("frame mismatch")
	}

	stamp := fmt.Sprintf("%08x", binary.BigEndian.Uint32(frame[11:15]))
	reordered := string([]byte{stamp[5], stamp[6], stamp[3], stamp[7], stamp[1], stamp[2], stamp[0], stamp[4]})
	key := md5.Sum([]byte(reordered + "0123456789abcdef" + "TXdfu$jyZ#TZHsg4"))

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	if payload == nil {
		encrypted := frame[19 : len(frame)-4]
		if len(encrypted)%aes.BlockSize != 0 {
			return nil, errors.New("cipher length mismatch")
		}

		plain := make([]byte, len(encrypted))
		for offset := 0; offset < len(plain); offset += aes.BlockSize {
			block.Decrypt(plain[offset:offset+aes.BlockSize], encrypted[offset:offset+aes.BlockSize])
		}

		padding := int(plain[len(plain)-1])
		if padding < 1 || padding > aes.BlockSize || !bytes.Equal(plain[len(plain)-padding:], bytes.Repeat([]byte{byte(padding)}, padding)) {
			return nil, errors.New("padding mismatch")
		}

		return plain[:len(plain)-padding], nil
	}

	padding := aes.BlockSize - len(payload)%aes.BlockSize

	plain := append(bytes.Clone(payload), bytes.Repeat([]byte{byte(padding)}, padding)...)
	payloadLength := len(plain)
	if payloadLength < 0 || payloadLength > 65535 {
		return nil, errors.New("fixture payload exceeds uint16 wire limit")
	}

	output := make([]byte, 19+len(plain)+4)
	copy(output, frame[:19])
	binary.BigEndian.PutUint16(output[15:17], 102)
	binary.BigEndian.PutUint16(output[17:19], uint16(payloadLength))

	for offset := 0; offset < len(plain); offset += aes.BlockSize {
		block.Encrypt(output[19+offset:19+offset+aes.BlockSize], plain[offset:offset+aes.BlockSize])
	}

	binary.BigEndian.PutUint32(output[len(output)-4:], crc32.ChecksumIEEE(output[:len(output)-4]))

	return output, nil
}

func mqttTestPacket(header byte, body []byte) []byte {
	packet := []byte{header}

	for remaining := len(body); ; {
		digit := byte(remaining % 128)

		remaining /= 128
		if remaining > 0 {
			digit |= 128
		}

		packet = append(packet, digit)

		if remaining == 0 {
			break
		}
	}

	return append(packet, body...)
}

func readMQTTPacket(reader io.Reader) (byte, []byte, error) {
	var header [1]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return 0, nil, err
	}

	length, multiplier := 0, 1

	for range 4 {
		var digit [1]byte
		if _, err := io.ReadFull(reader, digit[:]); err != nil {
			return 0, nil, err
		}

		length += int(digit[0]&127) * multiplier
		if length > 1<<20 {
			return 0, nil, errors.New("oversized MQTT")
		}

		if digit[0]&128 == 0 {
			body := make([]byte, length)
			_, err := io.ReadFull(reader, body)

			return header[0], body, err
		}

		multiplier *= 128
	}

	return 0, nil, errors.New("malformed MQTT")
}
func matchRPCPlain(plain []byte, method, params string) (int64, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(plain, &envelope); err != nil {
		return 0, err
	}

	var dps map[string]string
	if err := json.Unmarshal(envelope["dps"], &dps); err != nil {
		return 0, err
	}

	if len(envelope) != 2 || len(dps) != 1 {
		return 0, errors.New("RPC envelope mismatch")
	}

	var stamp int64
	if err := json.Unmarshal(envelope["t"], &stamp); err != nil || stamp <= 0 {
		return 0, errors.New("timestamp mismatch")
	}

	var request map[string]json.RawMessage
	if err := json.Unmarshal([]byte(dps["101"]), &request); err != nil {
		return 0, err
	}

	var (
		identifier   int64
		actualMethod string
	)

	if json.Unmarshal(request["id"], &identifier) != nil || identifier <= 0 {
		return 0, errors.New("identifier mismatch")
	}

	if json.Unmarshal(request["method"], &actualMethod) != nil || actualMethod != method || len(request) != 4 || !equalJSON(request["params"], []byte(params)) {
		return 0, errors.New("RPC request mismatch")
	}

	var security map[string]string
	if err := json.Unmarshal(request["security"], &security); err != nil {
		return 0, err
	}

	if security["endpoint"] != "goOmJ7S+" || len(security) != 2 {
		return 0, errors.New("endpoint mismatch")
	}

	if nonce, err := hex.DecodeString(security["nonce"]); err != nil || len(nonce) != 16 {
		return 0, errors.New("nonce mismatch")
	}

	return identifier, nil
}
