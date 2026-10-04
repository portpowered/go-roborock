// Package mapdata decodes bounded inner map payloads without network or rendering.
package mapdata

import (
	"errors"
	"fmt"

	"github.com/portpowered/go-roborock/internal/mapmodel"
)

const (
	maxPayload = 32 * 1024 * 1024
	maxCells   = 16_000_000
)

// ErrMalformed identifies unsupported, truncated, or inconsistent map data.
var ErrMalformed = errors.New("malformed map data")

// Decode parses decrypted, inflated V1/Q7 bytes or an inner Q10 packet.
func Decode(format mapmodel.MapFormat, payload []byte) (*mapmodel.MapSnapshot, error) {
	if len(payload) == 0 || len(payload) > maxPayload {
		return nil, fmt.Errorf("payload size %d: %w", len(payload), ErrMalformed)
	}
	snapshot := &mapmodel.MapSnapshot{Format: format, Paths: []mapmodel.MapPath{}, Rooms: []mapmodel.MapRoom{},
		CurrentZones: []mapmodel.MapArea{}, Restrictions: []mapmodel.MapArea{}, Walls: []mapmodel.MapArea{},
		UnknownBlocks: []mapmodel.MapUnknownBlock{}}
	var err error
	switch format {
	case mapmodel.MapFormatV1:
		err = decodeV1(payload, snapshot)
	case mapmodel.MapFormatQ7:
		err = decodeQ7(payload, snapshot)
	case mapmodel.MapFormatQ10:
		err = decodeQ10(payload, snapshot)
	default:
		err = fmt.Errorf("format %q: %w", format, ErrMalformed)
	}
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func pointer[T any](value T) *T { return &value }

func cells(width, height int) (int, error) {
	if width <= 0 || height <= 0 || width > maxCells/height {
		return 0, fmt.Errorf("grid dimensions %dx%d: %w", width, height, ErrMalformed)
	}
	return width * height, nil
}
