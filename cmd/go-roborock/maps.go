package main

import (
	"context"
	"fmt"

	"github.com/portpowered/go-roborock/pkg/roborock"
)

func isMapCommand(command string) bool {
	switch command {
	case commandMaps, commandMap, commandRooms, commandTrace, commandCapabilities,
		commandSelectMap, commandCleanZones, commandCleanRooms:
		return true
	default:
		return false
	}
}

//nolint:wrapcheck // SDK map and control errors already retain the operation, kind, and vendor code.
func mapOperation(
	ctx context.Context,
	session *roborock.DeviceSession,
	command string,
	input CommandInput,
) (any, error) {
	request := roborock.EmptyRequest{}

	switch command {
	case commandMaps:
		return session.ListMaps(ctx, request)
	case commandMap:
		return session.GetMap(ctx, roborock.GetMapRequest{MapID: input.MapID})
	case commandRooms:
		return session.GetRooms(ctx, request)
	case commandTrace:
		return session.GetMapTrace(ctx, request)
	case commandCapabilities:
		return session.GetCapabilities(ctx, request)
	default:
		return mapControl(ctx, session, command, input)
	}
}

//nolint:wrapcheck // SDK errors retain typed operation and vendor diagnostics.
func mapControl(ctx context.Context, session *roborock.DeviceSession, command string, input CommandInput) (any, error) {
	switch command {
	case commandSelectMap:
		return session.SelectMap(ctx, roborock.SelectMapRequest{MapID: input.MapID})
	case commandCleanRooms:
		return session.CleanSegments(ctx, input.CleanRooms)
	case commandCleanZones:
		zones, err := cleanZonesRequest(input.CleanZones)
		if err != nil {
			return nil, err
		}

		return session.CleanZones(ctx, zones)
	default:
		return nil, fmt.Errorf("%w: %q", errUnknownCommand, command)
	}
}
func cleanZonesRequest(input *CleanZonesInput) (roborock.CleanZonesRequest, error) {
	result := roborock.CleanZonesRequest{Zones: nil}
	if input == nil || len(input.Zones) == 0 {
		return result, errCleaningZones
	}

	for _, zone := range input.Zones {
		if !hasZoneCoordinates(zone) {
			return result, errCleaningZones
		}

		if *zone.X1 >= *zone.X2 || *zone.Y1 >= *zone.Y2 || zone.Repeats < 1 || zone.Repeats > 3 {
			return result, errCleaningZones
		}

		result.Zones = append(result.Zones, roborock.Zone{
			X1: *zone.X1, Y1: *zone.Y1, X2: *zone.X2, Y2: *zone.Y2, Repeats: zone.Repeats,
		})
	}

	return result, nil
}

func hasZoneCoordinates(zone CleaningZoneInput) bool {
	return zone.X1 != nil && zone.Y1 != nil && zone.X2 != nil && zone.Y2 != nil
}
