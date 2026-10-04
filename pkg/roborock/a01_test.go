package roborock

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

var errA01UnexpectedV1 = errors.New("unexpected V1 call")

// Tests use synthetic datapoints derived from pinned python-roborock contracts.
// They establish adapter behavior, not live-device verification.
type a01TestRPC struct {
	queryIDs []int
	values   map[int]json.RawMessage
	set      map[int]json.RawMessage
	err      error
}

func (r *a01TestRPC) Call(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, errA01UnexpectedV1
}

func (r *a01TestRPC) QueryA01(_ context.Context, ids []int) (map[int]json.RawMessage, error) {
	r.queryIDs = append([]int(nil), ids...)

	return r.values, r.err
}

func (r *a01TestRPC) SetA01(_ context.Context, values map[int]json.RawMessage) error {
	r.set = values

	return r.err
}

func (*a01TestRPC) Close() error { return nil }

func TestGetDyadStateTypedSelection(t *testing.T) {
	t.Parallel()

	rpc := &a01TestRPC{queryIDs: nil, set: nil, err: nil, values: map[int]json.RawMessage{
		201: json.RawMessage("901"), // Future status enum is preserved.
		206: json.RawMessage("4"),
		209: json.RawMessage("0"),
		229: json.RawMessage(`"3,4,5"`),
		230: json.RawMessage("999"), // Unselected value must not populate state.
		999: json.RawMessage(`{"future":true}`),
	}}

	session := newA01TestSession(rpc)

	state, err := session.GetDyadState(t.Context(), GetDyadStateRequest{Properties: []DyadProperty{
		DyadPropertyStatus, DyadPropertySuction, DyadPropertyPower, DyadPropertyRecentRunTime,
	}})
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(rpc.queryIDs, []int{201, 206, 209, 229}) {
		t.Fatalf("query inventory = %v", rpc.queryIDs)
	}

	assertA01Pointer(t, state.Status, DyadStatus(901))

	assertA01Pointer(t, state.Suction, DyadSuctionL4)

	assertA01Pointer(t, state.Power, 0)

	if state.TotalRunTime != nil {
		t.Fatalf("zero presence or selection lost: %+v", state)
	}

	if !reflect.DeepEqual(state.RecentRunTime, &[]int{3, 4, 5}) {
		t.Fatalf("durations = %v", state.RecentRunTime)
	}
}

func TestGetZeoStateTypedBooleans(t *testing.T) {
	t.Parallel()

	rpc := &a01TestRPC{queryIDs: nil, set: nil, err: nil, values: map[int]json.RawMessage{
		203: json.RawMessage("1"),
		204: json.RawMessage("2"),
		205: json.RawMessage("57"),
		223: json.RawMessage("false"),
		226: json.RawMessage("true"),
	}}

	session := newA01TestSession(rpc)

	state, err := session.GetZeoState(t.Context(), GetZeoStateRequest{Properties: []ZeoProperty{
		ZeoPropertyStatus, ZeoPropertyMode, ZeoPropertyProgram, ZeoPropertySound, ZeoPropertyDetergentEmpty,
	}})
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(rpc.queryIDs, []int{203, 204, 205, 223, 226}) {
		t.Fatalf("query inventory = %v", rpc.queryIDs)
	}

	assertA01Pointer(t, state.Status, ZeoStatusStandby)

	assertA01Pointer(t, state.Mode, ZeoModeWashAndDry)

	assertA01Pointer(t, state.Program, ZeoProgramSmallThingsDrying)

	assertA01Pointer(t, state.Sound, false)

	assertA01Pointer(t, state.DetergentEmpty, true)

	if state.SoftenerEmpty != nil {
		t.Fatalf("boolean presence = %+v", state)
	}
}

func TestA01QueryRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{`"charging"`, "true", "1.5", "null", " null ", "{}", "[]"} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			rpc := &a01TestRPC{queryIDs: nil, set: nil, err: nil, values: map[int]json.RawMessage{201: json.RawMessage(raw)}}

			session := newA01TestSession(rpc)

			_, err := session.GetDyadState(t.Context(), GetDyadStateRequest{Properties: []DyadProperty{DyadPropertyStatus}})

			if !errors.Is(err, roborockerrors.New(roborockerrors.Protocol, "", "", nil)) {
				t.Fatalf("value %s: expected protocol error, got %v", raw, err)
			}
		})
	}
}

func TestA01QueryRejectsInvalidSelections(t *testing.T) {
	t.Parallel()

	for name, properties := range map[string][]DyadProperty{
		"empty": nil, "duplicate": {DyadPropertyStatus, DyadPropertyStatus},
		"future": {DyadProperty(999)}, "write only": {DyadPropertyMeshReset},
		"unknown shape": {DyadPropertyFeatureInfo},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			rpc := new(a01TestRPC)

			session := newA01TestSession(rpc)

			_, err := session.GetDyadState(t.Context(), GetDyadStateRequest{Properties: properties})

			if !errors.Is(err, roborockerrors.New(roborockerrors.InvalidArgument, "", "", nil)) || rpc.queryIDs != nil {
				t.Fatalf("selection sent or wrong error: ids=%v err=%v", rpc.queryIDs, err)
			}
		})
	}
}

func TestA01SparseSettings(t *testing.T) {
	t.Parallel()

	rpc := new(a01TestRPC)

	session := newA01TestSession(rpc)

	suction, off := DyadSuctionL3, DyadSwitchOff

	var dyadRequest SetDyadSettingsRequest

	dyadRequest.Suction, dyadRequest.AutoDry = &suction, &off

	err := session.SetDyadSettings(t.Context(), dyadRequest)
	if err != nil {
		t.Fatal(err)
	}

	assertA01Values(t, rpc.set, map[int]json.RawMessage{206: json.RawMessage("3"), 213: json.RawMessage("0")})

	no, zero := false, 0

	var zeoRequest SetZeoSettingsRequest

	zeoRequest.ChildLock, zeoRequest.Countdown = &no, &zero

	err = session.SetZeoSettings(t.Context(), zeoRequest)
	if err != nil {
		t.Fatal(err)
	}

	assertA01Values(t, rpc.set, map[int]json.RawMessage{206: json.RawMessage("false"), 217: json.RawMessage("0")})
}

func TestStartZeoBundlesCycleParameters(t *testing.T) {
	t.Parallel()

	rpc := new(a01TestRPC)

	session := newA01TestSession(rpc)

	temperature, rinse := ZeoTemperatureMedium, ZeoRinseNone

	var request StartZeoRequest

	request.Mode, request.Program = ZeoModeWashAndDry, ZeoProgramStandard

	request.Temperature, request.Rinse = &temperature, &rinse

	err := session.StartZeo(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}

	assertA01Values(t, rpc.set, map[int]json.RawMessage{
		200: json.RawMessage("true"), 204: json.RawMessage("2"), 205: json.RawMessage("1"),
		207: json.RawMessage("3"), 208: json.RawMessage("0"),
	})
}

func TestA01OperationFailures(t *testing.T) {
	t.Parallel()

	rpc := new(a01TestRPC)

	session := newA01TestSession(rpc)

	var emptyRequest SetDyadSettingsRequest

	err := session.SetDyadSettings(t.Context(), emptyRequest)
	if !errors.Is(err, roborockerrors.New(roborockerrors.InvalidArgument, "", "", nil)) {
		t.Fatalf("empty write = %v", err)
	}

	var startRequest StartZeoRequest

	startRequest.Mode = ZeoModeWash

	err = session.StartZeo(t.Context(), startRequest)
	if !errors.Is(err, roborockerrors.New(roborockerrors.InvalidArgument, "", "", nil)) {
		t.Fatalf("missing program = %v", err)
	}

	session.protocol = string(ProtocolV1)

	startRequest.Program = ZeoProgramQuick

	err = session.StartZeo(t.Context(), startRequest)
	if !errors.Is(err, roborockerrors.New(roborockerrors.Unsupported, "", "", nil)) {
		t.Fatalf("wrong protocol = %v", err)
	}

	if rpc.set != nil {
		t.Fatal("invalid operation published data")
	}

	session.protocol = string(ProtocolA01)

	_, err = session.GetDyadState(t.Context(), GetDyadStateRequest{Properties: []DyadProperty{DyadPropertyStatus}})

	if !errors.Is(err, roborockerrors.New(roborockerrors.Protocol, "", "", nil)) {
		t.Fatalf("missing requested property = %v", err)
	}

	want := roborockerrors.New(roborockerrors.Timeout, "query", "timeout", context.DeadlineExceeded)

	rpc.err = want

	_, err = session.GetDyadState(t.Context(), GetDyadStateRequest{Properties: []DyadProperty{DyadPropertyStatus}})

	if !errors.Is(err, want) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("query failure not preserved: %v", err)
	}
}

func TestA01SettingBounds(t *testing.T) {
	t.Parallel()

	rpc := new(a01TestRPC)

	session := newA01TestSession(rpc)

	for _, value := range []int{-1, 1440} {
		var request SetZeoSettingsRequest

		request.SilentModeStartTime = &value

		err := session.SetZeoSettings(t.Context(), request)

		if !errors.Is(err, roborockerrors.New(roborockerrors.InvalidArgument, "", "", nil)) {
			t.Fatalf("minute of day %d accepted", value)
		}
	}

	for _, value := range []int{-1, 101} {
		var request SetDyadSettingsRequest

		request.Volume = &value

		err := session.SetDyadSettings(t.Context(), request)

		if !errors.Is(err, roborockerrors.New(roborockerrors.InvalidArgument, "", "", nil)) {
			t.Fatalf("volume %d accepted", value)
		}
	}

	if rpc.set != nil {
		t.Fatal("out-of-bounds setting sent")
	}

	lastMinute, maxVolume := 1439, 100

	var request SetDyadSettingsRequest

	request.Volume, request.SilentModeEndTime = &maxVolume, &lastMinute

	err := session.SetDyadSettings(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}

	assertA01Values(t, rpc.set, map[int]json.RawMessage{221: json.RawMessage("100"), 228: json.RawMessage("1439")})
}

func assertA01Values(t *testing.T, got, want map[int]json.RawMessage) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("published values = %v, want %v", got, want)
	}
}

type a01SchemaDocument struct {
	Components a01SchemaComponents `json:"components"`
}

type a01SchemaComponents struct {
	Schemas map[string]a01SchemaObject `json:"schemas"`
}

type a01SchemaObject struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}

func TestA01ProjectionSchemaContract(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../api/a01-models.openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}

	var document a01SchemaDocument

	err = json.Unmarshal(data, &document)
	if err != nil {
		t.Fatal(err)
	}

	for name, model := range map[string]reflect.Type{
		"DyadState": reflect.TypeFor[DyadState](), "ZeoState": reflect.TypeFor[ZeoState](),
		"GetDyadStateRequest":    reflect.TypeFor[GetDyadStateRequest](),
		"GetZeoStateRequest":     reflect.TypeFor[GetZeoStateRequest](),
		"SetDyadSettingsRequest": reflect.TypeFor[SetDyadSettingsRequest](),
		"SetZeoSettingsRequest":  reflect.TypeFor[SetZeoSettingsRequest](),
		"StartZeoRequest":        reflect.TypeFor[StartZeoRequest](),
	} {
		assertA01SchemaModel(t, name, model, document.Components.Schemas[name])
	}

	if len(a01Fields[DyadState, dependencymodels.DyadStateWire]()) != reflect.TypeFor[DyadState]().NumField() ||
		len(a01Fields[ZeoState, dependencymodels.ZeoStateWire]()) != reflect.TypeFor[ZeoState]().NumField() {
		t.Fatal("wire and public state inventories differ")
	}
}

func assertA01SchemaModel(t *testing.T, name string, model reflect.Type, schema a01SchemaObject) {
	t.Helper()

	if len(schema.Properties) != model.NumField() {
		t.Fatalf("%s: schema/generated field counts differ", name)
	}

	for i := range model.NumField() {
		field := model.Field(i)

		key := strings.Split(field.Tag.Get("json"), ",")[0]

		if _, exists := schema.Properties[key]; !exists {
			t.Fatalf("%s.%s absent from projection schema", name, field.Name)
		}

		for _, required := range schema.Required {
			if key == required && field.Type.Kind() == reflect.Pointer {
				t.Fatalf("%s.%s required field generated as optional", name, field.Name)
			}
		}
	}
}

func newA01TestSession(rpc *a01TestRPC) *DeviceSession {
	var session DeviceSession

	session.rpc, session.protocol = rpc, string(ProtocolA01)

	return &session
}

func assertA01Pointer[Value comparable](t *testing.T, got *Value, want Value) {
	t.Helper()

	if got == nil || *got != want {
		t.Fatalf("pointer value %v, want %v", got, want)
	}
}
