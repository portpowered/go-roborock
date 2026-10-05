package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

var errCustomerUsage = errors.New("invalid command arguments; run go-roborock help")

type customerOptions struct {
	command     string
	profile     string
	device      string
	repeats     int
	timeout     time.Duration
	zones       []string
	positionals []string
}

type zoneArguments []string

func (values *zoneArguments) String() string { return strings.Join(*values, ";") }
func (values *zoneArguments) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func customerCommand(args []string) bool {
	for _, value := range args {
		if value == "--input" || strings.HasPrefix(value, "--input=") {
			return false
		}
	}

	switch args[0] {
	case commandLogin, commandLogout, "zones":
		return true
	case commandDevices, commandMaps, commandRooms:
		return len(args) > 1 && !strings.HasPrefix(args[1], "-")
	default:
		return false
	}
}

func parseCustomer(args []string, stderr io.Writer) (customerOptions, error) {
	options := customerOptions{command: "", profile: "", device: "", repeats: 1,
		timeout: defaultOperationTimeout, zones: nil, positionals: nil}

	command, remaining, err := customerRoute(args)

	if err != nil {
		return options, err
	}

	options.command = command
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { _, _ = io.WriteString(stderr, helpText) }
	flags.StringVar(&options.profile, "profile", "", "saved login profile path")
	if command != commandLogin && command != commandLogout && command != commandDevices {
		flags.StringVar(&options.device, "device", "", "device ID from devices list")
	}
	if command == commandCleanRooms || command == commandCleanZones {
		flags.IntVar(&options.repeats, "repeats", 1, "cleaning passes (1 through 3)")
	}
	flags.DurationVar(&options.timeout, "timeout", defaultOperationTimeout, "timeout for each network exchange")

	var zones zoneArguments

	if command == commandCleanZones {
		flags.Var(&zones, "zone", "rectangle in map millimeters: x1,y1,x2,y2; repeat flag for more zones")
	}
	// The standard flag package stops at positional arguments; split them so documented flags may follow IDs.
	flagArgs, positionals, err := separateCustomerFlags(remaining)
	if err != nil {
		return options, err
	}

	err = flags.Parse(flagArgs)

	if err != nil {
		return options, fmt.Errorf("parse command: %w", err)
	}

	options.positionals = positionals

	options.zones = zones

	if options.timeout <= 0 {
		return options, errTimeout
	}

	if options.repeats < 1 || options.repeats > 3 {
		return options, errCustomerUsage
	}

	if options.profile == "" {
		options.profile, err = defaultProfile()
	}

	return options, err
}

func customerRoute(args []string) (string, []string, error) {
	if args[0] == commandLogin || args[0] == commandLogout {
		return args[0], args[1:], nil
	}

	if len(args) < minimumRouteArguments {
		return "", nil, errCustomerUsage
	}

	route := args[0] + " " + args[1]
	switch route {
	case "devices list":
		return commandDevices, args[2:], nil
	case "devices vacuum":
		return vacuumRoute(args)
	case "maps list":
		return commandMaps, args[2:], nil
	case "maps show":
		return commandMap, args[2:], nil
	case "maps select":
		return commandSelectMap, args[2:], nil
	case "maps trace":
		return commandTrace, args[2:], nil
	case "rooms list":
		return commandRooms, args[2:], nil
	case "rooms clean":
		return commandCleanRooms, args[2:], nil
	case "zones clean":
		return commandCleanZones, args[2:], nil
	default:
		return "", nil, errCustomerUsage
	}
}

func vacuumRoute(args []string) (string, []string, error) {
	if len(args) < minimumVacuumArguments || strings.HasPrefix(args[2], "-") {
		return "", nil, errCustomerUsage
	}
	command, remaining := commandStatus, args[3:]
	if len(remaining) > 0 && !strings.HasPrefix(remaining[0], "-") {
		command, remaining = remaining[0], remaining[1:]
	}
	for _, value := range remaining {
		if value == "--device" || strings.HasPrefix(value, "--device=") {
			return "", nil, errCustomerUsage
		}
	}
	switch command {
	case commandStatus, commandStart, commandPause, commandStop, commandDock,
		commandConsumables, commandSummary, commandCapabilities:
		return command, append([]string{"--device", args[2]}, remaining...), nil
	default:
		return "", nil, errCustomerUsage
	}
}

func separateCustomerFlags(args []string) ([]string, []string, error) {
	var flags, positionals []string

	for index := 0; index < len(args); index++ {
		value := args[index]
		if !strings.HasPrefix(value, "-") {
			positionals = append(positionals, value)

			continue
		}

		flags = append(flags, value)

		if strings.Contains(value, "=") || value == "--help" || value == "-h" {
			continue
		}

		index++
		if index >= len(args) {
			return nil, nil, errCustomerUsage
		}

		flags = append(flags, args[index])
	}

	return flags, positionals, nil
}

func customerInput(options customerOptions) (CommandInput, error) {
	//nolint:exhaustruct,exhaustruct_v5 // Optional generated fields apply only to the selected operation.
	input := CommandInput{}

	switch options.command {
	case commandMap, commandSelectMap:
		if len(options.positionals) > 1 || (options.command == commandSelectMap && len(options.positionals) != 1) {
			return input, errCustomerUsage
		}

		if len(options.positionals) == 1 {
			input.MapID = options.positionals[0]
		}
	case commandCleanRooms:
		return customerRoomsInput(input, options)
	case commandCleanZones:
		return customerZonesInput(input, options)
	default:
		if len(options.positionals) != 0 {
			return input, errCustomerUsage
		}
	}

	return input, nil
}

func customerRoomsInput(input CommandInput, options customerOptions) (CommandInput, error) {
	if len(options.positionals) == 0 {
		return input, errCustomerUsage
	}
	input.CleanRooms.Repeats = options.repeats
	for _, value := range options.positionals {
		roomID, err := strconv.ParseInt(value, 10, 64)
		if err != nil || roomID < 0 {
			return input, errCustomerUsage
		}
		input.CleanRooms.Segments = append(input.CleanRooms.Segments, roomID)
	}
	return input, nil
}

func customerZonesInput(input CommandInput, options customerOptions) (CommandInput, error) {
	if len(options.positionals) != 0 || len(options.zones) == 0 {
		return input, errCustomerUsage
	}
	input.CleanZones = &CleanZonesInput{Zones: nil}
	for _, value := range options.zones {
		zone, err := parseCustomerZone(value, options.repeats)
		if err != nil {
			return input, err
		}
		input.CleanZones.Zones = append(input.CleanZones.Zones, zone)
	}
	_, err := cleanZonesRequest(input.CleanZones)
	return input, err
}

func parseCustomerZone(value string, repeats int) (CleaningZoneInput, error) {
	zone := CleaningZoneInput{X1: nil, Y1: nil, X2: nil, Y2: nil, Repeats: repeats}

	parts := strings.Split(value, ",")

	if len(parts) != zoneCoordinateCount {
		return zone, errCustomerUsage
	}

	coordinates := make([]int64, len(parts))

	for index, part := range parts {
		coordinate, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return zone, errCustomerUsage
		}

		coordinates[index] = coordinate
	}

	zone.X1, zone.Y1, zone.X2, zone.Y2 = &coordinates[0], &coordinates[1], &coordinates[2], &coordinates[3]

	return zone, nil
}
