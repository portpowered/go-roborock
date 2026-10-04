package replay_test

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" //nolint:gosec // Independent B01 replay oracle checks the mandated IV derivation.
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/portpowered/go-roborock/pkg/dependencies/mqtt"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const (
	mqttReplayAddress     = "mqtt.example.test:8883"
	mqttReferenceRevision = "a8260c5211e60647fc352d6b496b827865938b21"
	mqttReplayNetwork     = "tcp"
)

type mapReplayStep struct {
	Operation        string          `json:"operation"`
	Request          json.RawMessage `json:"request"`
	Protocol         int             `json:"protocol"`
	ResponseFrameHex string          `json:"responseFrameHex"`
	Result           json.RawMessage `json:"result"`
	ResultBase64     string          `json:"resultBase64"`
	ResultErrorKind  string          `json:"resultErrorKind"`
}

type mapReplayCase struct {
	Name  string          `json:"name"`
	Steps []mapReplayStep `json:"steps"`
}

type mapReplayFixture struct {
	Provenance string          `json:"provenance"`
	Source     string          `json:"source"`
	Revision   string          `json:"revision"`
	Serial     string          `json:"serial"`
	Model      string          `json:"model"`
	Cases      []mapReplayCase `json:"cases"`
}

func TestPairedMQTTMapsB01(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("fixtures/mqtt/synthetic/maps.json")
	if err != nil {
		t.Fatal(err)
	}

	var fixture mapReplayFixture

	err = json.Unmarshal(data, &fixture)
	if err != nil {
		t.Fatal(err)
	}

	if fixture.Provenance != accountFixtureProvenance || fixture.Revision != mqttReferenceRevision ||
		fixture.Source == "" {
		t.Fatal("fixture provenance missing")
	}

	for _, replay := range fixture.Cases {
		t.Run(replay.Name, func(t *testing.T) { t.Parallel(); runMapReplay(t, fixture, replay) })
	}
}

func runMapReplay(t *testing.T, fixture mapReplayFixture, replay mapReplayCase) {
	t.Helper()
	account := loadMQTTFixture(t)
	client, broker := net.Pipe()

	t.Cleanup(func() { _ = client.Close(); _ = broker.Close() })

	deadline := time.Now().Add(5 * time.Second)

	err := broker.SetDeadline(deadline)
	if err != nil {
		t.Fatal(err)
	}

	completed := make(chan error, 1)

	go func() { completed <- replayMapTranscript(broker, account, replay); _ = broker.Close() }()

	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	session, err := mqtt.Open(ctx, mqtt.Config{
		BrokerURL: account.Broker, User: account.User, Secret: account.Secret, Key: account.Key,
		DeviceID: account.DeviceID, LocalKey: account.LocalKey, Protocol: "B01",
	}, func(_ context.Context, network, address string) (net.Conn, error) {
		if network != mqttReplayNetwork || address != mqttReplayAddress {
			return nil, mqttMismatch("unexpected map broker dial")
		}

		return client, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = session.Close() })

	executeMapReplaySteps(ctx, t, session, fixture, replay)

	err = session.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = <-completed
	if err != nil {
		t.Fatal(err)
	}
}

func executeMapReplaySteps(
	ctx context.Context, t *testing.T, session *mqtt.Session, fixture mapReplayFixture, replay mapReplayCase,
) {
	t.Helper()

	for _, step := range replay.Steps {
		result, err := executeMapReplay(ctx, session, fixture, replay.Name, step.Operation)
		if step.ResultErrorKind != "" {
			assertMapReplayError(t, err, step.ResultErrorKind)

			continue
		}

		if err != nil {
			t.Fatalf("%s: %v", step.Operation, err)
		}

		assertMapReplayResult(t, step, result)
	}
}

func assertMapReplayError(t *testing.T, err error, kind string) {
	t.Helper()

	var typed *roborockerrors.Error
	if !errors.As(err, &typed) || string(typed.Kind) != kind {
		t.Fatalf("error=%v expected kind=%s", err, kind)
	}

	if typed.Cause == nil {
		t.Fatalf("missing rejection cause: %v", err)
	}
}

func executeMapReplay(
	ctx context.Context, session *mqtt.Session, fixture mapReplayFixture, family, operation string,
) ([]byte, error) {
	switch operation {
	case "list":
		return executeMapListReplay(ctx, session, family)
	case "map":
		if family == "q7" {
			return wrapMapResult(session.FetchMapQ7(ctx, 7, fixture.Serial, fixture.Model))
		}

		return wrapMapResult(session.FetchMapQ10(ctx))
	case "trace":
		return wrapMapResult(session.FetchTraceQ10(ctx))
	case "rooms":
		params := json.RawMessage(`{"clean_type":1,"ctrl_value":1,"room_ids":[9]}`)

		return wrapMapResult(session.CallB01(ctx, dependencymodels.ServiceSetRoomClean, params))
	case "zone":
		return executeZoneReplay(ctx, session)
	default:
		return nil, mqttMismatch("unknown replay operation")
	}
}

func executeMapListReplay(ctx context.Context, session *mqtt.Session, family string) ([]byte, error) {
	if family == "q7" {
		return wrapMapResult(session.CallB01(ctx, dependencymodels.ServiceGetMapList, json.RawMessage(`{}`)))
	}

	result, err := session.QueryMapListQ10(ctx)
	if err != nil {
		return nil, fmt.Errorf("query Q10 maps: %w", err)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal map list: %w", err)
	}

	return encoded, nil
}

func wrapMapResult(result []byte, err error) ([]byte, error) {
	if err != nil {
		return nil, fmt.Errorf("map replay operation: %w", err)
	}

	return result, nil
}

func executeZoneReplay(ctx context.Context, session *mqtt.Session) ([]byte, error) {
	zone, err := mqtt.EncodeQ10Zone(mqtt.Q10Zone{X1: 25500, Y1: 25500, X2: 25600, Y2: 25700, Repeats: 2})
	if err != nil {
		return nil, fmt.Errorf("encode zone: %w", err)
	}

	parameters, err := json.Marshal(zone)
	if err != nil {
		return nil, fmt.Errorf("marshal zone: %w", err)
	}

	err = session.SetQ10Clean(ctx, dependencymodels.Q10CleanCommand{Cmd: dependencymodels.N3, CleanParamters: parameters})

	return wrapMapResult(nil, err)
}

func assertMapReplayResult(t *testing.T, step mapReplayStep, result []byte) {
	t.Helper()

	if step.ResultBase64 != "" {
		want, err := base64.StdEncoding.DecodeString(step.ResultBase64)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(result, want) {
			t.Fatalf("%s result=%x expected=%x", step.Operation, result, want)
		}
	} else if step.Result != nil && !equalMQTTJSON(result, step.Result) {
		t.Fatalf("%s JSON result=%s", step.Operation, result)
	}
}

func replayMapTranscript(conn net.Conn, account mqttFixture, replay mapReplayCase) error {
	err := replayMQTTHandshake(conn, account)
	if err != nil {
		return err
	}

	for index, step := range replay.Steps {
		sequence := uint32(index + 1)
		if replay.Name == "q7" {
			sequence *= 2
		}

		err = replayMapStep(conn, account, replay.Name, step, sequence)
		if err != nil {
			return err
		}
	}

	var extra [1]byte

	_, err = conn.Read(extra[:])
	if !errors.Is(err, io.EOF) {
		return mqttMismatch("unexpected or duplicate map exchange: %x (%v)", extra, err)
	}

	return nil
}

func replayMapStep(conn net.Conn, account mqttFixture, family string, step mapReplayStep, sequence uint32) error {
	body, err := readMQTTExchange(conn, 0x30)
	if err != nil {
		return err
	}

	topic, frame, err := takeMQTTString(body)
	if err != nil {
		return err
	}

	if topic != account.PublishTopic {
		return mqttMismatch("map request topic mismatch")
	}

	payload, err := decryptB01Replay(frame, account.LocalKey, family == "q7", sequence)
	if err != nil {
		return err
	}

	if !equalMQTTJSON(payload, step.Request) {
		return mqttMismatch("%s %s request mismatch: %s", family, step.Operation, payload)
	}

	if step.ResponseFrameHex != "" {
		return sendMQTTA01Response(conn, account.SubscribeTopic, step.ResponseFrameHex)
	}

	return nil
}

func decryptB01Replay(frame []byte, key string, innerPadding bool, sequence uint32) ([]byte, error) {
	err := validateMQTTDeviceFrame(frame, "B01", sequence)
	if err != nil {
		return nil, err
	}

	random := binary.BigEndian.Uint32(frame[7:11])
	// B01 reference IV uses this exact digest.
	digest := md5.Sum([]byte(fmt.Sprintf("%08x", random) + "5wwh9ikChRjASpMU8cxg7o1d2E")) //nolint:gosec
	initializationVector := []byte(hex.EncodeToString(digest[:])[9:25])

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return nil, fmt.Errorf("B01 replay cipher: %w", err)
	}

	plain := make([]byte, len(frame)-23)
	cipher.NewCBCDecrypter(block, initializationVector).CryptBlocks(plain, frame[19:len(frame)-4])

	plain, err = unpadMQTTRequest(plain)
	if err != nil {
		return nil, err
	}

	if innerPadding {
		return unpadMQTTRequest(plain)
	}

	return plain, nil
}
