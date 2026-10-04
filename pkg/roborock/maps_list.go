package roborock

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/portpowered/go-roborock/internal/protocol"

	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// ListMaps retrieves saved-map metadata. Q10 list order does not imply an active map.
func (s *DeviceSession) ListMaps(ctx context.Context, _ EmptyRequest) (ListMapsResult, error) {
	ctx, finish := s.operationContext(ctx)
	defer finish()

	err := ctx.Err()
	if err != nil {
		return ListMapsResult{}, operationError("ListMaps", err)
	}

	result := ListMapsResult{Maps: []MapInfo{}}

	switch s.deviceFamily() {
	case FamilyV1Vacuum:
		return s.listV1Maps(ctx)
	case FamilyB01Q7:
		return s.listQ7Maps(ctx)
	case FamilyB01Q10:
		return s.listQ10Maps(ctx)
	case FamilyDyad, FamilyZeo, FamilyUnknown:
		return result, unsupportedMap("ListMaps")
	default:
		return result, unsupportedMap("ListMaps")
	}
}

func (s *DeviceSession) listQ7Maps(ctx context.Context) (ListMapsResult, error) {
	result := ListMapsResult{Maps: []MapInfo{}}

	transport, err := s.mapsTransport("ListMaps")
	if err != nil {
		return result, err
	}

	parameters, err := json.Marshal(dependencymodels.MapsQ7ListRequest{})
	if err != nil {
		return result, operationError("ListMaps", err)
	}

	raw, err := transport.CallB01(ctx, dependencymodels.ServiceGetMapList, parameters)
	if err != nil {
		return result, operationError("ListMaps", err)
	}

	var list dependencymodels.MapsQ7ListResult

	err = json.Unmarshal(raw, &list)
	if err != nil {
		return result, roborockerrors.New(roborockerrors.Protocol, "ListMaps", "malformed Q7 map list", err)
	}

	for _, entry := range valueOrZero(list.MapList) {
		if entry.Id != nil {
			result.Maps = append(result.Maps, MapInfo{ID: strconv.FormatInt(*entry.Id, 10), Current: entry.Cur, Name: nil})
		}
	}

	return result, nil
}

func (s *DeviceSession) listQ10Maps(ctx context.Context) (ListMapsResult, error) {
	result := ListMapsResult{Maps: []MapInfo{}}

	transport, err := s.mapsTransport("ListMaps")
	if err != nil {
		return result, err
	}

	list, err := transport.QueryMapListQ10(ctx)
	if err != nil {
		return result, operationError("ListMaps", err)
	}

	for _, entry := range valueOrZero(list.Data) {
		result.Maps = append(result.Maps, MapInfo{ID: entry.Id, Name: entry.Name, Current: nil})
	}

	return result, nil
}

func (s *DeviceSession) listV1Maps(ctx context.Context) (ListMapsResult, error) {
	result := ListMapsResult{Maps: []MapInfo{}}

	raw, err := s.callV1(ctx, dependencymodels.RPCMethod(protocol.RPCGetMultiMapsList), dependencymodels.NoParameters{})
	if err != nil {
		return result, err
	}

	var lists dependencymodels.MapsV1ListResult

	err = json.Unmarshal(raw, &lists)
	if err != nil {
		return result, roborockerrors.New(roborockerrors.Protocol, "ListMaps", "malformed V1 map list", err)
	}

	for _, list := range lists {
		for _, entry := range valueOrZero(list.MapInfo) {
			name := entry.Name
			result.Maps = append(result.Maps, MapInfo{ID: strconv.FormatInt(entry.MapFlag, 10), Name: &name, Current: nil})
		}
	}

	return result, nil
}

// SelectMap explicitly changes the V1 active map. Reads never select maps implicitly.
func (s *DeviceSession) SelectMap(ctx context.Context, request SelectMapRequest) (CommandAcknowledgement, error) {
	ctx, finish := s.operationContext(ctx)
	defer finish()

	if s.deviceFamily() != FamilyV1Vacuum {
		return CommandAcknowledgement{}, unsupportedMap("SelectMap")
	}

	identifier, err := numericMapID(request.MapID, "SelectMap")
	if err != nil {
		return CommandAcknowledgement{}, err
	}

	return s.command(ctx, dependencymodels.RPCMethod(protocol.RPCLoadMultiMap),
		dependencymodels.MapsV1LoadRequest{identifier})
}
