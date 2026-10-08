package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
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
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fixtureRPCResult = `["ok"]`
const fixtureOutputTopic = "rr/m/o/synthetic-user/f8cd4bde/synthetic-device"

const deviceJSON = `{"auth":{"mqtt":{"brokerUrl":"ssl://example.invalid:8883","user":"synthetic-user","secret":"synthetic-secret","key":"synthetic-key"}},"deviceId":"synthetic-device","localKey":"0123456789abcdef","protocol":"1.0"}`

func TestVacuumReadAndControlCommandsPairedMQTT(t *testing.T) {
	t.Parallel()

	cases := [][4]string{
		{commandStart, "app_start", fixtureRPCResult, fixtureAcknowledged},
		{commandStop, "app_stop", fixtureRPCResult, fixtureAcknowledged},
		{commandPause, "app_pause", fixtureRPCResult, fixtureAcknowledged},
		{commandDock, "app_charge", fixtureRPCResult, fixtureAcknowledged},
		{commandStatus, "get_status", `{"battery":85}`, `"battery":85`},
		{commandConsumables, "get_consumable", `{"main_brush_work_time":3600}`, `"main_brush_work_time":3600`},
		{commandSummary, "get_clean_summary", `{"clean_time":120}`, `"clean_time":120`},
	}
	for _, testCase := range cases {
		t.Run(testCase[0], func(t *testing.T) {
			t.Parallel()
			client, done := rpcTestClient(t, func(conn net.Conn) error {
				return replyRPC(conn, testCase[1], `[]`, testCase[2], nil)
			})

			var out, errOut bytes.Buffer

			err := run(context.Background(), []string{testCase[0], fixtureInputFlag, "-"}, strings.NewReader(deviceJSON), &out, &errOut, noEnvironment, client)
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(out.String(), testCase[3]) {
				t.Fatal("missing public result")
			}

			err = <-done
			if err != nil {
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

	err := run(ctx, []string{commandStatus}, strings.NewReader(deviceJSON), &out, &errOut, noEnvironment, client)
	if err == nil || out.Len() != 0 {
		t.Fatal("canceled RPC returned result")
	}

	err = <-done
	if err != nil {
		t.Fatal(err)
	}
}

func TestCameraLifecycleAndOutput(t *testing.T) {
	t.Parallel()

	for _, success := range []bool{false, true} {
		client, done := rpcTestClient(t, func(conn net.Conn) error {
			pairs := [][3]string{
				{"check_homesec_password", `{"password":"0123456789abcdef0123456789abcdef"}`, fixtureRPCResult},
				{"start_camera_preview", `{"password":"0123456789abcdef0123456789abcdef","quality":"hd"}`, fixtureRPCResult},
			}

			if success {
				offer := base64.StdEncoding.EncodeToString([]byte(`{"sdp":"synthetic-offer","type":"offer"}`))
				pairs = append(pairs, [3]string{"get_turn_server", `{}`, `{"url":"turn:example.invalid","username":"user","credential":"secret"}`},
					[3]string{"send_sdp_to_robot", fmt.Sprintf(`{"app_sdp":%q}`, offer), fixtureRPCResult},
					[3]string{"get_device_sdp", `{}`, `{"sdp":"synthetic-ICE-credential"}`})
			} else {
				pairs = append(pairs, [3]string{"get_turn_server", `{}`, `{"unexpected":"synthetic"}`})
			}

			pairs = append(pairs, [3]string{"stop_camera_preview", `{}`, fixtureRPCResult})
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

		err = <-done
		if err != nil {
			t.Fatal(err)
		}
	}
}

func rpcTestClient(
	t *testing.T, exchange func(net.Conn) error, options ...roborock.Option,
) (*roborock.Client, <-chan error) {
	t.Helper()

	conn, server := net.Pipe()

	t.Cleanup(func() { _ = conn.Close(); _ = server.Close() })

	done := make(chan error, 1)

	go func() {
		_ = server.SetDeadline(time.Now().Add(5 * time.Second))
		done <- replayEstablishment(server, exchange)
	}()

	options = append(options, roborock.WithMQTTDial(func(_ context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != "example.invalid:8883" {
			return nil, errors.New("dial mismatch")
		}

		return conn, nil
	}))

	client, err := roborock.NewClient(options...)
	if err != nil {
		t.Fatal(err)
	}

	return client, done
}

// customerInventoryHTTP replays two strict synthetic inventory exchanges for each customer command.
// Room discovery repeats those exchanges once to resolve the cloud room name.
type customerInventoryHTTP struct {
	inventory inventoryHTTP
	calls     int
	expected  int
}

func (fixture *customerInventoryHTTP) Do(request *http.Request) (*http.Response, error) {
	fixture.calls++
	if fixture.calls > fixture.expected {
		return nil, errors.New("unexpected customer inventory exchange")
	}

	if fixture.inventory.calls == 2 {
		fixture.inventory.calls = 0
	}

	response, err := fixture.inventory.Do(request)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(response.Body)

	_ = response.Body.Close()

	if err != nil {
		return nil, err
	}

	body := strings.ReplaceAll(string(data), "synthetic-local-secret", fixtureLocalKey)
	body = strings.Replace(body, `"devices":[`, `"rooms":[{"id":1001,"name":"Kitchen"}],"devices":[`, 1)
	response.Body = io.NopCloser(strings.NewReader(body))

	return response, nil
}

type customerCommandReplay struct {
	name      string
	args      []string
	method    string
	params    string
	response  string
	result    string
	httpCalls int
}

func TestCustomerGuideCommandsPairedInventoryAndMQTT(t *testing.T) {
	t.Parallel()

	cases := []customerCommandReplay{
		{name: commandStart, args: []string{commandDevices, fixtureVacuum, fixtureDeviceID, commandStart}, method: "app_start", params: `[]`, response: fixtureRPCResult, result: fixtureAcknowledged, httpCalls: 2},
		{name: commandPause, args: []string{commandDevices, fixtureVacuum, fixtureDeviceID, commandPause}, method: "app_pause", params: `[]`, response: fixtureRPCResult, result: fixtureAcknowledged, httpCalls: 2},
		{name: commandDock, args: []string{commandDevices, fixtureVacuum, fixtureDeviceID, commandDock}, method: "app_charge", params: `[]`, response: fixtureRPCResult, result: fixtureAcknowledged, httpCalls: 2},
		{name: commandStop, args: []string{commandDevices, fixtureVacuum, fixtureDeviceID, commandStop}, method: "app_stop", params: `[]`, response: fixtureRPCResult, result: fixtureAcknowledged, httpCalls: 2},
		{name: commandStatus, args: []string{commandDevices, fixtureVacuum, fixtureDeviceID, commandStatus}, method: "get_status", params: `[]`, response: `{"battery":85}`, result: `"battery":85`, httpCalls: 2},
		{name: commandRooms, args: []string{commandRooms, fixtureList, deviceFlag, fixtureDeviceID}, method: "get_room_mapping", params: `[]`, response: `[[16,"1001"]]`, result: "16       Kitchen", httpCalls: 4},
		{name: commandCleanRooms, args: []string{commandRooms, fixtureClean, "16", deviceFlag, fixtureDeviceID}, method: "app_segment_clean", params: `[{"segments":[16],"repeat":1}]`, response: fixtureRPCResult, result: fixtureAcknowledged, httpCalls: 2},
		{name: commandMaps, args: []string{commandMaps, fixtureList, deviceFlag, fixtureDeviceID}, method: "get_multi_maps_list", params: `[]`, response: `[{"map_info":[{"mapFlag":3,"name":"Upstairs"}]}]`, result: "Upstairs", httpCalls: 2},
		{name: commandSelectMap, args: []string{commandMaps, fixtureSelect, "3", deviceFlag, fixtureDeviceID}, method: "load_multi_map", params: `[3]`, response: fixtureRPCResult, result: fixtureAcknowledged, httpCalls: 2},
		{name: commandCleanZones, args: []string{fixtureZones, fixtureClean, deviceFlag, fixtureDeviceID, zoneFlag, "100,200,300,400", zoneFlag, "500,600,700,800", "--repeats", "2"}, method: "app_zoned_clean", params: `[[100,200,300,400,2],[500,600,700,800,2]]`, response: fixtureRPCResult, result: fixtureAcknowledged, httpCalls: 2},
		{name: commandMap, args: []string{commandMaps, "show", deviceFlag, fixtureDeviceID}, method: "", params: "", response: "", result: `"point":{"x":1000,"y":2000}`, httpCalls: 2},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			httpFixture := &customerInventoryHTTP{inventory: inventoryHTTP{calls: 0}, calls: 0, expected: testCase.httpCalls}

			exchange := func(connection net.Conn) error {
				return replyRPC(connection, testCase.method, testCase.params, testCase.response, nil)
			}

			if testCase.name == commandMap {
				exchange = replySyntheticMap
			}

			client, completed := rpcTestClient(t, exchange, roborock.WithHTTPClient(httpFixture))
			profile := customerCommandProfile(t)

			var stdout, stderr bytes.Buffer

			err := run(t.Context(), append(testCase.args, profileFlag, profile), strings.NewReader(""), &stdout, &stderr, noEnvironment, client)
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(stdout.String(), testCase.result) || httpFixture.calls != testCase.httpCalls {
				t.Fatal("customer result or inventory exchange count mismatch")
			}

			for _, secret := range []string{fixtureLocalKey, fixtureToken, fixtureSecret, fixtureSigningKey} {
				if strings.Contains(stdout.String()+stderr.String(), secret) {
					t.Fatal("customer command disclosed account or device secrets")
				}
			}

			err = <-completed
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func customerCommandProfile(t *testing.T) string {
	t.Helper()

	profile := filepath.Join(t.TempDir(), "private", "profile.json")

	err := saveProfile(profile, roborock.AuthContext{BaseURL: fixtureBaseURL, ClientID: fixtureClientID, Token: fixtureToken, Mqtt: roborock.MQTTAuth{APIURL: fixtureBaseURL, BrokerURL: "ssl://example.invalid:8883", User: fixtureCustomerMQTTUser, Secret: fixtureSecret, SigningKey: fixtureSigningKey, Key: "synthetic-key"}})
	if err != nil {
		t.Fatal(err)
	}

	return profile
}

func replyRPC(conn net.Conn, method, params, result string, cancel context.CancelFunc) error {
	header, body, err := readMQTTPacket(conn)
	if err != nil {
		return err
	}

	topic := "rr/m/i/synthetic-user/f8cd4bde/synthetic-device"

	prefix := append([]byte{0, byte(len(topic) & 255)}, []byte(topic)...)
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

	topic = fixtureOutputTopic
	_, err = conn.Write(mqttTestPacket(48, append(append([]byte{0, byte(len(topic) & 255)}, []byte(topic)...), encoded...)))

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
	key := md5.Sum([]byte(reordered + fixtureLocalKey + "TXdfu$jyZ#TZHsg4"))

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

	_, err := io.ReadFull(reader, header[:])
	if err != nil {
		return 0, nil, err
	}

	length, multiplier := 0, 1

	for range 4 {
		var digit [1]byte

		_, err = io.ReadFull(reader, digit[:])
		if err != nil {
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

	err := json.Unmarshal(plain, &envelope)
	if err != nil {
		return 0, err
	}

	var dps map[string]string

	err = json.Unmarshal(envelope["dps"], &dps)
	if err != nil {
		return 0, err
	}

	if len(envelope) != 2 || len(dps) != 1 {
		return 0, errors.New("RPC envelope mismatch")
	}

	var stamp int64

	err = json.Unmarshal(envelope["t"], &stamp)
	if err != nil || stamp <= 0 {
		return 0, errors.New("timestamp mismatch")
	}

	var request map[string]json.RawMessage

	err = json.Unmarshal([]byte(dps["101"]), &request)
	if err != nil {
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

	err = matchRPCSecurity(request["security"])
	if err != nil {
		return 0, err
	}

	return identifier, nil
}
func matchRPCSecurity(raw json.RawMessage) error {
	var security map[string]string

	err := json.Unmarshal(raw, &security)
	if err != nil {
		return err
	}

	if security["endpoint"] != "goOmJ7S+" || len(security) != 2 {
		return errors.New("endpoint mismatch")
	}

	nonce, err := hex.DecodeString(security["nonce"])
	if err != nil || len(nonce) != 16 {
		return errors.New("nonce mismatch")
	}

	return nil
}

func TestZoneAndRoomCleaningPairedMQTT(t *testing.T) {
	t.Parallel()

	cases := [][3]string{
		{commandCleanZones, `,"cleanZones":{"zones":[{"x1":100,"y1":200,"x2":300,"y2":400,"repeats":2}]}`, `[[100,200,300,400,2]]`},
		{commandCleanRooms, `,"cleanRooms":{"segments":[16,17],"repeats":2}`, `[{"segments":[16,17],"repeat":2}]`},
	}
	for _, testCase := range cases {
		t.Run(testCase[0], func(t *testing.T) {
			t.Parallel()

			method := "app_zoned_clean"
			if testCase[0] == commandCleanRooms {
				method = "app_segment_clean"
			}

			client, done := rpcTestClient(t, func(conn net.Conn) error {
				return replyRPC(conn, method, testCase[2], fixtureRPCResult, nil)
			})
			input := strings.TrimSuffix(deviceJSON, "}") + testCase[1] + "}"

			var out, errOut bytes.Buffer

			err := run(context.Background(), []string{testCase[0], fixtureInputFlag, "-"}, strings.NewReader(input), &out, &errOut, noEnvironment, client)
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(out.String(), fixtureAcknowledged) {
				t.Fatal("missing command acknowledgement")
			}

			replayErr := <-done
			if replayErr != nil {
				t.Fatal(replayErr)
			}
		})
	}
}

func TestMapCommandsWithoutWireSideEffects(t *testing.T) {
	t.Parallel()

	cases := [][3]string{
		{commandCapabilities, "", `"mapContent":true`},
		{commandMap, `,"mapId":"3"`, ""},
		{commandTrace, "", ""},
		{commandCleanRooms, `,"cleanRooms":{"segments":[],"repeats":1}`, ""},
	}
	for _, testCase := range cases {
		t.Run(testCase[0], func(t *testing.T) {
			t.Parallel()
			client, done := rpcTestClient(t, func(_ net.Conn) error { return nil })
			input := strings.TrimSuffix(deviceJSON, "}") + testCase[1] + "}"

			var out, errOut bytes.Buffer

			err := run(context.Background(), []string{testCase[0], fixtureInputFlag, "-"}, strings.NewReader(input), &out, &errOut, noEnvironment, client)
			if testCase[2] != "" {
				if err != nil {
					t.Fatal(err)
				}

				if !strings.Contains(out.String(), testCase[2]) {
					t.Fatal("missing capabilities")
				}
			} else if err == nil || out.Len() != 0 {
				t.Fatal("invalid or unsupported request succeeded")
			}

			replayErr := <-done
			if replayErr != nil {
				t.Fatal(replayErr)
			}
		})
	}
}

func TestMapListAndSelectionPairedMQTT(t *testing.T) {
	t.Parallel()

	cases := [][5]string{
		{commandRooms, "get_room_mapping", `[]`, `[[16,"1001"]]`, `"segmentId":16`},
		{commandMaps, "get_multi_maps_list", `[]`, `[{"map_info":[{"mapFlag":3,"name":"Upstairs"}]}]`, `"id":"3"`},
		{commandSelectMap, "load_multi_map", `[3]`, fixtureRPCResult, fixtureAcknowledged},
	}
	for _, testCase := range cases {
		t.Run(testCase[0], func(t *testing.T) {
			t.Parallel()
			client, done := rpcTestClient(t, func(conn net.Conn) error { return replyRPC(conn, testCase[1], testCase[2], testCase[3], nil) })
			input := strings.TrimSuffix(deviceJSON, "}") + `,"mapId":"3"}`

			var out, errOut bytes.Buffer

			err := run(context.Background(), []string{testCase[0], fixtureInputFlag, "-"}, strings.NewReader(input), &out, &errOut, noEnvironment, client)
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(out.String(), testCase[4]) {
				t.Fatal("missing typed map result")
			}

			replayErr := <-done
			if replayErr != nil {
				t.Fatal(replayErr)
			}
		})
	}
}

func TestInvalidZonesNeverOpenDevice(t *testing.T) {
	t.Parallel()

	cases := []string{
		`{"zones":[{"x2":300,"y2":400,"repeats":1}]}`,
		`{"zones":[{"x1":null,"y1":0,"x2":300,"y2":400,"repeats":1}]}`,
		`{"zones":[{"x1":0,"y1":null,"x2":300,"y2":400,"repeats":1}]}`,
		`{"zones":[{"x1":300,"y1":200,"x2":100,"y2":400,"repeats":1}]}`,
		`{"zones":[]}`,
		`null`,
	}
	for _, zones := range cases {
		client, err := roborock.NewClient(roborock.WithMQTTDial(func(_ context.Context, _, _ string) (net.Conn, error) {
			t.Error("invalid zones opened MQTT connection")

			return nil, errors.New("unexpected dial")
		}))
		if err != nil {
			t.Fatal(err)
		}

		input := strings.TrimSuffix(deviceJSON, "}") + `,"cleanZones":` + zones + "}"

		var out, errOut bytes.Buffer

		err = run(context.Background(), []string{commandCleanZones}, strings.NewReader(input), &out, &errOut, noEnvironment, client)
		if !errors.Is(err, errCleaningZones) || out.Len() != 0 {
			t.Fatal("invalid zones accepted")
		}
	}
}

func TestCurrentMapPairedMQTT(t *testing.T) {
	t.Parallel()
	client, done := rpcTestClient(t, replySyntheticMap)

	var out, errOut bytes.Buffer

	err := run(context.Background(), []string{commandMap}, strings.NewReader(deviceJSON), &out, &errOut, noEnvironment, client)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), `"point":{"x":1000,"y":2000}`) {
		t.Fatal("map geometry missing")
	}

	for _, secret := range []string{fixtureSecret, fixtureLocalKey, "nonce", "goOmJ7S+"} {
		if strings.Contains(out.String(), secret) {
			t.Fatal("map output contains credentials")
		}
	}

	replayErr := <-done
	if replayErr != nil {
		t.Fatal(replayErr)
	}
}

func replySyntheticMap(conn net.Conn) error {
	header, body, err := readMQTTPacket(conn)
	if err != nil {
		return err
	}

	topic := "rr/m/i/synthetic-user/f8cd4bde/synthetic-device"

	prefix := append([]byte{0, byte(len(topic) & 255)}, []byte(topic)...)
	if header != 48 || !bytes.HasPrefix(body, prefix) {
		return errors.New("map topic mismatch")
	}

	frame := body[len(prefix):]

	plain, err := testFrame(frame, nil)
	if err != nil {
		return err
	}

	identifier, err := matchRPCPlain(plain, "get_map_v1", `[]`)
	if err != nil {
		return err
	}

	nonce, err := mapNonce(plain)
	if err != nil {
		return err
	}

	encrypted, err := syntheticMapCiphertext(nonce)
	if err != nil {
		return err
	}

	if identifier < 1 || identifier > 65535 {
		return errors.New("map request identifier out of range")
	}

	payload := make([]byte, 24+len(encrypted))
	copy(payload[24:], encrypted)
	copy(payload, "goOmJ7S+")
	binary.LittleEndian.PutUint16(payload[16:18], uint16(identifier))

	encoded, err := testFrame(frame, payload)
	if err != nil {
		return err
	}

	binary.BigEndian.PutUint16(encoded[15:17], 301)
	binary.BigEndian.PutUint32(encoded[len(encoded)-4:], crc32.ChecksumIEEE(encoded[:len(encoded)-4]))

	topic = fixtureOutputTopic
	_, err = conn.Write(mqttTestPacket(48, append(append([]byte{0, byte(len(topic) & 255)}, []byte(topic)...), encoded...)))

	return err
}

func mapNonce(plain []byte) ([]byte, error) {
	var envelope map[string]json.RawMessage

	err := json.Unmarshal(plain, &envelope)
	if err != nil {
		return nil, err
	}

	var dps map[string]string

	err = json.Unmarshal(envelope["dps"], &dps)
	if err != nil {
		return nil, err
	}

	var request map[string]json.RawMessage

	err = json.Unmarshal([]byte(dps["101"]), &request)
	if err != nil {
		return nil, err
	}

	var security map[string]string

	err = json.Unmarshal(request["security"], &security)
	if err != nil {
		return nil, err
	}

	return hex.DecodeString(security["nonce"])
}

func syntheticMapCiphertext(nonce []byte) ([]byte, error) {
	// Synthetic rr map: header followed by robot pose block, coordinates in millimeters.
	raw := make([]byte, 61)
	copy(raw, "rr")
	binary.LittleEndian.PutUint16(raw[2:4], 20)
	binary.LittleEndian.PutUint32(raw[12:16], 3)
	binary.LittleEndian.PutUint16(raw[20:22], 8)
	binary.LittleEndian.PutUint16(raw[22:24], 8)
	binary.LittleEndian.PutUint32(raw[24:28], 8)
	binary.LittleEndian.PutUint32(raw[28:32], 1000)
	binary.LittleEndian.PutUint32(raw[32:36], 2000)
	// A one-cell image block follows the pose.
	binary.LittleEndian.PutUint16(raw[36:38], 2)
	binary.LittleEndian.PutUint16(raw[38:40], 24)
	binary.LittleEndian.PutUint32(raw[40:44], 1)
	binary.LittleEndian.PutUint32(raw[52:56], 1)
	binary.LittleEndian.PutUint32(raw[56:60], 1)
	raw[60] = 1

	var compressed bytes.Buffer

	writer := gzip.NewWriter(&compressed)

	_, err := writer.Write(raw)
	if err != nil {
		return nil, err
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	payload := compressed.Bytes()
	padding := aes.BlockSize - len(payload)%aes.BlockSize
	payload = append(payload, bytes.Repeat([]byte{byte(padding)}, padding)...)

	block, err := aes.NewCipher(nonce)
	if err != nil {
		return nil, err
	}

	encrypted := make([]byte, len(payload))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(encrypted, payload) //nolint:gosec // GO-05/LIB-05: Pinned V1 map protocol requires a zero IV; paired synthetic oracle.

	return encrypted, nil
}
