package mapdata

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/portpowered/go-roborock/internal/mapmodel"
	"github.com/portpowered/go-roborock/internal/protocol"
)

const (
	signed16Boundary = 1 << 15
	signed16Modulus  = 1 << 16
)

func signedBE(data []byte, offset int) float64 {
	value := int(binary.BigEndian.Uint16(data[offset : offset+protocol.MapQ10SignedCoordinateSize]))
	if value >= signed16Boundary {
		value -= signed16Modulus
	}

	return float64(value)
}

func decodeQ10(payload []byte, snapshot *mapmodel.MapSnapshot) error {
	err := q10Marker(payload)
	if err != nil {
		return err
	}

	snapshot.FormatId = pointer(int64(payload[0]))
	if payload[0] == protocol.MapQ10KindTrace {
		return q10Trace(payload, snapshot)
	}

	if len(payload) < protocol.MapQ10HeaderLength {
		return fmt.Errorf("Q10 header truncated: %w", ErrMalformed)
	}

	width := int(binary.BigEndian.Uint16(payload[protocol.MapQ10WidthOffset:]))
	height := int(binary.BigEndian.Uint16(payload[protocol.MapQ10HeightOffset:]))

	err = q10DeclaredDimensions(width, height)
	if err != nil {
		return err
	}

	length := int(binary.BigEndian.Uint16(payload[protocol.MapQ10CompressedLengthOffset:]))
	if length == 0 || length > len(payload)-protocol.MapQ10HeaderLength {
		return fmt.Errorf("Q10 layout length: %w", ErrMalformed)
	}

	end := protocol.MapQ10HeaderLength + length

	limit := maxCells + protocol.MapQ10RoomHeaderLength + protocol.MapQ10MaximumRooms*protocol.MapQ10RoomRecordLength

	decoded, err := decompressLZ4(payload[protocol.MapQ10HeaderLength:end], limit)
	if err != nil {
		return err
	}

	count, height, err := q10LayoutDimensions(decoded, width, height)
	if err != nil {
		return err
	}

	q10Grid(payload, decoded[:count], width, height, snapshot)

	err = q10Rooms(decoded[count+protocol.MapQ10RoomHeaderLength:], snapshot)
	if err != nil {
		return err
	}

	return q10Tail(payload[end:], end, snapshot)
}

func q10Grid(payload, data []byte, width, height int, snapshot *mapmodel.MapSnapshot) {
	grid := &mapmodel.MapGrid{
		Width:  width,
		Height: height,
		Cells: append([]byte(nil),
			data...),
		Unit:          mapmodel.MapUnitMillimeters,
		Frame:         mapmodel.MapFrameQ10World,
		TopDown:       true,
		RowYDirection: -1,
		Origin:        nil,
		Resolution:    nil,
		RoomCells:     nil,
	}
	originX, originY := signedBE(payload, protocol.MapQ10OriginXOffset), signedBE(payload, protocol.MapQ10OriginYOffset)

	resolution := float64(binary.BigEndian.Uint16(payload[protocol.MapQ10ResolutionOffset:])) *
		protocol.MapQ10ResolutionUnitMillimeters

	if originX != 0 || originY != 0 {
		grid.Origin = &mapmodel.MapPoint{
			X: -originX / protocol.MapQ10HeaderUnitsPerPixel * resolution,
			Y: originY / protocol.MapQ10HeaderUnitsPerPixel * resolution,
		}
	}

	if resolution > 0 {
		grid.Resolution = pointer(resolution)
	}

	snapshot.Grid = grid
	snapshot.MapId = pointer(int64(binary.BigEndian.Uint32(payload[protocol.MapQ10IDOffset:])))

	dockX, dockY := signedBE(payload, protocol.MapQ10DockXOffset), signedBE(payload, protocol.MapQ10DockYOffset)

	if (dockX != 0 || dockY != 0) && dockX != -1 && dockY != -1 {
		snapshot.Dock = &mapmodel.MapPose{
			Point: mapmodel.MapPoint{
				X: dockX / protocol.MapQ10HeaderUnitsPerPixel,
				Y: dockY / protocol.MapQ10HeaderUnitsPerPixel,
			},
			Unit:  mapmodel.MapUnitPixels,
			Frame: mapmodel.MapFrameQ10Array,
			Heading: pointer(signedBE(payload,
				protocol.MapQ10DockHeadingOffset)),
			HeadingUnit: nil,
		}
	}
}

func q10Rooms(decoded []byte, snapshot *mapmodel.MapSnapshot) error {
	for start := 0; start < len(decoded); start += protocol.MapQ10RoomRecordLength {
		record := decoded[start : start+protocol.MapQ10RoomRecordLength]

		nameLength := int(record[protocol.MapQ10RoomNameLengthOffset])
		if nameLength > protocol.MapQ10RoomRecordLength-protocol.MapQ10RoomNameOffset {
			return fmt.Errorf("Q10 room name length: %w", ErrMalformed)
		}

		snapshot.Rooms = append(snapshot.Rooms, mapmodel.MapRoom{
			Id: pointer(int64(binary.BigEndian.Uint16(record[:protocol.MapQ10RoomIDLength]))),
			Name: pointer(strings.ToValidUTF8(
				string(record[protocol.MapQ10RoomNameOffset:protocol.MapQ10RoomNameOffset+nameLength]),
				"�")),
			Unit:       mapmodel.MapUnitMillimeters,
			Frame:      mapmodel.MapFrameQ10World,
			Bounds:     nil,
			CleanState: nil,
			Label:      nil,
			Material:   nil,
			Outline:    nil,
			RoomType:   nil,
		})
	}

	return nil
}

func q10Trace(payload []byte, snapshot *mapmodel.MapSnapshot) error {
	if len(payload) < protocol.MapQ10TraceHeaderLength {
		return fmt.Errorf("Q10 trace header: %w", ErrMalformed)
	}

	count := int(binary.BigEndian.Uint16(payload[protocol.MapQ10TracePointCountOffset:]))
	if len(payload)-protocol.MapQ10TraceHeaderLength != count*protocol.MapQ10PointStride {
		return fmt.Errorf("Q10 trace count: %w", ErrMalformed)
	}

	path := mapmodel.MapPath{
		Kind: "trace",
		Points: make([]mapmodel.MapPoint,
			0,
			count),
		Unit:  mapmodel.MapUnitMillimeters,
		Frame: mapmodel.MapFrameQ10Trace,
	}
	for start := protocol.MapQ10TraceHeaderLength; start < len(payload); start += protocol.MapQ10PointStride {
		path.Points = append(path.Points, mapmodel.MapPoint{
			X: signedBE(payload,
				start) * protocol.MapQ10TraceUnitMillimeters,
			Y: signedBE(payload,
				start+protocol.MapQ10SignedCoordinateSize) * protocol.MapQ10TraceUnitMillimeters,
		})
	}

	snapshot.Paths = append(snapshot.Paths, path)
	if count > 0 {
		snapshot.Robot = &mapmodel.MapPose{
			Point: path.Points[count-1],
			Unit:  path.Unit,
			Frame: path.Frame,
			Heading: pointer(signedBE(payload,
				protocol.MapQ10TraceHeadingOffset)),
			HeadingUnit: pointer(mapmodel.Degrees),
		}
	}

	return nil
}

func q10Tail(tail []byte, offset int, snapshot *mapmodel.MapSnapshot) error {
	if len(tail) == 0 {
		return nil
	}

	if len(tail) < protocol.MapQ10RoomHeaderLength {
		return fmt.Errorf("Q10 tail truncated: %w", ErrMalformed)
	}

	count, vertices := int(tail[0]), int(tail[1])
	if count > 0 && (vertices == 0 || vertices > protocol.MapQ10EraseMaximumVertices) {
		return fmt.Errorf("Q10 erase vertex count: %w", ErrMalformed)
	}

	end := protocol.MapQ10RoomHeaderLength + count*vertices*protocol.MapQ10PointStride
	if end > len(tail) {
		return fmt.Errorf("Q10 erase zone truncated: %w", ErrMalformed)
	}

	for start := protocol.MapQ10RoomHeaderLength; start < end; start += vertices * protocol.MapQ10PointStride {
		area := mapmodel.MapArea{
			Kind:  "erase",
			Unit:  mapmodel.MapUnitMillimeters,
			Frame: mapmodel.MapFrameQ10World,
			Points: make([]mapmodel.MapPoint,
				0,
				vertices),
			SourceIndex:  nil,
			SourceStatus: nil,
			SourceType:   nil,
		}

		for i := range vertices {
			position := start + i*protocol.MapQ10PointStride
			area.Points = append(area.Points, mapmodel.MapPoint{
				X: signedBE(tail,
					position) * protocol.MapQ10WorldUnitMillimeters,
				Y: signedBE(tail,
					position+protocol.MapQ10SignedCoordinateSize) * protocol.MapQ10WorldUnitMillimeters,
			})
		}

		snapshot.Restrictions = append(snapshot.Restrictions, area)
	}

	if end < len(tail) {
		snapshot.UnknownBlocks = append(snapshot.UnknownBlocks, mapmodel.MapUnknownBlock{
			Type:         0,
			Offset:       offset + end,
			HeaderLength: 0,
			DataLength:   len(tail) - end,
		})
	}

	return nil
}

func q10Marker(payload []byte) error {
	if len(payload) < protocol.MapQ10SignedCoordinateSize || payload[1] != protocol.MapQ10MarkerRevision ||
		payload[0] < protocol.MapQ10KindCurrent || payload[0] > protocol.MapQ10KindSaved {
		return fmt.Errorf("Q10 marker: %w", ErrMalformed)
	}

	return nil
}

func q10RoomRecords(decoded []byte, count int) error {
	if len(decoded) < count+protocol.MapQ10RoomHeaderLength ||
		decoded[count] != protocol.MapQ10RoomHeaderMarker ||
		int(decoded[count+protocol.MapQ10RoomCountOffset]) > protocol.MapQ10MaximumRooms ||
		len(decoded) != count+protocol.MapQ10RoomHeaderLength+
			int(decoded[count+protocol.MapQ10RoomCountOffset])*protocol.MapQ10RoomRecordLength {
		return fmt.Errorf("Q10 room records: %w", ErrMalformed)
	}

	return nil
}
