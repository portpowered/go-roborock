package main

import "errors"

var (
	errCleaningZones   = errors.New("cleanZones requires zones with explicit ordered coordinates and repeats 1 through 3")
	errDeviceInput     = errors.New("deviceId, localKey, and protocol are required for device commands")
	errUnknownCommand  = errors.New("unknown command; use help")
	errPositionalInput = errors.New("positional inputs are not accepted; use JSON stdin, file, or environment")
	errTimeout         = errors.New("--timeout must be positive")
	errExportCommand   = errors.New("--export is supported only by resolve-login, login, devices, or camera commands")
	errInputSize       = errors.New("input exceeds 1 MiB")
	errInputObject     = errors.New("input must be a JSON object")
	errInputFields     = errors.New("input must be a JSON object with documented fields")
	errInputTrailing   = errors.New("input must contain exactly one JSON object")
)
