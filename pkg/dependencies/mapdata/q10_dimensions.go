package mapdata

import (
	"fmt"

	"github.com/portpowered/go-roborock/internal/protocol"
)

func q10DeclaredDimensions(width, height int) error {
	if width <= 0 {
		return fmt.Errorf("Q10 width: %w", ErrMalformed)
	}

	if height == 0 {
		return nil
	}

	_, err := cells(width, height)

	return err
}

func q10LayoutDimensions(decoded []byte, width, height int) (int, int, error) {
	if height > 0 {
		count := width * height

		err := q10RoomRecords(decoded, count)

		if err == nil {
			return count, height, nil
		}
	}

	for roomCount := 0; roomCount <= protocol.MapQ10MaximumRooms; roomCount++ {
		metadataLength := protocol.MapQ10RoomHeaderLength + roomCount*protocol.MapQ10RoomRecordLength

		count := len(decoded) - metadataLength
		if count <= 0 || count > maxCells || count%width != 0 {
			continue
		}

		err := q10RoomRecords(decoded, count)
		if err == nil {
			return count, count / width, nil
		}
	}

	return 0, 0, fmt.Errorf("Q10 dimensions cannot be inferred from exact room records: %w", ErrMalformed)
}
