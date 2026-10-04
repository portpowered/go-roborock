package mapdata

import (
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/portpowered/go-roborock/internal/mapmodel"
	"github.com/portpowered/go-roborock/internal/protocol"
)

func decodeV1(payload []byte, snapshot *mapmodel.MapSnapshot) error {
	if len(payload) < protocol.MapV1HeaderMinimum || string(payload[:2]) != protocol.MapV1Magic {
		return fmt.Errorf("V1 header: %w", ErrMalformed)
	}
	start := int(binary.LittleEndian.Uint16(payload[2:4]))
	if start < protocol.MapV1HeaderMinimum || start > len(payload) {
		return fmt.Errorf("V1 header length: %w", ErrMalformed)
	}
	snapshot.MapId = pointer(int64(binary.LittleEndian.Uint32(payload[12:16])))
	for start < len(payload) {
		if len(payload)-start < protocol.MapBlockHeaderMinimum {
			return fmt.Errorf("V1 truncated block header: %w", ErrMalformed)
		}
		header := int(binary.LittleEndian.Uint16(payload[start+2 : start+4]))
		length := int64(binary.LittleEndian.Uint32(payload[start+4 : start+8]))
		if header < protocol.MapBlockHeaderMinimum || header > len(payload)-start || length > int64(len(payload)-start-header) {
			return fmt.Errorf("V1 block bounds: %w", ErrMalformed)
		}
		kind := binary.LittleEndian.Uint16(payload[start : start+2])
		block := payload[start : start+header]
		data := payload[start+header : start+header+int(length)]
		if err := v1Block(kind, block, data, start, snapshot); err != nil {
			return err
		}
		start += header + int(length)
	}
	return nil
}

func v1Block(kind uint16, header, data []byte, offset int, snapshot *mapmodel.MapSnapshot) error {
	switch kind {
	case protocol.MapBlockImage:
		return v1Grid(header, data, snapshot)
	case protocol.MapBlockCharger, protocol.MapBlockRobot:
		if len(data) != 8 && len(data) != 12 {
			return fmt.Errorf("V1 pose length: %w", ErrMalformed)
		}
		pose := &mapmodel.MapPose{Point: mapmodel.MapPoint{X: float64(binary.LittleEndian.Uint32(data[:4])), Y: float64(binary.LittleEndian.Uint32(data[4:8]))}, Unit: mapmodel.MapUnitMillimeters, Frame: mapmodel.MapFrameV1World}
		if len(data) == 12 {
			angle := int64(binary.LittleEndian.Uint32(data[8:12]))
			if angle > 255 {
				angle = (angle & 255) - 256
			}
			pose.Heading = pointer(float64(angle))
			pose.HeadingUnit = pointer(mapmodel.Degrees)
		}
		if kind == protocol.MapBlockRobot {
			snapshot.Robot = pose
		} else {
			snapshot.Dock = pose
		}
	case protocol.MapBlockPath, protocol.MapBlockGotoPath, protocol.MapBlockPredictedPath:
		if len(header) < 20 || len(data)%4 != 0 {
			return fmt.Errorf("V1 path length: %w", ErrMalformed)
		}
		name := "clean"
		if kind == protocol.MapBlockGotoPath {
			name = "goto"
		}
		if kind == protocol.MapBlockPredictedPath {
			name = "predicted"
		}
		snapshot.Paths = append(snapshot.Paths, mapmodel.MapPath{Kind: name, Points: v1Points(data), Unit: mapmodel.MapUnitMillimeters, Frame: mapmodel.MapFrameV1World})
	case protocol.MapBlockCurrentZones, protocol.MapBlockWalls, protocol.MapBlockNoGo, protocol.MapBlockNoMop, protocol.MapBlockNoCarpet:
		return v1Areas(kind, header, data, snapshot)
	default:
		snapshot.UnknownBlocks = append(snapshot.UnknownBlocks, mapmodel.MapUnknownBlock{Type: int64(kind), Offset: offset, HeaderLength: len(header), DataLength: len(data)})
	}
	return nil
}

func v1Points(data []byte) []mapmodel.MapPoint {
	points := make([]mapmodel.MapPoint, 0, len(data)/4)
	for start := 0; start < len(data); start += 4 {
		points = append(points, mapmodel.MapPoint{X: float64(binary.LittleEndian.Uint16(data[start : start+2])), Y: float64(binary.LittleEndian.Uint16(data[start+2 : start+4]))})
	}
	return points
}

func v1Areas(kind uint16, header, data []byte, snapshot *mapmodel.MapSnapshot) error {
	if len(header) < 10 {
		return fmt.Errorf("V1 area count missing: %w", ErrMalformed)
	}
	stride := 8
	name := "current"
	switch kind {
	case protocol.MapBlockWalls:
		name = "wall"
	case protocol.MapBlockNoGo:
		name = "no-go"
		stride = 16
	case protocol.MapBlockNoMop:
		name = "no-mop"
		stride = 16
	case protocol.MapBlockNoCarpet:
		name = "no-carpet"
		stride = 16
	}
	count := int(binary.LittleEndian.Uint16(header[8:10]))
	if count*stride != len(data) {
		return fmt.Errorf("V1 area records: %w", ErrMalformed)
	}
	for start := 0; start < len(data); start += stride {
		area := mapmodel.MapArea{Kind: name, Points: v1Points(data[start : start+stride]), Unit: mapmodel.MapUnitMillimeters, Frame: mapmodel.MapFrameV1World}
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
	if len(header) < 24 {
		return fmt.Errorf("V1 image header: %w", ErrMalformed)
	}
	fields := header[len(header)-16:]
	width, height := int(binary.LittleEndian.Uint32(fields[12:16])), int(binary.LittleEndian.Uint32(fields[8:12]))
	count, err := cells(width, height)
	if err != nil {
		return err
	}
	if len(data) != count {
		return fmt.Errorf("V1 image length: %w", ErrMalformed)
	}
	origin := &mapmodel.MapPoint{X: float64(binary.LittleEndian.Uint32(fields[4:8])) * 50, Y: float64(binary.LittleEndian.Uint32(fields[:4])) * 50}
	grid := &mapmodel.MapGrid{Width: width, Height: height, Cells: append([]byte(nil), data...), Origin: origin, Resolution: pointer(50.0), Unit: mapmodel.MapUnitMillimeters, Frame: mapmodel.MapFrameV1World, TopDown: false, RowYDirection: 1}
	snapshot.Grid = grid
	rooms := make(map[int]*mapmodel.MapRectangle)
	for i, value := range data {
		if value == 255 || value == 7 || value&7 != 7 {
			continue
		}
		id := int(value >> 3)
		point := mapmodel.MapPoint{X: origin.X + float64(i%width)*50, Y: origin.Y + float64(i/width)*50}
		if rooms[id] == nil {
			rooms[id] = &mapmodel.MapRectangle{Min: point, Max: point}
		} else {
			rooms[id] = pointBounds([]mapmodel.MapPoint{rooms[id].Min, rooms[id].Max, point})
		}
	}
	ids := make([]int, 0, len(rooms))
	for id := range rooms {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		snapshot.Rooms = append(snapshot.Rooms, mapmodel.MapRoom{Id: pointer(int64(id)), Bounds: rooms[id], Unit: mapmodel.MapUnitMillimeters, Frame: mapmodel.MapFrameV1World})
	}
	return nil
}
