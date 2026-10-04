package mapdata

import (
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/portpowered/go-roborock/internal/mapmodel"
	"github.com/portpowered/go-roborock/internal/protocol"
)

func decodeV1(payload []byte, snapshot *mapmodel.MapSnapshot) error {
	if len(payload) < protocol.MapV1HeaderMinimum ||
		string(payload[:protocol.MapV1HeaderLengthOffset]) != protocol.MapV1Magic {
		return fmt.Errorf("V1 header: %w", ErrMalformed)
	}

	start := int(binary.LittleEndian.Uint16(payload[protocol.MapV1HeaderLengthOffset:]))
	if start < protocol.MapV1HeaderMinimum || start > len(payload) {
		return fmt.Errorf("V1 header length: %w", ErrMalformed)
	}

	snapshot.MapId = pointer(int64(binary.LittleEndian.Uint32(payload[protocol.MapV1MapIDOffset:])))

	for start < len(payload) {
		if len(payload)-start < protocol.MapBlockHeaderMinimum {
			return fmt.Errorf("V1 truncated block header: %w", ErrMalformed)
		}

		header, length, err := v1BlockBounds(payload[start:])
		if err != nil {
			return err
		}

		kind := binary.LittleEndian.Uint16(payload[start : start+protocol.MapBlockTypeSize])
		block := payload[start : start+header]

		data := payload[start+header : start+header+int(length)]

		err = v1Block(kind, block, data, start, snapshot)
		if err != nil {
			return err
		}

		start += header + int(length)
	}

	if snapshot.Grid == nil {
		return fmt.Errorf("V1 grid missing: %w", ErrMalformed)
	}

	return nil
}

func v1Block(kind uint16, header, data []byte, offset int, snapshot *mapmodel.MapSnapshot) error {
	switch kind {
	case protocol.MapBlockImage:
		return v1Grid(header, data, snapshot)
	case protocol.MapBlockCharger, protocol.MapBlockRobot:
		return v1Pose(kind, data, snapshot)
	case protocol.MapBlockPath, protocol.MapBlockGotoPath, protocol.MapBlockPredictedPath:
		return v1Path(kind, header, data, snapshot)
	case protocol.MapBlockCurrentZones, protocol.MapBlockWalls, protocol.MapBlockNoGo,
		protocol.MapBlockNoMop, protocol.MapBlockNoCarpet:
		return v1Areas(kind, header, data, snapshot)
	default:
		snapshot.UnknownBlocks = append(snapshot.UnknownBlocks, mapmodel.MapUnknownBlock{
			Type:         int64(kind),
			Offset:       offset,
			HeaderLength: len(header),
			DataLength:   len(data),
		})
	}

	return nil
}

func v1Points(data []byte) []mapmodel.MapPoint {
	points := make([]mapmodel.MapPoint, 0, len(data)/protocol.MapV1PointStride)
	for start := 0; start < len(data); start += protocol.MapV1PointStride {
		points = append(points, mapmodel.MapPoint{
			X: float64(binary.LittleEndian.Uint16(data[start : start+protocol.MapV1PointCoordinateSize])),
			Y: float64(binary.LittleEndian.Uint16(data[start+protocol.MapV1PointCoordinateSize:])),
		})
	}

	return points
}

func v1Areas(kind uint16, header, data []byte, snapshot *mapmodel.MapSnapshot) error {
	if len(header) < protocol.MapV1AreaCountHeaderLength {
		return fmt.Errorf("V1 area count missing: %w", ErrMalformed)
	}

	name, stride := v1AreaKind(kind)

	count := int(binary.LittleEndian.Uint16(header[protocol.MapV1AreaCountOffset:]))
	if count*stride != len(data) {
		return fmt.Errorf("V1 area records: %w", ErrMalformed)
	}

	for start := 0; start < len(data); start += stride {
		area := mapmodel.MapArea{
			Kind:         name,
			Points:       v1Points(data[start : start+stride]),
			Unit:         mapmodel.MapUnitMillimeters,
			Frame:        mapmodel.MapFrameV1World,
			SourceIndex:  nil,
			SourceStatus: nil,
			SourceType:   nil,
		}

		switch kind {
		case protocol.MapBlockCurrentZones:
			snapshot.CurrentZones = append(snapshot.CurrentZones, area)
		case protocol.MapBlockWalls:
			snapshot.Walls = append(snapshot.Walls, area)
		default:
			snapshot.Restrictions = append(snapshot.Restrictions, area)
		}
	}

	return nil
}

func v1Grid(header, data []byte, snapshot *mapmodel.MapSnapshot) error {
	if len(header) < protocol.MapV1ImageHeaderMinimum {
		return fmt.Errorf("V1 image header: %w", ErrMalformed)
	}

	fields := header[len(header)-protocol.MapV1ImageFieldsLength:]
	width := int(binary.LittleEndian.Uint32(fields[protocol.MapV1ImageWidthOffset:]))
	height := int(binary.LittleEndian.Uint32(fields[protocol.MapV1ImageHeightOffset:]))

	count, err := cells(width, height)
	if err != nil {
		return err
	}

	if len(data) != count {
		return fmt.Errorf("V1 image length: %w", ErrMalformed)
	}

	origin := &mapmodel.MapPoint{
		X: float64(binary.LittleEndian.Uint32(fields[protocol.MapV1ImageLeftOffset:])) * protocol.MapV1Resolution,
		Y: float64(binary.LittleEndian.Uint32(fields[protocol.MapV1ImageTopOffset:])) * protocol.MapV1Resolution,
	}
	grid := &mapmodel.MapGrid{
		Width:  width,
		Height: height,
		Cells: append([]byte(nil),
			data...),
		Origin:        origin,
		Resolution:    pointer(float64(protocol.MapV1Resolution)),
		Unit:          mapmodel.MapUnitMillimeters,
		Frame:         mapmodel.MapFrameV1World,
		TopDown:       false,
		RowYDirection: 1,
		RoomCells:     nil,
	}
	snapshot.Grid = grid
	v1Rooms(data, width, origin, snapshot)

	return nil
}

func v1Pose(kind uint16, data []byte, snapshot *mapmodel.MapSnapshot) error {
	if len(data) != protocol.MapV1PoseLength && len(data) != protocol.MapV1PoseHeadingLength {
		return fmt.Errorf("V1 pose length: %w", ErrMalformed)
	}

	pose := &mapmodel.MapPose{
		Point: mapmodel.MapPoint{
			X: float64(binary.LittleEndian.Uint32(data[:protocol.MapV1CoordinateSize])),
			Y: float64(binary.LittleEndian.Uint32(data[protocol.MapV1CoordinateSize:protocol.MapV1PoseLength])),
		},
		Unit:        mapmodel.MapUnitMillimeters,
		Frame:       mapmodel.MapFrameV1World,
		Heading:     nil,
		HeadingUnit: nil,
	}

	if len(data) == protocol.MapV1PoseHeadingLength {
		angle := int64(binary.LittleEndian.Uint32(data[protocol.MapV1PoseHeadingOffset:]))
		if angle > protocol.MapV1HeadingMask {
			angle = (angle & protocol.MapV1HeadingMask) - protocol.MapV1HeadingWrap
		}

		pose.Heading = pointer(float64(angle))
		pose.HeadingUnit = pointer(mapmodel.Degrees)
	}

	if kind == protocol.MapBlockRobot {
		snapshot.Robot = pose
	} else {
		snapshot.Dock = pose
	}

	return nil
}

func v1Path(kind uint16, header, data []byte, snapshot *mapmodel.MapSnapshot) error {
	if len(header) < protocol.MapV1PathHeaderLength || len(data)%protocol.MapV1PointStride != 0 {
		return fmt.Errorf("V1 path length: %w", ErrMalformed)
	}

	name := "clean"
	if kind == protocol.MapBlockGotoPath {
		name = "goto"
	}

	if kind == protocol.MapBlockPredictedPath {
		name = "predicted"
	}

	snapshot.Paths = append(snapshot.Paths, mapmodel.MapPath{
		Kind:   name,
		Points: v1Points(data),
		Unit:   mapmodel.MapUnitMillimeters,
		Frame:  mapmodel.MapFrameV1World,
	})

	return nil
}

func v1AreaKind(kind uint16) (string, int) {
	stride := protocol.MapV1ZoneStride

	name := "current"

	switch kind {
	case protocol.MapBlockWalls:
		name = "wall"
	case protocol.MapBlockNoGo:
		name = "no-go"
		stride = protocol.MapV1PolygonStride
	case protocol.MapBlockNoMop:
		name = "no-mop"
		stride = protocol.MapV1PolygonStride
	case protocol.MapBlockNoCarpet:
		name = "no-carpet"
		stride = protocol.MapV1PolygonStride
	}

	return name, stride
}

func v1Rooms(data []byte, width int, origin *mapmodel.MapPoint, snapshot *mapmodel.MapSnapshot) {
	rooms := make(map[int]*mapmodel.MapRectangle)

	for cellIndex, value := range data {
		if value == protocol.MapV1CellLegacyFloor ||
			value == protocol.MapV1CellScan ||
			value&protocol.MapV1CellKindMask != protocol.MapV1CellScan {
			continue
		}

		roomID := int(value >> protocol.MapV1RoomIDShift)

		point := mapmodel.MapPoint{
			X: origin.X + float64(cellIndex%width)*protocol.MapV1Resolution,
			Y: origin.Y + float64(cellIndex/width)*protocol.MapV1Resolution,
		}

		if rooms[roomID] == nil {
			rooms[roomID] = &mapmodel.MapRectangle{
				Min: point,
				Max: point,
			}
		} else {
			rooms[roomID] = pointBounds([]mapmodel.MapPoint{
				rooms[roomID].Min,
				rooms[roomID].Max,
				point,
			})
		}
	}

	ids := make([]int, 0, len(rooms))
	for roomID := range rooms {
		ids = append(ids, roomID)
	}

	sort.Ints(ids)

	for _, roomID := range ids {
		snapshot.Rooms = append(snapshot.Rooms, mapmodel.MapRoom{
			Id:         pointer(int64(roomID)),
			Bounds:     rooms[roomID],
			Unit:       mapmodel.MapUnitMillimeters,
			Frame:      mapmodel.MapFrameV1World,
			CleanState: nil,
			Label:      nil,
			Material:   nil,
			Name:       nil,
			Outline:    nil,
			RoomType:   nil,
		})
	}
}

func v1BlockBounds(payload []byte) (int, int64, error) {
	header := int(binary.LittleEndian.Uint16(payload[protocol.MapV1HeaderLengthOffset:]))

	length := int64(binary.LittleEndian.Uint32(payload[protocol.MapBlockDataLengthOffset:]))

	if header < protocol.MapBlockHeaderMinimum || header > len(payload) || length > int64(len(payload)-header) {
		return 0, 0, fmt.Errorf("V1 block bounds: %w", ErrMalformed)
	}

	return header, length, nil
}
