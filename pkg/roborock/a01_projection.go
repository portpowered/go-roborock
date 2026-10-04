package roborock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

var (
	errA01NoProperties = errors.New("at least one property is required")

	errA01InvalidProperty = errors.New("unknown, unsupported, or repeated property")

	errA01NullProperty = errors.New("appliance property cannot be null")
)

// a01Fields derives numeric keys from generated wire models and semantic keys
// from generated public projections. The shared Go field name joins the models.
// Contract tests keep this mapping aligned with both checked-in schemas.
func a01Fields[Projection, Wire any]() map[int]string {
	projection := reflect.TypeFor[Projection]()

	wire := reflect.TypeFor[Wire]()

	fields := make(map[int]string, wire.NumField())

	for i := range wire.NumField() {
		field := wire.Field(i)

		publicField, exists := projection.FieldByName(field.Name)

		if !exists {
			continue
		}

		propertyID, err := strconv.Atoi(strings.Split(field.Tag.Get("json"), ",")[0])
		if err == nil {
			fields[propertyID] = strings.Split(publicField.Tag.Get("json"), ",")[0]
		}
	}

	return fields
}

func queryA01State[Projection, Wire any, Property ~int](
	ctx context.Context, session *DeviceSession, properties []Property, operation string, state *Projection,
) error {
	err := session.requireA01(operation)
	if err != nil {
		return fmt.Errorf("adapt A01 property: %w", err)
	}

	fields := a01Fields[Projection, Wire]()

	ids, err := validateA01Properties(properties, fields)
	if err != nil {
		return roborockerrors.New(roborockerrors.InvalidArgument, operation, "invalid property selection", err)
	}

	values, err := session.rpc.QueryA01(ctx, ids)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	// Decode only selected values: unsolicited data does not change this request's result.
	selected := make(map[int]json.RawMessage, len(ids))

	for _, propertyID := range ids {
		value, exists := values[propertyID]

		if !exists {
			return roborockerrors.New(roborockerrors.Protocol, operation, "requested property is missing", nil)
		}

		selected[propertyID] = value
	}

	err = decodeA01[Projection, Wire](selected, state)
	if err != nil {
		return roborockerrors.New(roborockerrors.Protocol, operation, "invalid appliance property value", err)
	}

	return nil
}

func validateA01Properties[Property ~int](properties []Property, fields map[int]string) ([]int, error) {
	if len(properties) == 0 {
		return nil, errA01NoProperties
	}

	ids := make([]int, len(properties))

	seen := make(map[int]bool, len(properties))

	for position, property := range properties {
		propertyID := int(property)

		if _, known := fields[propertyID]; !known || seen[propertyID] {
			return nil, errA01InvalidProperty
		}

		ids[position], seen[propertyID] = propertyID, true
	}

	return ids, nil
}

func decodeA01[Projection, Wire any](values map[int]json.RawMessage, state *Projection) error {
	fields := a01Fields[Projection, Wire]()

	semantic := make(map[string]json.RawMessage, len(values))

	wireValues := make(map[string]json.RawMessage, len(values))

	for propertyID, value := range values {
		if string(value) == "null" || strings.TrimSpace(string(value)) == "null" {
			return errA01NullProperty
		}

		wireValues[strconv.Itoa(propertyID)] = value

		semantic[fields[propertyID]] = value
	}
	// Validate wire types before adaptation, including encoded string-valued fields.
	body, err := json.Marshal(wireValues)
	if err != nil {
		return fmt.Errorf("adapt A01 property: %w", err)
	}

	var wire Wire

	err = json.Unmarshal(body, &wire)
	if err != nil {
		return fmt.Errorf("adapt A01 property: %w", err)
	}

	err = adaptA01Durations[Projection](semantic)
	if err != nil {
		return fmt.Errorf("adapt A01 property: %w", err)
	}

	body, err = json.Marshal(semantic)
	if err != nil {
		return fmt.Errorf("adapt A01 property: %w", err)
	}

	err = json.Unmarshal(body, state)
	if err != nil {
		return fmt.Errorf("decode A01 state: %w", err)
	}

	return nil
}

func adaptA01Durations[Projection any](semantic map[string]json.RawMessage) error {
	// This field is the sole string-encoded scalar projection in the pinned Dyad contract.
	field, exists := reflect.TypeFor[Projection]().FieldByName("RecentRunTime")

	if !exists {
		return nil
	}

	key := strings.Split(field.Tag.Get("json"), ",")[0]

	raw, present := semantic[key]

	if !present {
		return nil
	}

	var encoded string

	err := json.Unmarshal(raw, &encoded)
	if err != nil {
		return fmt.Errorf("adapt A01 property: %w", err)
	}

	durations := make([]int, 0)

	if encoded != "" {
		for _, token := range strings.Split(encoded, ",") {
			minutes, err := strconv.Atoi(token)
			if err != nil {
				return fmt.Errorf("adapt A01 property: %w", err)
			}

			durations = append(durations, minutes)
		}
	}

	result, err := json.Marshal(durations)
	if err != nil {
		return fmt.Errorf("adapt A01 property: %w", err)
	}

	semantic[key] = result

	return nil
}

func encodeA01[Projection, Wire any](request Projection) (map[int]json.RawMessage, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode A01 settings: %w", err)
	}

	var semantic map[string]json.RawMessage

	err = json.Unmarshal(body, &semantic)
	if err != nil {
		return nil, fmt.Errorf("encode A01 settings: %w", err)
	}

	values := make(map[int]json.RawMessage, len(semantic))

	for propertyID, key := range a01Fields[Projection, Wire]() {
		if value, present := semantic[key]; present {
			values[propertyID] = value
		}
	}

	return values, nil
}

func setA01Settings[Projection, Wire any](
	ctx context.Context, session *DeviceSession, request Projection, operation string,
) error {
	err := session.requireA01(operation)
	if err != nil {
		return fmt.Errorf("adapt A01 property: %w", err)
	}

	values, err := encodeA01[Projection, Wire](request)

	if err != nil || len(values) == 0 {
		return roborockerrors.New(roborockerrors.InvalidArgument, operation, "at least one setting is required", err)
	}

	return publishA01(ctx, session, values, operation)
}

func publishA01(ctx context.Context, session *DeviceSession, values map[int]json.RawMessage, operation string) error {
	err := session.rpc.SetA01(ctx, values)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	return nil
}
