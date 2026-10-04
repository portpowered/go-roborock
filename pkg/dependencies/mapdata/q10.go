package mapdata

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/portpowered/go-roborock/internal/mapmodel"
	"github.com/portpowered/go-roborock/internal/protocol"
)

const (
	q10MaxRooms   = 32
	q10WorldScale = 5.0
	q10TraceScale = 2.5
)

func signedBE(data []byte, offset int) float64 {
	return float64(int16(binary.BigEndian.Uint16(data[offset : offset+2])))
}

func decodeQ10(payload []byte, snapshot *mapmodel.MapSnapshot) error {
	if len(payload) < 2 || payload[1] != 1 || payload[0] < 1 || payload[0] > 4 {
		return fmt.Errorf("Q10 marker: %w", ErrMalformed)
	}
	snapshot.FormatId = pointer(int64(payload[0]))
	if payload[0] == 2 {
		return q10Trace(payload, snapshot)
	}
	if len(payload) < protocol.MapQ10HeaderLength {
		return fmt.Errorf("Q10 header truncated: %w", ErrMalformed)
	}
	width := int(binary.BigEndian.Uint16(payload[protocol.MapQ10WidthOffset:]))
	height := int(binary.BigEndian.Uint16(payload[protocol.MapQ10HeightOffset:]))
	count, err := cells(width, height)
	if err != nil {
		return err
	}
	length := int(binary.BigEndian.Uint16(payload[protocol.MapQ10CompressedLengthOffset:]))
	if length == 0 || length > len(payload)-protocol.MapQ10HeaderLength {
		return fmt.Errorf("Q10 layout length: %w", ErrMalformed)
	}
	end := protocol.MapQ10HeaderLength + length
	decoded, err := decompressLZ4(payload[protocol.MapQ10HeaderLength:end], count+2+q10MaxRooms*protocol.MapQ10RoomRecordLength)
	if err != nil {
		return err
	}
	if len(decoded) < count+2 || decoded[count] != 1 || int(decoded[count+1]) > q10MaxRooms || len(decoded) != count+2+int(decoded[count+1])*protocol.MapQ10RoomRecordLength {
		return fmt.Errorf("Q10 room records: %w", ErrMalformed)
	}
	grid := &mapmodel.MapGrid{Width: width, Height: height, Cells: append([]byte(nil), decoded[:count]...), Unit: mapmodel.MapUnitMillimeters, Frame: mapmodel.MapFrameQ10World, TopDown: true, RowYDirection: -1}
	x, y := signedBE(payload, protocol.MapQ10OriginXOffset), signedBE(payload, protocol.MapQ10OriginYOffset)
	resolution := float64(binary.BigEndian.Uint16(payload[protocol.MapQ10ResolutionOffset:])) * 10
	if x != 0 || y != 0 {
		grid.Origin = &mapmodel.MapPoint{X: -x / 10 * resolution, Y: y / 10 * resolution}
	}
	if resolution > 0 {
		grid.Resolution = pointer(resolution)
	}
	snapshot.Grid = grid
	snapshot.MapId = pointer(int64(binary.BigEndian.Uint32(payload[protocol.MapQ10IDOffset:])))
	dx, dy := signedBE(payload, protocol.MapQ10DockXOffset), signedBE(payload, protocol.MapQ10DockYOffset)
	if (dx != 0 || dy != 0) && dx != -1 && dy != -1 {
		snapshot.Dock = &mapmodel.MapPose{Point: mapmodel.MapPoint{X: dx / 10, Y: dy / 10}, Unit: mapmodel.MapUnitPixels, Frame: mapmodel.MapFrameQ10Array, Heading: pointer(signedBE(payload, protocol.MapQ10DockHeadingOffset))}
	}
	for start := count + 2; start < len(decoded); start += protocol.MapQ10RoomRecordLength {
		record := decoded[start : start+protocol.MapQ10RoomRecordLength]
		nameLength := int(record[protocol.MapQ10RoomNameLengthOffset])
		if nameLength > protocol.MapQ10RoomRecordLength-protocol.MapQ10RoomNameOffset {
			return fmt.Errorf("Q10 room name length: %w", ErrMalformed)
		}
		snapshot.Rooms = append(snapshot.Rooms, mapmodel.MapRoom{Id: pointer(int64(binary.BigEndian.Uint16(record[:2]))), Name: pointer(strings.ToValidUTF8(string(record[protocol.MapQ10RoomNameOffset:protocol.MapQ10RoomNameOffset+nameLength]), "�")), Unit: mapmodel.MapUnitMillimeters, Frame: mapmodel.MapFrameQ10World})
	}
	return q10Tail(payload[end:], end, snapshot)
}

func q10Trace(payload []byte, snapshot *mapmodel.MapSnapshot) error {
	if len(payload) < protocol.MapQ10TraceHeaderLength {
		return fmt.Errorf("Q10 trace header: %w", ErrMalformed)
	}
	count := int(binary.BigEndian.Uint16(payload[protocol.MapQ10TracePointCountOffset:]))
	if len(payload)-protocol.MapQ10TraceHeaderLength != count*4 {
		return fmt.Errorf("Q10 trace count: %w", ErrMalformed)
	}
	path := mapmodel.MapPath{Kind: "trace", Points: make([]mapmodel.MapPoint, 0, count), Unit: mapmodel.MapUnitMillimeters, Frame: mapmodel.MapFrameQ10Trace}
	for start := protocol.MapQ10TraceHeaderLength; start < len(payload); start += 4 {
		path.Points = append(path.Points, mapmodel.MapPoint{X: signedBE(payload, start) * q10TraceScale, Y: signedBE(payload, start+2) * q10TraceScale})
	}
	snapshot.Paths = append(snapshot.Paths, path)
	if count > 0 {
		snapshot.Robot = &mapmodel.MapPose{Point: path.Points[count-1], Unit: path.Unit, Frame: path.Frame, Heading: pointer(signedBE(payload, protocol.MapQ10TraceHeadingOffset)), HeadingUnit: pointer(mapmodel.Degrees)}
	}
	return nil
}

func q10Tail(tail []byte, offset int, snapshot *mapmodel.MapSnapshot) error {
	if len(tail) == 0 {
		return nil
	}
	if len(tail) < 2 {
		return fmt.Errorf("Q10 tail truncated: %w", ErrMalformed)
	}
	count, vertices := int(tail[0]), int(tail[1])
	if count > 0 && (vertices == 0 || vertices > 16) {
		return fmt.Errorf("Q10 erase vertex count: %w", ErrMalformed)
	}
	end := 2 + count*vertices*4
	if end > len(tail) {
		return fmt.Errorf("Q10 erase zone truncated: %w", ErrMalformed)
	}
	for start := 2; start < end; start += vertices * 4 {
		area := mapmodel.MapArea{Kind: "erase", Unit: mapmodel.MapUnitMillimeters, Frame: mapmodel.MapFrameQ10World, Points: make([]mapmodel.MapPoint, 0, vertices)}
		for i := 0; i < vertices; i++ {
			position := start + i*4
			area.Points = append(area.Points, mapmodel.MapPoint{X: signedBE(tail, position) * q10WorldScale, Y: signedBE(tail, position+2) * q10WorldScale})
		}
		snapshot.Restrictions = append(snapshot.Restrictions, area)
	}
	if end < len(tail) {
		snapshot.UnknownBlocks = append(snapshot.UnknownBlocks, mapmodel.MapUnknownBlock{Type: 0, Offset: offset + end, HeaderLength: 0, DataLength: len(tail) - end})
	}
	return nil
}
