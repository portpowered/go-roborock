package roborock

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/portpowered/go-roborock/internal/mapmodel"
	"github.com/portpowered/go-roborock/pkg/dependencies/mapdata"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

type mapTransport interface {
	FetchMapV1(ctx context.Context) ([]byte, error)
	CallB01(ctx context.Context, method dependencymodels.B01Method, params json.RawMessage) (json.RawMessage, error)
	FetchMapQ7(ctx context.Context, mapID int64, serial, model string) ([]byte, error)
	QueryMapListQ10(ctx context.Context) (dependencymodels.MapsQ10ListResult, error)
	FetchMapQ10(ctx context.Context) ([]byte, error)
	FetchTraceQ10(ctx context.Context) ([]byte, error)
	SetQ10Clean(ctx context.Context, command dependencymodels.Q10CleanCommand) error
}

type mapProvider struct{ mapTransport }

func unsupportedMap(operation string) error {
	return roborockerrors.New(roborockerrors.Unsupported, operation, "operation unsupported by device family", nil)
}

func (s *DeviceSession) mapsTransport(operation string) (*mapProvider, error) {
	transport, ok := s.rpc.(mapTransport)
	if !ok {
		return nil, unsupportedMap(operation)
	}

	return &mapProvider{mapTransport: transport}, nil
}

// GetMap retrieves map content without changing the active map.
// Q10 returns the next matching stream observation while the request is pending.
func (s *DeviceSession) GetMap(ctx context.Context, request GetMapRequest) (MapSnapshot, error) {
	ctx, finish := s.operationContext(ctx)
	defer finish()

	transport, err := s.mapsTransport("GetMap")
	if err != nil {
		return MapSnapshot{}, err
	}

	payload, format, err := s.fetchMap(ctx, transport, request)
	if err != nil {
		return MapSnapshot{}, operationError("GetMap", err)
	}

	return decodeMap(format, payload, "GetMap")
}

func (s *DeviceSession) fetchMap(
	ctx context.Context, transport *mapProvider, request GetMapRequest,
) ([]byte, mapmodel.MapFormat, error) {
	switch s.deviceFamily() {
	case FamilyV1Vacuum:
		if request.MapID != "" {
			return nil, mapmodel.MapFormatV1, unsupportedMap("GetMap explicit V1 map")
		}

		payload, err := transport.FetchMapV1(ctx)

		return payload, mapmodel.MapFormatV1, err
	case FamilyB01Q7:
		mapID, err := s.q7MapID(ctx, request.MapID)
		if err != nil {
			return nil, mapmodel.MapFormatQ7, err
		}

		payload, err := transport.FetchMapQ7(ctx, mapID, s.metadata.serial, s.metadata.model)

		return payload, mapmodel.MapFormatQ7, err
	case FamilyB01Q10:
		if request.MapID != "" {
			return nil, mapmodel.MapFormatQ10, unsupportedMap("GetMap explicit Q10 map")
		}

		payload, err := transport.FetchMapQ10(ctx)

		return payload, mapmodel.MapFormatQ10, err
	case FamilyDyad, FamilyZeo, FamilyUnknown:
		return nil, "", unsupportedMap("GetMap")
	default:
		return nil, "", unsupportedMap("GetMap")
	}
}

func decodeMap(format mapmodel.MapFormat, payload []byte, operation string) (MapSnapshot, error) {
	parsed, err := mapdata.Decode(format, payload)
	if err != nil {
		return MapSnapshot{}, roborockerrors.New(roborockerrors.Protocol, operation, "malformed device map", err)
	}

	return *parsed, nil
}

// GetMapTrace requests the next matching Q10 trace observation separately from map content.
// Q10 stream responses are uncorrelated; no cross-stream coherence is implied.
func (s *DeviceSession) GetMapTrace(ctx context.Context, _ EmptyRequest) (MapSnapshot, error) {
	ctx, finish := s.operationContext(ctx)
	defer finish()

	if s.deviceFamily() != FamilyB01Q10 {
		return MapSnapshot{}, unsupportedMap("GetMapTrace")
	}

	transport, err := s.mapsTransport("GetMapTrace")
	if err != nil {
		return MapSnapshot{}, err
	}

	payload, err := transport.FetchTraceQ10(ctx)
	if err != nil {
		return MapSnapshot{}, operationError("GetMapTrace", err)
	}

	return decodeMap(mapmodel.MapFormatQ10, payload, "GetMapTrace")
}

func (s *DeviceSession) q7MapID(ctx context.Context, requested string) (int64, error) {
	if requested != "" {
		return numericMapID(requested, "GetMap")
	}

	maps, err := s.ListMaps(ctx, EmptyRequest{})
	if err != nil {
		return 0, err
	}

	return currentQ7Map(maps.Maps)
}

func currentQ7Map(maps []MapInfo) (int64, error) {
	var selected *MapInfo

	for _, item := range maps {
		if item.Current != nil && *item.Current {
			if selected != nil {
				return 0, roborockerrors.New(roborockerrors.Protocol, "GetMap", "active map is ambiguous", nil)
			}

			selected = &item
		}
	}

	if selected != nil {
		return numericMapID(selected.ID, "GetMap")
	}

	return 0, roborockerrors.New(roborockerrors.Unavailable, "GetMap", "device did not identify its active map", nil)
}

func numericMapID(value, operation string) (int64, error) {
	identifier, err := strconv.ParseInt(value, 10, 64)
	if err != nil || identifier < 0 {
		return 0, roborockerrors.New(roborockerrors.InvalidArgument,
			operation, "map identifier is not a nonnegative decimal integer", err)
	}

	return identifier, nil
}

var _ MapOperations = (*DeviceSession)(nil)
