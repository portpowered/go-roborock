package roborock

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/portpowered/go-roborock/pkg/dependencies/mapdata"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"
)

const (
	fixtureVacuumCategory    = "robot.vacuum.cleaner"
	fixtureOtherModel        = "other"
	fixtureNullResponse      = "null"
	fixtureRoomMappingMethod = "get_room_mapping"
)

type familyCase struct {
	version         ProtocolVersion
	model, category string
	want            DeviceFamily
}

func TestDeviceFamilies(t *testing.T) {
	t.Parallel()

	cases := []familyCase{
		{ProtocolV1, "roborock.vacuum.a15", fixtureVacuumCategory, FamilyV1Vacuum},
		{ProtocolV1, fixtureOtherModel, "roborock.wm", FamilyUnknown},
		{ProtocolA01, fixtureOtherModel, "roborock.wetdryvac", FamilyDyad},
		{ProtocolA01, fixtureOtherModel, "roborock.wm", FamilyZeo},
		{ProtocolB01, "roborock.vacuum.ss07", fixtureVacuumCategory, FamilyB01Q10},
		{ProtocolB01, "roborock.vacuum.sc05", fixtureVacuumCategory, FamilyB01Q7},
		{ProtocolB01, "roborock.vacuum.sc_ss", fixtureVacuumCategory, FamilyB01Q10},
		{ProtocolB01, "roborock.vacuum.future", fixtureVacuumCategory, FamilyUnknown},
		{ProtocolL01, "roborock.vacuum.sc05", fixtureVacuumCategory, FamilyUnknown},
	}
	for _, test := range cases {
		if got := productFamily(test.version, test.model, test.category); got != test.want {
			t.Fatalf("family %s/%s: got %s want %s", test.version, test.model, got, test.want)
		}
	}
}

type pairedMapRPC struct{ *pairedRPC }

func (p *pairedMapRPC) CallB01(
	ctx context.Context, method dependencymodels.B01Method, parameters json.RawMessage,
) (json.RawMessage, error) {
	return p.Call(ctx, string(method), parameters)
}
func (p *pairedMapRPC) FetchMapV1(context.Context) ([]byte, error) { return nil, errResultShape }
func (p *pairedMapRPC) FetchMapQ7(context.Context, int64, string, string) ([]byte, error) {
	return nil, errResultShape
}
func (p *pairedMapRPC) QueryMapListQ10(context.Context) (dependencymodels.MapsQ10ListResult, error) {
	return dependencymodels.MapsQ10ListResult{}, errResultShape
}
func (p *pairedMapRPC) FetchMapQ10(context.Context) ([]byte, error)   { return nil, errResultShape }
func (p *pairedMapRPC) FetchTraceQ10(context.Context) ([]byte, error) { return nil, errResultShape }
func (p *pairedMapRPC) SetQ10Clean(ctx context.Context, command dependencymodels.Q10CleanCommand) error {
	parameters, err := json.Marshal(command)
	if err != nil {
		return operationError("synthetic", err)
	}

	_, err = p.Call(ctx, "synthetic-q10-clean", parameters)

	return err
}

func familySession(t *testing.T, family DeviceFamily, exchanges ...rpcExchange) *DeviceSession {
	t.Helper()
	session := operationSession(t, exchanges...)
	session.family = family

	rpc, ok := session.rpc.(*pairedRPC)
	if !ok {
		t.Fatal("unexpected fixture transport")
	}

	session.rpc = &pairedMapRPC{pairedRPC: rpc}

	return session
}

func TestFamilyRoomCleaning(t *testing.T) {
	t.Parallel()
	q7 := familySession(t, FamilyB01Q7, rpcExchange{method: "service.set_room_clean",
		params: `{"clean_type":1,"ctrl_value":1,"room_ids":[16,17]}`, response: `{"accepted":true}`, failure: nil})

	ack, err := q7.CleanSegments(t.Context(), CleanSegmentsRequest{Segments: []int64{16, 17}, Repeats: 1})
	if err != nil || !ack.Sent || !ack.Acknowledged {
		t.Fatalf("Q7 acknowledgement: %+v %v", ack, err)
	}

	//nolint:misspell // Firmware requires this exact JSON field spelling.
	q10 := familySession(t, FamilyB01Q10, rpcExchange{method: "synthetic-q10-clean",
		params: `{"clean_paramters":[9],"cmd":2}`, response: fixtureNullResponse, failure: nil})

	ack, err = q10.CleanSegments(t.Context(), CleanSegmentsRequest{Segments: []int64{9}, Repeats: 1})
	if err != nil || !ack.Sent || ack.Acknowledged {
		t.Fatalf("Q10 publication: %+v %v", ack, err)
	}

	for _, family := range []DeviceFamily{FamilyB01Q7, FamilyB01Q10} {
		session := familySession(t, family)

		_, err = session.CleanSegments(t.Context(), CleanSegmentsRequest{Segments: []int64{9}, Repeats: 2})
		if err == nil {
			t.Fatalf("%s silently accepted repeat", family)
		}
	}
}

func TestQ7MapSelectionRequiresCurrent(t *testing.T) {
	t.Parallel()

	responses := []string{`{"map_list":[{"id":1}]}`, `{"map_list":[{"id":1,"cur":true},{"id":2,"cur":true}]}`}
	for _, response := range responses {
		session := familySession(t, FamilyB01Q7, rpcExchange{method: "service.get_map_list",
			params: `{}`, response: response, failure: nil})

		_, err := session.GetMap(t.Context(), GetMapRequest{MapID: ""})
		if err == nil {
			t.Fatal("map read guessed active map")
		}
	}
}

func TestV1MapMetadataAndRoomMappings(t *testing.T) {
	t.Parallel()
	session := operationSession(t,
		rpcExchange{method: "get_multi_maps_list", params: `[]`,
			response: `[{"map_info":[{"mapFlag":0,"name":"Ground"}]}]`, failure: nil},
		rpcExchange{method: fixtureRoomMappingMethod, params: `[]`, response: `[[16,1001],[17,"1002"]]`, failure: nil},
		rpcExchange{method: "load_multi_map", params: `[0]`, response: `["ok"]`, failure: nil},
	)

	maps, err := session.ListMaps(t.Context(), EmptyRequest{})
	if err != nil {
		t.Fatal(err)
	}

	name := "Ground"

	wantMaps := ListMapsResult{Maps: []MapInfo{{ID: "0", Name: &name, Current: nil}}}

	if !reflect.DeepEqual(maps, wantMaps) {
		t.Fatalf("maps: %+v %v", maps, err)
	}

	rooms, err := session.GetRooms(t.Context(), EmptyRequest{})
	if err != nil {
		t.Fatal(err)
	}

	first, second := "1001", "1002"

	wantRooms := MapRoomsResult{Rooms: []MapRoomMapping{
		{SegmentID: 16, IoTID: &first, Name: nil}, {SegmentID: 17, IoTID: &second, Name: nil}}}

	if !reflect.DeepEqual(rooms, wantRooms) {
		t.Fatalf("rooms: %+v %v", rooms, err)
	}

	ack, err := session.SelectMap(t.Context(), SelectMapRequest{MapID: "0"})
	if err != nil || !ack.Acknowledged || !ack.Sent {
		t.Fatalf("select map: %+v %v", ack, err)
	}
}

type waitingRoomHTTP struct{ started chan struct{} }

func TestMapErrorsPreserveClassification(t *testing.T) {
	t.Parallel()

	_, err := decodeMap(MapFormatV1, nil, "GetMap")
	assertMapFailure(t, err, roborockerrors.Protocol)

	var (
		grid      MapGrid
		point     MapPoint
		rectangle MapRectangle
	)

	_, err = MapToPixel(grid, point)
	assertMapFailure(t, err, roborockerrors.InvalidArgument)
	_, err = PixelToMap(grid, point)
	assertMapFailure(t, err, roborockerrors.InvalidArgument)
	_, err = PixelRectangleToMap(grid, rectangle)
	assertMapFailure(t, err, roborockerrors.InvalidArgument)
}

func assertMapFailure(t *testing.T, err error, kind roborockerrors.Kind) {
	t.Helper()

	var typed *roborockerrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("untyped error: %v", err)
	}

	if typed.Kind != kind {
		t.Fatalf("error kind %s want %s", typed.Kind, kind)
	}

	if !errors.Is(err, mapdata.ErrMalformed) {
		t.Fatalf("missing malformed-map cause: %v", err)
	}
}

func (w waitingRoomHTTP) Do(request *http.Request) (*http.Response, error) {
	close(w.started)
	<-request.Context().Done()

	return nil, operationError("synthetic", request.Context().Err())
}

func TestRoomLookupEndsWithSession(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})

	client, err := NewClient(WithHTTPClient(waitingRoomHTTP{started: started}))
	if err != nil {
		t.Fatal(err)
	}

	session := operationSession(t, rpcExchange{method: fixtureRoomMappingMethod, params: `[]`,
		response: `[16,"1001"]`, failure: nil})
	session.client = client
	session.auth.Token = "synthetic"
	session.auth.ClientID = "synthetic"
	session.auth.BaseURL = "https://example.test"
	session.life, session.cancel = context.WithCancel(t.Context())
	result := make(chan error, 1)

	go func() { _, lookupErr := session.GetRooms(t.Context(), EmptyRequest{}); result <- lookupErr }()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("room lookup did not reach HTTP")
	}

	err = session.Close()
	if err != nil {
		t.Fatal(err)
	}

	select {
	case err = <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("room cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("room lookup survived session close")
	}

	_, err = session.GetCapabilities(t.Context(), EmptyRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("capabilities after close: %v", err)
	}
}

const (
	fixtureAcknowledgement   = `["ok"]`
	fixtureStopPreview       = "stop_camera_preview"
	fixtureSDPOffer          = "v=0 offer"
	fixtureICECandidate      = "candidate:1"
	fixtureVendorFailureCode = 142
	fixtureRecordParameters  = "[123]"
	fixtureRecordMethod      = "get_clean_record"
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

	payloads := []string{
		`{"clean_time":3600,"clean_area":1000000,"clean_count":1,"records":[123]}`,
		`[3600,1000000,1,[123]]`,
	}
	for _, payload := range payloads {
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

	payloads := []string{
		`{"begin":1,"end":2,"duration":3,"area":4}`,
		`[{"begin":1,"end":2,"duration":3,"area":4}]`,
		`[1,2,3,4]`,
	}
	for _, payload := range payloads {
		t.Run(payload, func(t *testing.T) {
			t.Parallel()
			session := operationSession(
				t,
				rpcExchange{method: fixtureRecordMethod, params: fixtureRecordParameters, response: payload, failure: nil},
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

type terminalLifecycleRPC struct {
	deviceRPC

	done    chan struct{}
	failure error
}

func (r *terminalLifecycleRPC) Done() <-chan struct{} { return r.done }
func (r *terminalLifecycleRPC) Err() error {
	select {
	case <-r.done:
		return r.failure
	default:
		return nil
	}
}

func TestDeviceObservesTerminalTransportFailure(t *testing.T) {
	t.Parallel()
	session := operationSession(t)

	cause := errResultShape
	failure := roborockerrors.New(roborockerrors.Unavailable, "synthetic_connection", "connection failed", cause)
	failure.Code = fixtureVendorFailureCode

	transport := &terminalLifecycleRPC{deviceRPC: session.rpc, done: make(chan struct{}), failure: failure}
	session.rpc = transport

	if session.Err() != nil {
		t.Fatal("active session reported failure")
	}

	go session.observeTransport(transport)

	close(transport.done)

	wait, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	select {
	case <-session.Done():
	case <-wait.Done():
		t.Fatal("device observer did not terminate")
	}

	terminal := session.Err()

	var typed *roborockerrors.Error
	if !errors.Is(terminal, cause) || !errors.As(terminal, &typed) || typed.Code != fixtureVendorFailureCode ||
		typed.Kind != roborockerrors.Unavailable {
		t.Fatalf("terminal failure lost: %v", terminal)
	}
}

type lifecycleEndCase struct {
	name     string
	manual   bool
	deadline bool
	kind     roborockerrors.Kind
	cause    error
}

func TestDeviceLifecycleTerminationClasses(t *testing.T) {
	t.Parallel()

	cases := []lifecycleEndCase{
		{name: "manual", manual: true, deadline: false, kind: roborockerrors.Closed, cause: nil},
		{
			name:     "canceled",
			manual:   false,
			deadline: false,
			kind:     roborockerrors.Canceled,
			cause:    context.Canceled,
		},
		{
			name:     "deadline",
			manual:   false,
			deadline: true,
			kind:     roborockerrors.Timeout,
			cause:    context.DeadlineExceeded,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runTerminationCase(t, testCase)
		})
	}
}

func runTerminationCase(t *testing.T, testCase lifecycleEndCase) {
	t.Helper()
	session := operationSession(t)

	life, cancel := context.WithCancel(t.Context())
	if testCase.deadline {
		cancel()

		life, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	}

	t.Cleanup(cancel)

	session.life, session.cancel = life, cancel
	transport := &terminalLifecycleRPC{deviceRPC: session.rpc, done: make(chan struct{}), failure: nil}
	session.rpc = transport
	observed := make(chan struct{})

	go func() { session.observeTransport(transport); close(observed) }()

	if testCase.manual {
		err := session.Close()
		if err != nil {
			t.Fatal(err)
		}
	} else if !testCase.deadline {
		cancel()
	}

	wait, cancelWait := context.WithTimeout(t.Context(), time.Second)
	defer cancelWait()

	select {
	case <-observed:
	case <-wait.Done():
		t.Fatal("termination observer did not exit")
	}

	assertTermination(t, session, testCase)
}

func assertTermination(t *testing.T, session *DeviceSession, testCase lifecycleEndCase) {
	t.Helper()

	select {
	case <-session.Done():
	default:
		t.Fatal("device Done did not close")
	}

	terminal := session.Err()
	if !errors.Is(terminal, roborockerrors.New(testCase.kind, "test", "", nil)) {
		t.Fatalf("terminal class %v", terminal)
	}

	if testCase.cause != nil && !errors.Is(terminal, testCase.cause) {
		t.Fatalf("terminal cause %v", terminal)
	}
}

func TestCleanRecordMultipartDepth(t *testing.T) {
	t.Parallel()

	session := operationSession(t,
		rpcExchange{
			method: fixtureRecordMethod, params: fixtureRecordParameters,
			response: `[[1,2,3,4],[5,6,7,8]]`, failure: nil,
		},
		rpcExchange{method: fixtureRecordMethod, params: fixtureRecordParameters, response: `[[[1,2,3,4]]]`, failure: nil},
		rpcExchange{method: fixtureRecordMethod, params: fixtureRecordParameters, response: `[null,2,3,4]`, failure: nil},
	)

	result, err := session.GetCleanRecord(t.Context(), CleanRecordRequest{RecordId: 123})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Records) != 2 {
		t.Fatalf("expected two record parts: %v", result)
	}

	_, err = session.GetCleanRecord(t.Context(), CleanRecordRequest{RecordId: 123})
	if !errors.Is(err, roborockerrors.New(roborockerrors.Protocol, "test", "", nil)) {
		t.Fatalf("arbitrary nesting accepted: %v", err)
	}

	_, err = session.GetCleanRecord(t.Context(), CleanRecordRequest{RecordId: 123})
	if !errors.Is(err, roborockerrors.New(roborockerrors.Protocol, "test", "", nil)) {
		t.Fatalf("null tuple counter accepted: %v", err)
	}
}

func TestGetRoomsAcceptsRoomTypeAndNullMapping(t *testing.T) {
	t.Parallel()

	// Reference sample (python-roborock tests/devices/traits/v1/test_rooms.py): a third room-type column.
	session := operationSession(t,
		rpcExchange{method: fixtureRoomMappingMethod, params: `[]`,
			response: `[[16,"2362048",6],[17,"2362044"]]`, failure: nil},
		rpcExchange{method: fixtureRoomMappingMethod, params: `[]`, response: fixtureNullResponse, failure: nil},
	)

	rooms, err := session.GetRooms(t.Context(), EmptyRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if len(rooms.Rooms) != 2 || rooms.Rooms[0].SegmentID != 16 || rooms.Rooms[1].IoTID == nil ||
		*rooms.Rooms[1].IoTID != "2362044" {
		t.Fatalf("room-type tuples: %+v", rooms)
	}

	rooms, err = session.GetRooms(t.Context(), EmptyRequest{})
	if err != nil || len(rooms.Rooms) != 0 {
		t.Fatalf("null mapping: %+v %v", rooms, err)
	}
}

func TestGetRoomsRejectsMalformedPairsWithoutPartialResult(t *testing.T) {
	t.Parallel()

	responses := []string{`[[9]]`, `[[8,"valid"],[9]]`, `[[null,"iot"]]`, `[["nine","iot"]]`}
	for _, response := range responses {
		t.Run(response, func(t *testing.T) {
			t.Parallel()

			session := operationSession(t, rpcExchange{
				method: fixtureRoomMappingMethod, params: `[]`, response: response, failure: nil,
			})

			result, err := session.GetRooms(t.Context(), EmptyRequest{})
			if !errors.Is(err, roborockerrors.New(roborockerrors.Protocol, "GetRooms", "", nil)) {
				t.Fatalf("malformed pair error class: %v", err)
			}

			var expected MapRoomsResult

			if !reflect.DeepEqual(result, expected) {
				t.Fatalf("malformed pair exposed partial result: %+v", result)
			}
		})
	}
}

func TestSetCleaningModeSendsAllMotorSettings(t *testing.T) {
	t.Parallel()

	// Reference payload: python-roborock get_cleaning_mode_parameters(CleaningMode.VACUUM).
	session := operationSession(t,
		rpcExchange{method: "set_clean_motor_mode",
			params: `[{"fan_power":102,"mop_mode":300,"water_box_mode":200}]`, response: fixtureAcknowledgement, failure: nil},
		rpcExchange{method: "set_clean_motor_mode",
			params: `[{"fan_power":102,"water_box_mode":200}]`, response: fixtureAcknowledgement, failure: nil},
	)

	route := MopModeStandard

	ack, err := session.SetCleaningMode(t.Context(),
		SetCleaningModeRequest{FanSpeed: FanSpeedBalanced, WaterMode: WaterModeOff, MopMode: &route})
	if err != nil || !ack.Acknowledged {
		t.Fatalf("vacuum only with route: %+v %v", ack, err)
	}

	ack, err = session.SetCleaningMode(t.Context(),
		SetCleaningModeRequest{FanSpeed: FanSpeedBalanced, WaterMode: WaterModeOff, MopMode: nil})
	if err != nil || !ack.Acknowledged {
		t.Fatalf("vacuum only without route: %+v %v", ack, err)
	}
}
