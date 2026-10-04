package mapdata_test

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/portpowered/go-roborock/internal/mapmodel"
	"github.com/portpowered/go-roborock/pkg/dependencies/mapdata"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestQ7MessageAllocationBound(t *testing.T) {
	t.Parallel()

	data := make([]byte, 0, 140_000)
	for range 65_536 {
		data = protowire.AppendTag(data, 5, protowire.BytesType)
		data = protowire.AppendBytes(data, nil)
	}

	_, err := mapdata.Decode(mapmodel.MapFormatQ7, data)
	if !errors.Is(err, mapdata.ErrMalformed) {
		t.Fatal("protobuf allocation bound not enforced")
	}
}

func TestQ10LZ4MalformedBlocks(t *testing.T) {
	t.Parallel()

	for _, block := range [][]byte{{}, {0xF0}, {0x10}, {0, 0, 0}, {0, 1, 0}, {0x10, 1, 2, 0}, {0x1F, 1, 1, 0, 255}} {
		header := make([]byte, 29)
		header[0], header[1] = 1, 1
		binary.BigEndian.PutUint16(header[7:], 2)
		binary.BigEndian.PutUint16(header[9:], 2)

		if len(block) > 255 {
			t.Fatal("test literal overflow")
		}

		header[28] = byte(len(block) & 255)

		_, err := mapdata.Decode(mapmodel.MapFormatQ10, append(append([]byte(nil), header...), block...))
		if !errors.Is(err, mapdata.ErrMalformed) {
			t.Fatalf("malformed LZ4 accepted: %x", block)
		}
	}
}

func TestV1GeometryAndUnknownBlock(t *testing.T) {
	t.Parallel()
	data := payload(t, vectors(t)[0])
	path := []byte{3, 0, 20, 0, 8, 0, 0, 0, 2, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 10, 0, 20, 0, 30, 0, 40, 0}
	data = append(data, path...)
	data = append(data, []byte{6, 0, 10, 0, 8, 0, 0, 0, 1, 0, 10, 0, 20, 0, 30, 0, 40, 0}...)
	data = append(data, []byte{10, 0, 10, 0, 8, 0, 0, 0, 1, 0, 10, 0, 20, 0, 30, 0, 40, 0}...)
	data = append(data, []byte{199, 0, 8, 0, 2, 0, 0, 0, 10, 20}...)

	snapshot, err := mapdata.Decode(mapmodel.MapFormatV1, data)
	if err != nil {
		t.Fatal(err)
	}

	if len(snapshot.Paths) != 1 || len(snapshot.Paths[0].Points) != 2 || snapshot.Paths[0].Points[1].X != 30 ||
		len(snapshot.CurrentZones) != 1 || len(snapshot.Walls) != 1 {
		t.Fatal("V1 overlays lost")
	}

	assertUnknownMetadata(t, snapshot)
}

func assertUnknownMetadata(t *testing.T, snapshot *mapmodel.MapSnapshot) {
	t.Helper()

	if len(snapshot.UnknownBlocks) != 1 || snapshot.UnknownBlocks[0].Type != 199 ||
		snapshot.UnknownBlocks[0].DataLength != 2 {
		t.Fatal("unknown block metadata lost")
	}

	data := payload(t, vectors(t)[0])
	data = append(data, []byte{199, 0, 8, 0, 255, 0, 0, 0, 10, 20}...)

	_, err := mapdata.Decode(mapmodel.MapFormatV1, data)
	if !errors.Is(err, mapdata.ErrMalformed) {
		t.Fatal("overflowing block length accepted")
	}
}

func TestGeometryRejectsNonFinite(t *testing.T) {
	t.Parallel()

	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, vector := range vectors(t) {
			if vector.Width == 0 {
				continue
			}

			snapshot, err := mapdata.Decode(vector.Format, payload(t, vector))
			if err != nil {
				t.Fatal(err)
			}

			point := mapmodel.MapPoint{X: value, Y: 0}

			_, err = mapdata.MapToPixel(*snapshot.Grid, point)
			if !errors.Is(err, mapdata.ErrMalformed) {
				t.Fatal("non-finite map point accepted")
			}

			_, err = mapdata.PixelToMap(*snapshot.Grid, point)
			if !errors.Is(err, mapdata.ErrMalformed) {
				t.Fatal("non-finite pixel accepted")
			}
		}
	}
}
