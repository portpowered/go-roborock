//nolint:testpackage // LIB-05: Private paired seam proves rejected room identifiers cannot publish commands.
package roborock

import (
	"errors"
	"testing"

	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

type invalidFamilyRoomCase struct {
	family DeviceFamily
	ids    []int64
}

func TestFamilyRoomIdentifiersRejectBeforeSend(t *testing.T) {
	t.Parallel()

	cases := []invalidFamilyRoomCase{
		{family: FamilyB01Q7, ids: []int64{-1}},
		{family: FamilyB01Q7, ids: []int64{4294967296}},
		{family: FamilyB01Q7, ids: []int64{9, -1}},
		{family: FamilyB01Q10, ids: []int64{-1}},
		{family: FamilyB01Q10, ids: []int64{65536}},
		{family: FamilyB01Q10, ids: []int64{9, 65536}},
	}

	for _, test := range cases {
		session := familySession(t, test.family)

		ack, err := session.CleanSegments(t.Context(), CleanSegmentsRequest{Segments: test.ids, Repeats: 1})
		if !errors.Is(err, roborockerrors.New(roborockerrors.InvalidArgument, "", "", nil)) {
			t.Fatalf("%s IDs %v: expected invalid_argument, got %v", test.family, test.ids, err)
		}

		if ack.Sent || ack.Acknowledged {
			t.Fatalf("invalid room identifiers reported sent: %+v", ack)
		}
	}
}

func TestQ7ReportedRoomIdentifierDomain(t *testing.T) {
	t.Parallel()

	session := familySession(t, FamilyB01Q7, rpcExchange{
		method:   "service.set_room_clean",
		params:   `{"clean_type":1,"ctrl_value":1,"room_ids":[0,4294967295]}`,
		response: `{"accepted":true}`, failure: nil,
	})

	ack, err := session.CleanSegments(t.Context(), CleanSegmentsRequest{Segments: []int64{0, 4294967295}, Repeats: 1})
	if err != nil {
		t.Fatal(err)
	}

	if !ack.Sent || !ack.Acknowledged {
		t.Fatalf("Q7 boundary command result: %+v", ack)
	}
}

func TestQ10ReportedRoomIdentifierDomain(t *testing.T) {
	t.Parallel()

	//nolint:misspell // Firmware requires this exact JSON field spelling.
	session := familySession(t, FamilyB01Q10, rpcExchange{
		method:   "synthetic-q10-clean",
		params:   `{"clean_paramters":[0,65535],"cmd":2}`,
		response: fixtureNullResponse, failure: nil,
	})

	ack, err := session.CleanSegments(t.Context(), CleanSegmentsRequest{Segments: []int64{0, 65535}, Repeats: 1})
	if err != nil {
		t.Fatal(err)
	}

	if !ack.Sent || ack.Acknowledged {
		t.Fatalf("Q10 boundary publication: %+v", ack)
	}
}
