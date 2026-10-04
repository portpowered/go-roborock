package mapdata_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"os"
	"testing"

	"github.com/portpowered/go-roborock/internal/mapmodel"
	"github.com/portpowered/go-roborock/pkg/dependencies/mapdata"
)

type fixtureVector struct {
	Name    string             `json:"name"`
	Format  mapmodel.MapFormat `json:"format"`
	Payload string             `json:"payload"`
	Unit    mapmodel.MapUnit   `json:"unit"`
	Frame   mapmodel.MapFrame  `json:"frame"`
	Width   int                `json:"width"`
	Height  int                `json:"height"`
	Origin  []float64          `json:"origin"`
}

type fixtureDocument struct {
	Provenance  string          `json:"provenance"`
	Revision    string          `json:"revision"`
	WheelSHA256 string          `json:"wheelSHA256"`
	Vectors     []fixtureVector `json:"vectors"`
}

func vectors(t *testing.T) []fixtureVector {
	t.Helper()
	data, err := os.ReadFile("../../../tests/replay/fixtures/maps/synthetic/decoder-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var document fixtureDocument
	if err = json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document.Provenance != "synthetic" || document.Revision != "a8260c5211e60647fc352d6b496b827865938b21" || document.WheelSHA256 != "125422c863049a519e2d76ad8683a6756e44414fc9e30a5e2b7dd5281b481281" {
		t.Fatal("invalid fixture provenance")
	}
	return document.Vectors
}

func payload(t *testing.T, vector fixtureVector) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(vector.Payload)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestIndependentSyntheticVectors(t *testing.T) {
	t.Parallel()
	for _, vector := range vectors(t) {
		t.Run(vector.Name, func(t *testing.T) {
			t.Parallel()
			snapshot, err := mapdata.Decode(vector.Format, payload(t, vector))
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Format != vector.Format {
				t.Fatal("format changed")
			}
			if vector.Width == 0 {
				if snapshot.Grid != nil || snapshot.Robot == nil || snapshot.Robot.Frame != vector.Frame || snapshot.Robot.Point.X != 7.5 || snapshot.Robot.Point.Y != 5 {
					t.Fatalf("trace projection: %+v", snapshot)
				}
				return
			}
			grid := snapshot.Grid
			if grid == nil || grid.Width != vector.Width || grid.Height != vector.Height || grid.Unit != vector.Unit || grid.Frame != vector.Frame || len(grid.Cells) != 4 {
				t.Fatalf("grid mismatch: %+v", grid)
			}
			if grid.Origin == nil || grid.Origin.X != vector.Origin[0] || grid.Origin.Y != vector.Origin[1] {
				t.Fatalf("origin mismatch: %+v", grid.Origin)
			}
			if vector.Format == mapmodel.MapFormatQ7 {
				if snapshot.FormatId == nil || *snapshot.FormatId != 0 || snapshot.Rooms[0].Material == nil || *snapshot.Rooms[0].Material != 0 || snapshot.Rooms[0].Id == nil || *snapshot.Rooms[0].Id != 0 {
					t.Fatal("proto2 present-zero fields lost")
				}
				if snapshot.Restrictions[0].Kind != "unknown" || *snapshot.Restrictions[0].SourceType != 9 {
					t.Fatal("unknown area semantics invented")
				}
			}
		})
	}
}

func TestRejectMalformedPayloads(t *testing.T) {
	t.Parallel()
	for _, vector := range vectors(t) {
		data := payload(t, vector)
		for _, length := range []int{0, 1, 2, 7, len(data) - 1} {
			_, err := mapdata.Decode(vector.Format, data[:length])
			if !errors.Is(err, mapdata.ErrMalformed) {
				t.Fatalf("%s length %d accepted: %v", vector.Name, length, err)
			}
		}
	}
	_, err := mapdata.Decode(mapmodel.MapFormatQ10, make([]byte, 33*1024*1024))
	if !errors.Is(err, mapdata.ErrMalformed) {
		t.Fatal("oversize payload accepted")
	}
}

func TestTransformsKeepFrames(t *testing.T) {
	t.Parallel()
	for _, vector := range vectors(t) {
		if vector.Width == 0 {
			continue
		}
		snapshot, err := mapdata.Decode(vector.Format, payload(t, vector))
		if err != nil {
			t.Fatal(err)
		}
		grid := *snapshot.Grid
		point := mapmodel.MapPoint{X: 0.25, Y: 0.75}
		world, err := mapdata.PixelToMap(grid, point)
		if err != nil {
			t.Fatal(err)
		}
		back, err := mapdata.MapToPixel(grid, world)
		if err != nil || math.Abs(back.X-point.X) > 1e-10 || math.Abs(back.Y-point.Y) > 1e-10 {
			t.Fatalf("%s round trip: %+v %v", vector.Name, back, err)
		}
		if vector.Format == mapmodel.MapFormatQ10 && (world.X != -37.5 || world.Y != 62.5) {
			t.Fatalf("Q10 header offset/sign: %+v", world)
		}
		rectangle := mapmodel.MapRectangle{Min: mapmodel.MapPoint{X: 0, Y: 0}, Max: mapmodel.MapPoint{X: 1, Y: 1}}
		result, err := mapdata.PixelRectangleToMap(grid, rectangle)
		if err != nil || result.Min.X >= result.Max.X || result.Min.Y >= result.Max.Y {
			t.Fatalf("rectangle: %+v %v", result, err)
		}
		rectangle.Max.X = 2
		if _, err = mapdata.PixelRectangleToMap(grid, rectangle); !errors.Is(err, mapdata.ErrMalformed) {
			t.Fatal("out-of-bounds rectangle accepted")
		}
		grid.Resolution = nil
		if _, err = mapdata.MapToPixel(grid, point); !errors.Is(err, mapdata.ErrMalformed) {
			t.Fatal("missing calibration accepted")
		}
	}
}
