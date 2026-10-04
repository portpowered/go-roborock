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
	if err := budget.inspect(payload, wire.ProtoReflect().Descriptor(), 0); err != nil {
		return err
	}
	if err := (proto.UnmarshalOptions{RecursionLimit: 64}).Unmarshal(payload, wire); err != nil {
		return fmt.Errorf("Q7 protobuf: %w: %w", ErrMalformed, err)
	}
	head := wire.GetMapHead()
	count, err := cells(int(head.GetSizeX()), int(head.GetSizeY()))
	if err != nil {
		return err
	}
	data := wire.GetMapData().GetMapData()
	if len(data) < count {
		return fmt.Errorf("Q7 grid truncated: %w", ErrMalformed)
	}
	grid := &mapmodel.MapGrid{Width: int(head.GetSizeX()), Height: int(head.GetSizeY()), Cells: append([]byte(nil), data[:count]...),
		Unit: mapmodel.MapUnitMeters, Frame: mapmodel.MapFrameQ7World, TopDown: false, RowYDirection: 1}
	if head.MinX != nil && head.MinY != nil {
		grid.Origin = &mapmodel.MapPoint{X: float64(head.GetMinX()), Y: float64(head.GetMinY())}
	}
	if head.Resolution != nil {
		grid.Resolution = pointer(float64(head.GetResolution()))
	}
	if grid.Resolution != nil && (!finite(*grid.Resolution) || *grid.Resolution <= 0) {
		return fmt.Errorf("Q7 resolution: %w", ErrMalformed)
	}
	if grid.Origin != nil && (!finite(grid.Origin.X) || !finite(grid.Origin.Y)) {
		return fmt.Errorf("Q7 origin: %w", ErrMalformed)
	}
	snapshot.Grid = grid
	snapshot.FormatId = optionalUint(wire.MapType)
	if wire.CurrentPose != nil && wire.CurrentPose.X != nil && wire.CurrentPose.Y != nil {
		snapshot.Robot = q7Pose(wire.CurrentPose.GetX(), wire.CurrentPose.GetY(), wire.CurrentPose.Phi)
	}
	if wire.ChargeStation != nil && wire.ChargeStation.X != nil && wire.ChargeStation.Y != nil {
		snapshot.Dock = q7Pose(wire.ChargeStation.GetX(), wire.ChargeStation.GetY(), wire.ChargeStation.Phi)
	}
	if wire.HistoryPose != nil {
		path := mapmodel.MapPath{Kind: "clean", Unit: mapmodel.MapUnitMeters, Frame: mapmodel.MapFrameQ7World, Points: []mapmodel.MapPoint{}}
		for _, point := range wire.HistoryPose.Points {
			path.Points = append(path.Points, mapmodel.MapPoint{X: float64(point.GetX()), Y: float64(point.GetY())})
		}
		snapshot.Paths = append(snapshot.Paths, path)
	}
	for _, area := range wire.AreaInfo {
		item := mapmodel.MapArea{Kind: "unknown", Unit: mapmodel.MapUnitMeters, Frame: mapmodel.MapFrameQ7World,
			SourceType: optionalUint(area.Type), SourceIndex: optionalUint(area.AreaIndex), SourceStatus: optionalUint(area.Status), Points: []mapmodel.MapPoint{}}
		for _, point := range area.Points {
			item.Points = append(item.Points, mapmodel.MapPoint{X: float64(point.GetX()), Y: float64(point.GetY())})
		}
		snapshot.Restrictions = append(snapshot.Restrictions, item)
	}
	q7Rooms(wire, snapshot)
	return validatePoints(snapshot)
}

func optionalUint(value *uint32) *int64 {
	if value == nil {
		return nil
	}
	return pointer(int64(*value))
}

func q7Pose(x, y float32, heading *float32) *mapmodel.MapPose {
	pose := &mapmodel.MapPose{Point: mapmodel.MapPoint{X: float64(x), Y: float64(y)}, Unit: mapmodel.MapUnitMeters, Frame: mapmodel.MapFrameQ7World}
	if heading != nil {
		pose.Heading = pointer(float64(*heading))
		pose.HeadingUnit = pointer(mapmodel.Radians)
	}
	return pose
}

func q7Rooms(wire *mapproto.RobotMap, snapshot *mapmodel.MapSnapshot) {
	for _, metadata := range wire.RoomDataInfo {
		room := mapmodel.MapRoom{Id: optionalUint(metadata.RoomId), Name: metadata.RoomName, RoomType: optionalUint(metadata.RoomTypeId),
			Material: optionalUint(metadata.MeterialId), CleanState: optionalUint(metadata.CleanState), Unit: mapmodel.MapUnitMeters, Frame: mapmodel.MapFrameQ7World}
		if metadata.RoomNamePost != nil {
			room.Label = &mapmodel.MapPoint{X: float64(metadata.RoomNamePost.GetX()), Y: float64(metadata.RoomNamePost.GetY())}
		}
		snapshot.Rooms = append(snapshot.Rooms, room)
	}
	for _, outline := range wire.RoomOutline {
		index := -1
		for i, room := range snapshot.Rooms {
			if room.Id != nil && *room.Id == int64(outline.GetRoomId()) {
				index = i
				break
			}
		}
		if index < 0 {
			snapshot.Rooms = append(snapshot.Rooms, mapmodel.MapRoom{Id: optionalUint(outline.RoomId), Unit: mapmodel.MapUnitMeters, Frame: mapmodel.MapFrameQ7World})
			index = len(snapshot.Rooms) - 1
		}
		if snapshot.Grid.Origin == nil || snapshot.Grid.Resolution == nil {
			continue
		}
		points := make([]mapmodel.MapPoint, 0, len(outline.Points))
		for _, point := range outline.Points {
			points = append(points, mapmodel.MapPoint{X: snapshot.Grid.Origin.X + float64(point.GetX())**snapshot.Grid.Resolution, Y: snapshot.Grid.Origin.Y + float64(point.GetY())**snapshot.Grid.Resolution})
		}
		snapshot.Rooms[index].Outline = &points
		snapshot.Rooms[index].Bounds = pointBounds(points)
	}
}

func pointBounds(points []mapmodel.MapPoint) *mapmodel.MapRectangle {
	if len(points) == 0 {
		return nil
	}
	result := &mapmodel.MapRectangle{Min: points[0], Max: points[0]}
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
	for _, pose := range []*mapmodel.MapPose{snapshot.Robot, snapshot.Dock} {
		if pose != nil {
			points = append(points, pose.Point)
			if pose.Heading != nil && !finite(*pose.Heading) {
				return fmt.Errorf("non-finite heading: %w", ErrMalformed)
			}
		}
	}
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
