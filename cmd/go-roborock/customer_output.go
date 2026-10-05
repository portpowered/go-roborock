package main

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"text/tabwriter"

	"github.com/portpowered/go-roborock/pkg/roborock"
)

const tableCellPadding = 2

func writeCustomerResult(stdout io.Writer, command string, result any, asJSON bool) error {
	if devices, ok := result.(roborock.ListDevicesResult); ok {
		devices.Devices = slices.Clone(devices.Devices)
		for index := range devices.Devices {
			devices.Devices[index].LocalKey = ""
		}

		result = devices
	}

	if asJSON {
		return writeCustomerJSON(stdout, result)
	}

	return writeCustomerTable(stdout, command, result)
}

func writeCustomerTable(stdout io.Writer, command string, result any) error {
	switch command {
	case commandDevices:
		devices, ok := result.(roborock.ListDevicesResult)
		if !ok {
			return errCustomerUsage
		}

		return writeDeviceTable(stdout, devices)
	case commandMaps:
		maps, ok := result.(roborock.ListMapsResult)
		if !ok {
			return errCustomerUsage
		}

		return writeMapTable(stdout, maps)
	case commandRooms:
		rooms, ok := result.(roborock.MapRoomsResult)
		if !ok {
			return errCustomerUsage
		}

		return writeRoomTable(stdout, rooms)
	default:
		return writeCustomerJSON(stdout, result)
	}
}

func writeDeviceTable(stdout io.Writer, result roborock.ListDevicesResult) error {
	writer := tabwriter.NewWriter(stdout, 0, 0, tableCellPadding, ' ', 0)

	_, err := fmt.Fprintln(writer, "DEVICE_ID\tNAME\tMODEL")
	if err != nil {
		return fmt.Errorf("write device table: %w", err)
	}

	for _, device := range result.Devices {
		_, err = fmt.Fprintf(writer, "%s\t%s\t%s\n",
			displayCell(device.ID), displayCell(device.Name), displayCell(device.Model))
		if err != nil {
			return fmt.Errorf("write device table: %w", err)
		}
	}

	err = writer.Flush()
	if err != nil {
		return fmt.Errorf("write device table: %w", err)
	}

	return nil
}

func writeMapTable(stdout io.Writer, result roborock.ListMapsResult) error {
	writer := tabwriter.NewWriter(stdout, 0, 0, tableCellPadding, ' ', 0)

	_, err := fmt.Fprintln(writer, "MAP_ID\tNAME\tCURRENT")
	if err != nil {
		return fmt.Errorf("write map table: %w", err)
	}

	for _, saved := range result.Maps {
		current := "unknown"
		if saved.Current != nil {
			current = strconv.FormatBool(*saved.Current)
		}

		_, err = fmt.Fprintf(writer, "%s\t%s\t%s\n", displayCell(saved.ID), optionalName(saved.Name), current)
		if err != nil {
			return fmt.Errorf("write map table: %w", err)
		}
	}

	err = writer.Flush()
	if err != nil {
		return fmt.Errorf("write map table: %w", err)
	}

	return nil
}

func writeRoomTable(stdout io.Writer, result roborock.MapRoomsResult) error {
	writer := tabwriter.NewWriter(stdout, 0, 0, tableCellPadding, ' ', 0)

	_, err := fmt.Fprintln(writer, "ROOM_ID\tNAME")
	if err != nil {
		return fmt.Errorf("write room table: %w", err)
	}

	for _, room := range result.Rooms {
		_, err = fmt.Fprintf(writer, "%d\t%s\n", room.SegmentID, optionalName(room.Name))
		if err != nil {
			return fmt.Errorf("write room table: %w", err)
		}
	}

	err = writer.Flush()
	if err != nil {
		return fmt.Errorf("write room table: %w", err)
	}

	return nil
}

func optionalName(name *string) string {
	if name == nil {
		return "-"
	}

	return displayCell(*name)
}

func displayCell(value string) string {
	quoted := strconv.QuoteToGraphic(value)

	return quoted[1 : len(quoted)-1]
}
