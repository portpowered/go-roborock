package roborock

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/portpowered/go-roborock/internal/protocol"

	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

var errMalformedMapList = errors.New("device map list does not match its known response shape")

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

	err = decodeKnownMapList(raw, &list)
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

	err = decodeKnownMapList(raw, &lists)
	if err != nil {
		return result, roborockerrors.New(roborockerrors.Protocol, "ListMaps", "malformed V1 map list", err)
	}

	if len(lists) == 0 {
		return result, roborockerrors.New(roborockerrors.Protocol, "ListMaps", "empty V1 map list response",
			errMalformedMapList)
	}

	for _, list := range lists {
		for _, entry := range valueOrZero(list.MapInfo) {
			name := entry.Name
			result.Maps = append(result.Maps, MapInfo{ID: strconv.FormatInt(entry.MapFlag, 10), Name: &name, Current: nil})
		}
	}

	return result, nil
}

func decodeKnownMapList(raw json.RawMessage, target any) error {
	err := json.Unmarshal(raw, target)
	if err != nil {
		return fmt.Errorf("decode device map list: %w", err)
	}

	return validateMapListShape(raw, reflect.TypeOf(target).Elem())
}

func validateMapListShape(raw json.RawMessage, model reflect.Type) error {
	for model.Kind() == reflect.Pointer {
		model = model.Elem()
	}

	if model == reflect.TypeFor[json.RawMessage]() {
		return nil
	}

	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errMalformedMapList
	}

	switch {
	case model.Kind() == reflect.Struct:
		return validateMapListObject(raw, model)
	case model.Kind() == reflect.Slice:
		var values []json.RawMessage

		err := json.Unmarshal(raw, &values)
		if err != nil {
			return fmt.Errorf("decode map list sequence: %w", err)
		}

		for _, value := range values {
			err = validateMapListShape(value, model.Elem())
			if err != nil {
				return err
			}
		}
	default:
	}

	return nil
}

func validateMapListObject(raw json.RawMessage, model reflect.Type) error {
	var object map[string]json.RawMessage

	err := json.Unmarshal(raw, &object)
	if err != nil {
		return fmt.Errorf("decode map list object: %w", err)
	}

	for index := range model.NumField() {
		field := model.Field(index)

		tag := field.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}

		parts := strings.Split(tag, ",")

		value, present := object[parts[0]]
		if !present {
			if !strings.Contains(tag, ",omitempty") {
				return fmt.Errorf("%w: missing %s", errMalformedMapList, parts[0])
			}

			continue
		}

		err = validateMapListShape(value, field.Type)
		if err != nil {
			return err
		}
	}

	return nil
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
