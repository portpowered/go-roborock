package roborock //nolint:testpackage // LIB-05: Tests inject the existing private paired RPC seam.

import (
	"errors"
	"testing"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

func TestListMapsRejectsMalformedKnownResponses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		family   DeviceFamily
		method   string
		params   string
		response string
	}{
		{"v1 null", FamilyV1Vacuum, string(protocol.RPCGetMultiMapsList), "[]", fixtureNullResponse},
		{"v1 missing fields", FamilyV1Vacuum, string(protocol.RPCGetMultiMapsList), "[]", `[{"map_info":[{}]}]`},
		{"v1 missing name", FamilyV1Vacuum, string(protocol.RPCGetMultiMapsList), "[]", `[{"map_info":[{"mapFlag":0}]}]`},
		{"v1 null name", FamilyV1Vacuum, string(protocol.RPCGetMultiMapsList), "[]",
			`[{"map_info":[{"mapFlag":0,"name":null}]}]`},
		{"q7 null", FamilyB01Q7, string(dependencymodels.ServiceGetMapList), "{}", fixtureNullResponse},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			session := familySession(t, test.family, rpcExchange{
				method:   test.method,
				params:   test.params,
				response: test.response,
				failure:  nil})

			result, err := session.ListMaps(t.Context(), EmptyRequest{})
			if !errors.Is(err, roborockerrors.New(roborockerrors.Protocol, "", "", nil)) ||
				len(result.Maps) != 0 ||
				!errors.Is(err, errMalformedMapList) {
				t.Fatalf("result=%+v error=%v", result, err)
			}
		})
	}
}

func TestListMapsAcceptsNullOptionalMembers(t *testing.T) {
	t.Parallel()
	// Reference shape (python-roborock tests/devices/traits/v1/test_home.py) with optional members nulled.
	session := familySession(t, FamilyV1Vacuum, rpcExchange{
		method: string(protocol.RPCGetMultiMapsList),
		params: "[]",
		response: `[{"max_multi_map":null,"map_info":[{"mapFlag":0,"name":"Ground Floor","add_time":null,` +
			`"bak_maps":[{"mapFlag":4,"add_time":1747132936}],"rooms":[{"id":16,"iot_name":null,"tag":null}]},` +
			`{"mapFlag":123,"name":"Second Floor","rooms":null}]}]`,
		failure: nil})

	result, err := session.ListMaps(t.Context(), EmptyRequest{})
	if err != nil || len(result.Maps) != 2 || result.Maps[1].ID != "123" {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}

func TestListMapsPreservesZeroIDAndFutureFields(t *testing.T) {
	t.Parallel()
	session := familySession(t, FamilyV1Vacuum, rpcExchange{
		method:   string(protocol.RPCGetMultiMapsList),
		params:   "[]",
		response: `[{"future":true,"map_info":[{"mapFlag":0,"name":"Synthetic","future":123}]}]`,
		failure:  nil})

	result, err := session.ListMaps(t.Context(), EmptyRequest{})
	if err != nil || len(result.Maps) != 1 || result.Maps[0].ID != "0" {
		t.Fatalf("result=%+v error=%v", result, err)
	}

	q7Session := familySession(t, FamilyB01Q7, rpcExchange{
		method:   string(dependencymodels.ServiceGetMapList),
		params:   "{}",
		response: `{"future":true}`,
		failure:  nil})

	result, err = q7Session.ListMaps(t.Context(), EmptyRequest{})
	if err != nil || len(result.Maps) != 0 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}
