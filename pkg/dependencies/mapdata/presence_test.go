package mapdata_test

import (
	"testing"

	"github.com/portpowered/go-roborock/internal/mapmodel"
	"github.com/portpowered/go-roborock/pkg/dependencies/mapdata"
	"github.com/portpowered/go-roborock/pkg/dependencymodels/mapproto"
	"google.golang.org/protobuf/proto"
)

func TestQ7AbsentAndZeroGeometry(t *testing.T) {
	t.Parallel()

	wire := new(mapproto.RobotMap)
	data := payload(t, vectors(t)[1])

	err := proto.Unmarshal(data, wire)
	if err != nil {
		t.Fatal(err)
	}

	wire.HistoryPose = new(mapproto.DeviceHistoryPoseInfo)
	wire.HistoryPose.Points = []*mapproto.DevicePoseDataInfo{new(mapproto.DevicePoseDataInfo), zeroHistoryPoint()}
	wire.AreaInfo[0].Points = []*mapproto.DevicePointInfo{new(mapproto.DevicePointInfo), zeroPoint()}
	wire.RoomDataInfo[0].RoomNamePost = new(mapproto.DevicePointInfo)
	outline := new(mapproto.RoomOutlineInfo)
	outline.RoomId = proto.Uint32(0)
	emptyOutline := new(mapproto.RoomOutlinePointInfo)
	zeroOutline := new(mapproto.RoomOutlinePointInfo)
	zeroOutline.X = proto.Uint32(0)
	zeroOutline.Y = proto.Uint32(0)
	outline.Points = []*mapproto.RoomOutlinePointInfo{emptyOutline, zeroOutline}
	wire.RoomOutline = []*mapproto.RoomOutlineInfo{outline}

	encoded, err := proto.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := mapdata.Decode(mapmodel.MapFormatQ7, encoded)
	if err != nil {
		t.Fatal(err)
	}

	assertQ7GeometryPresence(t, snapshot)
}

func assertQ7GeometryPresence(t *testing.T, snapshot *mapmodel.MapSnapshot) {
	t.Helper()

	if len(snapshot.Paths) != 1 || len(snapshot.Paths[0].Points) != 1 || snapshot.Paths[0].Points[0].X != 0 {
		t.Fatal("absent path coordinates became zero geometry")
	}

	if len(snapshot.Restrictions[0].Points) != 1 || snapshot.Restrictions[0].Points[0].X != 0 {
		t.Fatal("absent area coordinates became zero geometry")
	}

	if snapshot.Rooms[0].Label != nil || snapshot.Rooms[0].Outline == nil || len(*snapshot.Rooms[0].Outline) != 1 {
		t.Fatal("absent room geometry became origin geometry")
	}
}

func zeroPoint() *mapproto.DevicePointInfo {
	point := new(mapproto.DevicePointInfo)
	point.X = proto.Float32(0)
	point.Y = proto.Float32(0)

	return point
}

func zeroHistoryPoint() *mapproto.DevicePoseDataInfo {
	point := new(mapproto.DevicePoseDataInfo)
	point.X = proto.Float32(0)
	point.Y = proto.Float32(0)

	return point
}

func TestQ10EmptyGeometryCollections(t *testing.T) {
	t.Parallel()
	data := payload(t, vectors(t)[2])

	snapshot, err := mapdata.Decode(mapmodel.MapFormatQ10, data)
	if err != nil {
		t.Fatal(err)
	}

	for _, present := range []bool{snapshot.Rooms != nil, snapshot.Paths != nil, snapshot.CurrentZones != nil,
		snapshot.Restrictions != nil, snapshot.Walls != nil, snapshot.UnknownBlocks != nil} {
		if !present {
			t.Fatal("empty geometry collection is null")
		}
	}

	for _, count := range []int{len(snapshot.Rooms), len(snapshot.Paths), len(snapshot.CurrentZones),
		len(snapshot.Restrictions), len(snapshot.Walls), len(snapshot.UnknownBlocks)} {
		if count != 0 {
			t.Fatal("no-feature map contains fabricated geometry")
		}
	}
}

func TestQ7RoomLookupBound(t *testing.T) {
	t.Parallel()

	wire := new(mapproto.RobotMap)

	err := proto.Unmarshal(payload(t, vectors(t)[1]), wire)
	if err != nil {
		t.Fatal(err)
	}

	wire.RoomDataInfo = nil

	for index := range uint32(20_000) {
		room := new(mapproto.RoomDataInfo)
		room.RoomId = proto.Uint32(index)
		wire.RoomDataInfo = append(wire.RoomDataInfo, room)
		outline := new(mapproto.RoomOutlineInfo)
		outline.RoomId = proto.Uint32(index)
		wire.RoomOutline = append(wire.RoomOutline, outline)
	}

	wire.RoomOutline = append(wire.RoomOutline, new(mapproto.RoomOutlineInfo))

	encoded, err := proto.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := mapdata.Decode(mapmodel.MapFormatQ7, encoded)
	if err != nil {
		t.Fatal(err)
	}

	if len(snapshot.Rooms) != 20_001 || snapshot.Rooms[20_000].Id != nil {
		t.Fatal("absent outline room ID incorrectly matched present zero ID")
	}
}
