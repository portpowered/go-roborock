package mqtt

import (
	"encoding/base64"
	"encoding/binary"
	"math"

	"github.com/portpowered/go-roborock/internal/protocol"
)

// Q10Zone describes one rectangular zone in the reference millimetre frame,
// whose dock origin is (25500, 25500). Coordinates must align to a 5 mm grid.
type Q10Zone struct {
	X1      int64
	Y1      int64
	X2      int64
	Y2      int64
	Repeats int
}

// EncodeQ10Zone encodes the reference Q10 zone payload for a typed clean command.
func EncodeQ10Zone(zone Q10Zone) (string, error) {
	if zone.Repeats < protocol.Q10ZoneMinRepeat || zone.Repeats > protocol.Q10ZoneMaxRepeat ||
		zone.X1 == zone.X2 || zone.Y1 == zone.Y2 {
		return "", invalid("Q10 zone", "zone must have area and repeat count from 1 through 3")
	}

	values := []int64{
		min(zone.X1, zone.X2), min(zone.Y1, zone.Y2), max(zone.X1, zone.X2), min(zone.Y1, zone.Y2),
		max(zone.X1, zone.X2), max(zone.Y1, zone.Y2), min(zone.X1, zone.X2), max(zone.Y1, zone.Y2),
	}
	size := protocol.Q10ZoneHeaderSize + protocol.Q10ZoneVertices*protocol.Q10ZoneVertexBytes +
		protocol.Q10ZoneNameLengthBytes + protocol.Q10ZoneNameBytes
	payload := make([]byte, size)
	copy(payload, []byte{protocol.Q10ZoneVersion, byte(zone.Repeats), protocol.Q10ZoneCount, protocol.Q10ZoneVertices})

	for index, value := range values {
		// Check before subtraction so extreme user coordinates cannot overflow.
		if value < protocol.Q10ZoneCoordinateOffsetMM+math.MinInt16*protocol.Q10ZoneVectorUnitMM ||
			value > protocol.Q10ZoneCoordinateOffsetMM+math.MaxInt16*protocol.Q10ZoneVectorUnitMM {
			return "", invalid("Q10 zone", "coordinate outside int16 vector range")
		}

		relative := value - protocol.Q10ZoneCoordinateOffsetMM
		if relative%protocol.Q10ZoneVectorUnitMM != 0 {
			return "", invalid("Q10 zone", "coordinate must align to the 5 mm grid")
		}

		offset := protocol.Q10ZoneHeaderSize + index*protocol.Q10ZoneCoordinateBytes
		coordinate := uint16(int16(relative / protocol.Q10ZoneVectorUnitMM)) //nolint:gosec // Signed range checked above.
		binary.BigEndian.PutUint16(payload[offset:offset+protocol.Q10ZoneCoordinateBytes], coordinate)
	}

	return base64.StdEncoding.EncodeToString(payload), nil
}
