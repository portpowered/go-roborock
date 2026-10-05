package main

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/portpowered/go-roborock/pkg/roborock"
)

func TestCustomerDeviceOutputProtectsSecretsAndTerminal(t *testing.T) {
	t.Parallel()

	var device roborock.Device

	device.ID = "vacuum-1"
	device.Name = "Kitchen\n\x1b[31m\u202e"
	device.Model = "roborock.vacuum.example"
	device.LocalKey = "synthetic-private-local-key"
	result := roborock.ListDevicesResult{Devices: []roborock.Device{device}}

	var output bytes.Buffer

	err := writeCustomerResult(&output, commandDevices, result, false)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(output.String(), device.LocalKey) || strings.ContainsAny(output.String(), "\x1b\u202e") {
		t.Fatal("table exposed a device secret or terminal control character")
	}

	if !strings.Contains(output.String(), device.ID) {
		t.Fatal("output omitted the ID needed for the next command")
	}

	if strings.Count(output.String(), "\n") != 2 || !strings.Contains(output.String(), "DEVICE_ID") {
		t.Fatal("device output did not remain a header and one readable row")
	}

	if result.Devices[0].LocalKey != device.LocalKey {
		t.Fatal("rendering mutated caller-owned device credentials")
	}
}

func TestCustomerJSONOutputRedactsWithoutMutatingCredentials(t *testing.T) {
	t.Parallel()

	var device roborock.Device

	device.ID = "vacuum-1"
	device.LocalKey = "synthetic-output-secret"
	result := roborock.ListDevicesResult{Devices: []roborock.Device{device}}

	var output bytes.Buffer

	err := writeCustomerResult(&output, commandDevices, result, true)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(output.String(), device.LocalKey) || result.Devices[0].LocalKey != device.LocalKey {
		t.Fatal("JSON rendering exposed or mutated credentials")
	}
}

func TestCustomerMapAndRoomTablesPreserveIDsAndUnknownMetadata(t *testing.T) {
	t.Parallel()

	name := "Living room"
	current := false
	maps := roborock.ListMapsResult{Maps: []roborock.MapInfo{
		{ID: "floor-2", Name: &name, Current: &current},
		{ID: "floor-3", Name: nil, Current: nil},
	}}
	rooms := roborock.MapRoomsResult{Rooms: []roborock.MapRoomMapping{
		{SegmentID: 0, Name: &name, IoTID: nil},
		{SegmentID: 4294967295, Name: nil, IoTID: nil},
	}}

	var output bytes.Buffer

	err := writeCustomerResult(&output, commandMaps, maps, false)
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{"MAP_ID", "floor-2", strconv.FormatBool(current), "floor-3", "unknown"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("map table omitted %q", expected)
		}
	}

	output.Reset()

	err = writeCustomerResult(&output, commandRooms, rooms, false)
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{"ROOM_ID", "0", "4294967295", name} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("room table omitted %q", expected)
		}
	}
}

type failedCustomerOutput struct{}

func (failedCustomerOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestCustomerTableOutputReportsWriteFailure(t *testing.T) {
	t.Parallel()

	result := roborock.ListDevicesResult{Devices: nil}

	err := writeCustomerResult(failedCustomerOutput{}, commandDevices, result, false)
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("lost the output failure: %v", err)
	}
}
