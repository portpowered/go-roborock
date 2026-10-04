package mapdata_test

import (
	"encoding/binary"
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/go-roborock/internal/mapmodel"
	"github.com/portpowered/go-roborock/pkg/dependencies/mapdata"
)

func TestQ10ExactDimensionInference(t *testing.T) {
	t.Parallel()

	data := payload(t, vectors(t)[2])

	baseline, err := mapdata.Decode(mapmodel.MapFormatQ10, data)
	if err != nil {
		t.Fatal(err)
	}

	for _, height := range []uint16{0, 1} {
		modified := append([]byte(nil), data...)
		binary.BigEndian.PutUint16(modified[9:], height)

		snapshot, decodeErr := mapdata.Decode(mapmodel.MapFormatQ10, modified)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}

		if !reflect.DeepEqual(snapshot.Grid, baseline.Grid) {
			t.Fatal("height inference changed calibration or grid")
		}
	}

	binary.BigEndian.PutUint16(data[9:], 0)
	binary.BigEndian.PutUint16(data[7:], 3)

	_, err = mapdata.Decode(mapmodel.MapFormatQ10, data)
	if !errors.Is(err, mapdata.ErrMalformed) {
		t.Fatal("nonrectangular inferred grid accepted")
	}
}
