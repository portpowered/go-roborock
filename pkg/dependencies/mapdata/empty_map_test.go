package mapdata_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/portpowered/go-roborock/internal/mapmodel"
	"github.com/portpowered/go-roborock/pkg/dependencies/mapdata"
)

// capturedNoMapPayload is a payload captured from an S8 MaxV Ultra whose status reported map_present 0:
// the image block is 0 by 0 with no cells. It is a map with no raster, not a damaged one.
func capturedNoMapPayload(t *testing.T) []byte {
	t.Helper()

	raw, err := os.ReadFile("../../../tests/replay/fixtures/maps/captured/v1-no-map-loaded.json")
	if err != nil {
		t.Fatal(err)
	}

	var document struct {
		Payload string `json:"payload"`
	}

	err = json.Unmarshal(raw, &document)
	if err != nil {
		t.Fatal(err)
	}

	data, err := base64.StdEncoding.DecodeString(document.Payload)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func TestV1NoMapLoadedDecodes(t *testing.T) {
	t.Parallel()

	snapshot, err := mapdata.Decode(mapmodel.MapFormatV1, capturedNoMapPayload(t))
	if err != nil {
		t.Fatal(err)
	}

	if snapshot.Grid != nil || snapshot.Robot == nil || len(snapshot.Paths) != 1 {
		t.Fatalf("empty map decoded wrongly: grid %v robot %v paths %d", snapshot.Grid, snapshot.Robot, len(snapshot.Paths))
	}
}

func TestV1NoMapLoadedStillRejectsDamage(t *testing.T) {
	t.Parallel()

	data := capturedNoMapPayload(t)

	// Truncations, and a map with no image block at all (just the header), stay malformed.
	for _, length := range []int{0, 1, 2, 7, 20, len(data) - 1} {
		_, err := mapdata.Decode(mapmodel.MapFormatV1, data[:length])
		if !errors.Is(err, mapdata.ErrMalformed) {
			t.Fatalf("length %d accepted: %v", length, err)
		}
	}
}
