package roborock

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
	"reflect"
	"sync"
	"testing"
)

const (
	fixtureAcknowledgement = `["ok"]`
	fixtureStopPreview     = "stop_camera_preview"
	fixtureSDPOffer        = "v=0 offer"
	fixtureICECandidate    = "candidate:1"
)

type rpcExchange struct {
	method, params, response string
	failure                  error
}
type pairedRPC struct {
	t         *testing.T
	mu        sync.Mutex
	exchanges []rpcExchange
	next      int
}

func (p *pairedRPC) Call(_ context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.next >= len(p.exchanges) {
		p.t.Errorf("unexpected RPC %s", method)

		return nil, errResultShape
	}

	expected := p.exchanges[p.next]

	p.next++

	if method != expected.method || string(params) != expected.params {
		p.t.Errorf("RPC %d got %s %s want %s %s", p.next, method, params, expected.method, expected.params)

		return nil, errResultShape
	}

	return json.RawMessage(expected.response), expected.failure
}
func (p *pairedRPC) QueryA01(context.Context, []int) (map[int]json.RawMessage, error) {
	return nil, errResultShape
}
func (p *pairedRPC) SetA01(context.Context, map[int]json.RawMessage) error {
	return errResultShape
}
func (p *pairedRPC) Close() error { return nil }
func (p *pairedRPC) consumed() {
	p.t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.next != len(p.exchanges) {
		p.t.Errorf("consumed %d of %d", p.next, len(p.exchanges))
	}
}
func operationSession(t *testing.T, exchanges ...rpcExchange) *DeviceSession {
	t.Helper()

	rpc := new(pairedRPC)
	rpc.t = t
	rpc.exchanges = exchanges
	t.Cleanup(rpc.consumed)

	life, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	session := new(DeviceSession)
	session.rpc = rpc
	session.protocol = "1.0"
	session.life = life
	session.cancel = cancel

	return session
}

func TestStatusPresenceAndFutureValues(t *testing.T) {
	t.Parallel()
	session := operationSession(
		t,
		rpcExchange{
			method:   "get_status",
			params:   "[]",
			response: `[{"battery":0,"fan_power":999,"new_firmware_flag":7}]`,
			failure:  nil,
		},
	)

	result, err := session.GetStatus(context.Background(), EmptyRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if result.Battery == nil || *result.Battery != 0 || result.State != nil || result.FanPower == nil ||
		*result.FanPower != 999 {
		t.Fatalf("presence or open enum lost: %+v", result)
	}

	if string(result.AdditionalProperties["new_firmware_flag"]) != "7" {
		t.Fatal("unknown status field was lost")
	}
}

func TestVacuumCommandPairs(t *testing.T) {
	t.Parallel()
	session := operationSession(
		t,
		rpcExchange{method: "app_start", params: "[]", response: fixtureAcknowledgement, failure: nil},
		rpcExchange{method: "set_custom_mode", params: "[102]", response: fixtureAcknowledgement, failure: nil},
		rpcExchange{method: "app_zoned_clean", params: "[[1,2,3,4,2]]", response: fixtureAcknowledgement, failure: nil},
		rpcExchange{
			method:   "app_segment_clean",
			params:   `[{"repeat":2,"segments":[16,17]}]`,
			response: fixtureAcknowledgement,
			failure:  nil,
		},
		rpcExchange{
			method:   "app_rc_move",
			params:   `[{"duration":100,"omega":0,"seqnum":1,"velocity":0.1}]`,
			response: fixtureAcknowledgement,
			failure:  nil,
		},
	)

	ack, err := session.StartCleaning(context.Background(), EmptyRequest{})
	if err != nil || !ack.Acknowledged {
		t.Fatalf("ack %v %v", ack, err)
	}

	_, err = session.SetFanSpeed(context.Background(), SetFanSpeedRequest{Speed: 102})

	if err != nil {
		t.Fatal(err)
	}

	_, err = session.CleanZones(
		context.Background(),
		CleanZonesRequest{Zones: []Zone{{X1: 1, Y1: 2, X2: 3, Y2: 4, Repeats: 2}}},
	)

	if err != nil {
		t.Fatal(err)
	}

	_, err = session.CleanSegments(context.Background(), CleanSegmentsRequest{Segments: []int64{16, 17}, Repeats: 2})

	if err != nil {
		t.Fatal(err)
	}

	_, err = session.RCMove(context.Background(), RCMoveRequest{Velocity: 0.1, Omega: 0, Duration: 100, Sequence: 1})

	if err != nil {
		t.Fatal(err)
	}
}

func TestSummaryShapes(t *testing.T) {
	t.Parallel()

	for _, payload := range []string{`{"clean_time":3600,"clean_area":1000000,"clean_count":1,"records":[123]}`, `[3600,1000000,1,[123]]`} {
		t.Run(payload, func(t *testing.T) {
			t.Parallel()
			session := operationSession(
				t,
				rpcExchange{method: "get_clean_summary", params: "[]", response: payload, failure: nil},
			)

			result, err := session.GetCleaningSummary(context.Background(), EmptyRequest{})
			if err != nil {
				t.Fatal(err)
			}

			if result.CleanTime == nil || *result.CleanTime != 3600 || result.Records == nil ||
				!reflect.DeepEqual(*result.Records, []int64{123}) {
				t.Fatalf("summary: %+v", result)
			}
		})
	}
}

func TestRecordShapes(t *testing.T) {
	t.Parallel()

	for _, payload := range []string{`{"begin":1,"end":2,"duration":3,"area":4}`, `[{"begin":1,"end":2,"duration":3,"area":4}]`, `[1,2,3,4]`} {
		t.Run(payload, func(t *testing.T) {
			t.Parallel()
			session := operationSession(
				t,
				rpcExchange{method: "get_clean_record", params: "[123]", response: payload, failure: nil},
			)

			result, err := session.GetCleanRecord(context.Background(), CleanRecordRequest{RecordId: 123})
			if err != nil {
				t.Fatal(err)
			}

			if len(result.Records) != 1 || result.Records[0].Area == nil || *result.Records[0].Area != 4 {
				t.Fatalf("records: %+v", result)
			}
		})
	}
}

func TestOperationRejections(t *testing.T) {
	t.Parallel()

	for _, payload := range []string{`null`, `{}`, `[]`, `[{} ,{}]`, `{"battery":"bad"}`} {
		t.Run(payload, func(t *testing.T) {
			t.Parallel()
			session := operationSession(
				t,
				rpcExchange{method: "get_status", params: "[]", response: payload, failure: nil},
			)

			_, err := session.GetStatus(context.Background(), EmptyRequest{})
			if !errors.Is(err, roborockerrors.New(roborockerrors.Protocol, "test", "", nil)) {
				t.Fatalf("got %v", err)
			}
		})
	}

	session := operationSession(t)
	_, err := session.RCMove(context.Background(), RCMoveRequest{Velocity: 1, Omega: 0, Duration: 100, Sequence: 0})
	if !errors.Is(
		err,
		roborockerrors.New(roborockerrors.InvalidArgument, "test", "", nil),
	) {
		t.Fatalf("got %v", err)
	}
}

func TestCommandRejectsUncertainAcknowledgement(t *testing.T) {
	t.Parallel()
	session := operationSession(
		t,
		rpcExchange{method: "app_charge", params: "[]", response: `["failed"]`, failure: nil},
	)

	ack, err := session.ReturnToDock(context.Background(), EmptyRequest{})
	if ack.Acknowledged ||
		!errors.Is(err, roborockerrors.New(roborockerrors.Protocol, "test", "", nil)) {
		t.Fatalf("ack %v err %v", ack, err)
	}
}
