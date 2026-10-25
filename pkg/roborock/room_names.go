package roborock

import (
	"context"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// NameSegments sends the complete current-map cloud-room bindings to a V1 vacuum.
// Preserve unchanged segments and tags. This does not create or rename cloud directory entries,
// select a map, retry an uncertain command, or confirm names. Read back the mapping and directory.
func (s *DeviceSession) NameSegments(ctx context.Context, request NameSegmentsRequest) (CommandAcknowledgement, error) {
	ctx, finish := s.operationContext(ctx)
	defer finish()

	if s.deviceFamily() != FamilyV1Vacuum {
		return CommandAcknowledgement{}, unsupportedMap("NameSegments")
	}

	if len(request.Mappings) == 0 {
		return CommandAcknowledgement{}, roborockerrors.New(roborockerrors.InvalidArgument,
			"NameSegments", "complete room mapping required", nil)
	}

	seen := make(map[int64]struct{}, len(request.Mappings))
	parameters := make(dependencymodels.MapsV1NameSegmentsRequest, 0, len(request.Mappings))

	for _, mapping := range request.Mappings {
		if _, duplicate := seen[mapping.SegmentID]; duplicate {
			return CommandAcknowledgement{}, roborockerrors.New(roborockerrors.InvalidArgument,
				"NameSegments", "duplicate segment ID", nil)
		}

		seen[mapping.SegmentID] = struct{}{}

		parameters = append(parameters, dependencymodels.MapsV1RoomName{
			IotRoomId: mapping.CloudRoomID, RobotRoomId: mapping.SegmentID, RobotTagId: mapping.RobotTagID,
		})
	}

	return s.command(ctx, dependencymodels.RPCMethod(protocol.RPCNameSegment), parameters)
}
