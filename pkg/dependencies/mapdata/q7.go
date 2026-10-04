package mapdata

import (
	"fmt"

	"github.com/portpowered/go-roborock/internal/mapmodel"
	"github.com/portpowered/go-roborock/pkg/dependencymodels/mapproto"
	"google.golang.org/protobuf/proto"
)

func decodeQ7(payload []byte, snapshot *mapmodel.MapSnapshot) error {
	wire := new(mapproto.RobotMap)

	budget := new(protoBudget)

	err := budget.inspect(payload, wire.ProtoReflect().Descriptor(), 0)
	if err != nil {
		return err
	}

	options := new(proto.UnmarshalOptions)
	options.RecursionLimit = maxProtoUnmarshalDepth

	err = options.Unmarshal(payload, wire)
	if err != nil {
		return fmt.Errorf("Q7 protobuf: %w: %w", ErrMalformed, err)
	}

	grid, err := q7Grid(wire)
	if err != nil {
		return err
	}

	snapshot.Grid = grid
	q7Geometry(wire, snapshot)
	q7Rooms(wire, snapshot)

	return validatePoints(snapshot)
}

func q7Grid(wire *mapproto.RobotMap) (*mapmodel.MapGrid, error) {
	head := wire.GetMapHead()

	count, err := cells(int(head.GetSizeX()), int(head.GetSizeY()))
	if err != nil {
		return nil, err
	}

	data := wire.GetMapData().GetMapData()
	if len(data) < count {
		return nil, fmt.Errorf("Q7 grid truncated: %w", ErrMalformed)
	}

	grid := &mapmodel.MapGrid{
		Width:  int(head.GetSizeX()),
		Height: int(head.GetSizeY()),
		Cells: append([]byte(nil),
			data[:count]...),
		Unit:          mapmodel.MapUnitMeters,
		Frame:         mapmodel.MapFrameQ7World,
		TopDown:       false,
		RowYDirection: 1,
		Origin:        nil,
		Resolution:    nil,
		RoomCells:     nil,
	}
	if head.MinX != nil && head.MinY != nil {
		grid.Origin = &mapmodel.MapPoint{
			X: float64(head.GetMinX()),
			Y: float64(head.GetMinY()),
		}
	}

	if head.Resolution != nil {
		grid.Resolution = pointer(float64(head.GetResolution()))
	}

	err = validateQ7Calibration(grid)
	if err != nil {
		return nil, err
	}

	return grid, nil
}

func q7Geometry(wire *mapproto.RobotMap, snapshot *mapmodel.MapSnapshot) {
	snapshot.FormatId = optionalUint(wire.MapType)

	if wire.GetCurrentPose() != nil && wire.CurrentPose.X != nil && wire.CurrentPose.Y != nil {
		snapshot.Robot = q7Pose(wire.GetCurrentPose().GetX(), wire.GetCurrentPose().GetY(), wire.GetCurrentPose().Phi)
	}

	if wire.GetChargeStation() != nil && wire.ChargeStation.X != nil && wire.ChargeStation.Y != nil {
		snapshot.Dock = q7Pose(wire.GetChargeStation().GetX(), wire.GetChargeStation().GetY(), wire.GetChargeStation().Phi)
	}

	q7Paths(wire, snapshot)
	q7Areas(wire, snapshot)
}

func q7Paths(wire *mapproto.RobotMap, snapshot *mapmodel.MapSnapshot) {
	if wire.GetHistoryPose() != nil {
		path := mapmodel.MapPath{
			Kind:   mapmodel.MapPathKindClean,
			Unit:   mapmodel.MapUnitMeters,
			Frame:  mapmodel.MapFrameQ7World,
			Points: []mapmodel.MapPoint{},
		}

		for _, point := range wire.GetHistoryPose().GetPoints() {
			if point.X == nil || point.Y == nil {
				continue
			}

			path.Points = append(path.Points, mapmodel.MapPoint{
				X: float64(point.GetX()),
				Y: float64(point.GetY()),
			})
		}

		snapshot.Paths = append(snapshot.Paths, path)
	}
}

func q7Areas(wire *mapproto.RobotMap, snapshot *mapmodel.MapSnapshot) {
	for _, area := range wire.GetAreaInfo() {
		item := mapmodel.MapArea{
			Kind:         mapmodel.MapAreaKindUnknown,
			Unit:         mapmodel.MapUnitMeters,
			Frame:        mapmodel.MapFrameQ7World,
			SourceType:   optionalUint(area.Type),
			SourceIndex:  optionalUint(area.AreaIndex),
			SourceStatus: optionalUint(area.Status),
			Points:       []mapmodel.MapPoint{},
		}

		for _, point := range area.GetPoints() {
			if point.X == nil || point.Y == nil {
				continue
			}

			item.Points = append(item.Points, mapmodel.MapPoint{
				X: float64(point.GetX()),
				Y: float64(point.GetY()),
			})
		}

		snapshot.Restrictions = append(snapshot.Restrictions, item)
	}
}

func optionalUint(value *uint32) *int64 {
	if value == nil {
		return nil
	}

	return pointer(int64(*value))
}

func q7Pose(x, y float32, heading *float32) *mapmodel.MapPose {
	pose := &mapmodel.MapPose{
		Point: mapmodel.MapPoint{
			X: float64(x),
			Y: float64(y),
		},
		Unit:        mapmodel.MapUnitMeters,
		Frame:       mapmodel.MapFrameQ7World,
		Heading:     nil,
		HeadingUnit: nil,
	}
	if heading != nil {
		pose.Heading = pointer(float64(*heading))
		pose.HeadingUnit = pointer(mapmodel.Radians)
	}

	return pose
}

func q7Rooms(wire *mapproto.RobotMap, snapshot *mapmodel.MapSnapshot) {
	for _, metadata := range wire.GetRoomDataInfo() {
		room := mapmodel.MapRoom{
			Id:         optionalUint(metadata.RoomId),
			Name:       metadata.RoomName,
			RoomType:   optionalUint(metadata.RoomTypeId),
			Material:   optionalUint(metadata.MeterialId),
			CleanState: optionalUint(metadata.CleanState),
			Unit:       mapmodel.MapUnitMeters,
			Frame:      mapmodel.MapFrameQ7World,
			Bounds:     nil,
			Label:      nil,
			Outline:    nil,
		}
		if metadata.GetRoomNamePost() != nil && metadata.RoomNamePost.X != nil && metadata.RoomNamePost.Y != nil {
			room.Label = &mapmodel.MapPoint{
				X: float64(metadata.GetRoomNamePost().GetX()),
				Y: float64(metadata.GetRoomNamePost().GetY()),
			}
		}

		snapshot.Rooms = append(snapshot.Rooms, room)
	}

	q7Outlines(wire, snapshot)
}

func q7Outlines(wire *mapproto.RobotMap, snapshot *mapmodel.MapSnapshot) {
	indexes := q7RoomIndexes(snapshot.Rooms)

	for _, outline := range wire.GetRoomOutline() {
		index := -1

		if outline.RoomId != nil {
			if existing, exists := indexes[int64(outline.GetRoomId())]; exists {
				index = existing
			}
		}

		if index < 0 {
			snapshot.Rooms = append(snapshot.Rooms, mapmodel.MapRoom{
				Id:         optionalUint(outline.RoomId),
				Unit:       mapmodel.MapUnitMeters,
				Frame:      mapmodel.MapFrameQ7World,
				Bounds:     nil,
				CleanState: nil,
				Label:      nil,
				Material:   nil,
				Name:       nil,
				Outline:    nil,
				RoomType:   nil,
			})

			index = len(snapshot.Rooms) - 1
			if outline.RoomId != nil {
				indexes[int64(outline.GetRoomId())] = index
			}
		}

		if snapshot.Grid.Origin == nil || snapshot.Grid.Resolution == nil {
			continue
		}

		points := q7OutlinePoints(outline, snapshot.Grid)

		snapshot.Rooms[index].Outline = &points
		snapshot.Rooms[index].Bounds = pointBounds(points)
	}
}

func pointBounds(points []mapmodel.MapPoint) *mapmodel.MapRectangle {
	if len(points) == 0 {
		return nil
	}

	result := &mapmodel.MapRectangle{
		Min: points[0],
		Max: points[0],
	}
	for _, point := range points {
		if point.X < result.Min.X {
			result.Min.X = point.X
		}

		if point.Y < result.Min.Y {
			result.Min.Y = point.Y
		}

		if point.X > result.Max.X {
			result.Max.X = point.X
		}

		if point.Y > result.Max.Y {
			result.Max.Y = point.Y
		}
	}

	return result
}

func validatePoints(snapshot *mapmodel.MapSnapshot) error {
	points := []mapmodel.MapPoint{}

	posePoints, err := validatePoses(snapshot)
	if err != nil {
		return err
	}

	points = append(points, posePoints...)
	for _, path := range snapshot.Paths {
		points = append(points, path.Points...)
	}

	for _, area := range snapshot.Restrictions {
		points = append(points, area.Points...)
	}

	for _, room := range snapshot.Rooms {
		if room.Label != nil {
			points = append(points, *room.Label)
		}

		if room.Outline != nil {
			points = append(points, (*room.Outline)...)
		}
	}

	for _, point := range points {
		if !finite(point.X) || !finite(point.Y) {
			return fmt.Errorf("non-finite geometry: %w", ErrMalformed)
		}
	}

	return nil
}

func validatePoses(snapshot *mapmodel.MapSnapshot) ([]mapmodel.MapPoint, error) {
	points := []mapmodel.MapPoint{}

	for _, pose := range []*mapmodel.MapPose{
		snapshot.Robot,
		snapshot.Dock,
	} {
		if pose != nil {
			points = append(points, pose.Point)

			if pose.Heading != nil && !finite(*pose.Heading) {
				return nil, fmt.Errorf("non-finite heading: %w", ErrMalformed)
			}
		}
	}

	return points, nil
}

func validateQ7Calibration(grid *mapmodel.MapGrid) error {
	if grid.Resolution != nil && (!finite(*grid.Resolution) || *grid.Resolution <= 0) {
		return fmt.Errorf("Q7 resolution: %w", ErrMalformed)
	}

	if grid.Origin != nil && (!finite(grid.Origin.X) || !finite(grid.Origin.Y)) {
		return fmt.Errorf("Q7 origin: %w", ErrMalformed)
	}

	return nil
}

func q7RoomIndexes(rooms []mapmodel.MapRoom) map[int64]int {
	indexes := make(map[int64]int, len(rooms))

	for index, room := range rooms {
		if room.Id != nil {
			if _, exists := indexes[*room.Id]; !exists {
				indexes[*room.Id] = index
			}
		}
	}

	return indexes
}

func q7OutlinePoints(outline *mapproto.RoomOutlineInfo, grid *mapmodel.MapGrid) []mapmodel.MapPoint {
	points := make([]mapmodel.MapPoint, 0, len(outline.GetPoints()))

	for _, point := range outline.GetPoints() {
		if point.X == nil || point.Y == nil {
			continue
		}

		points = append(points, mapmodel.MapPoint{
			X: grid.Origin.X + float64(point.GetX())**grid.Resolution,
			Y: grid.Origin.Y + float64(point.GetY())**grid.Resolution,
		})
	}

	return points
}
