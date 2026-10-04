package main

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	testModeField = "mode"
	testModeType  = "Mode"
)

func TestRenderPublicFieldsAndEnums(t *testing.T) {
	t.Parallel()

	spec := document{Components: components{Schemas: map[string]schema{
		"Settings": {Description: "Sparse settings", Properties: map[string]property{
			testModeField: {Description: "Mode value"}, "enabled": {Description: "False is explicit"},
		}, Required: []string{testModeField}, Enum: nil, EnumNames: nil},
		testModeType: {Description: "Open value", Properties: nil, Required: nil,
			Enum: []json.RawMessage{json.RawMessage("1")}, EnumNames: []string{"ModeOne"}},
	}}}
	fields := map[string][]goField{"Settings": {
		{name: "Enabled", jsonName: "enabled", kind: "*bool"},
		{name: testModeType, jsonName: testModeField, kind: testModeType},
	}}

	rendered, err := render(spec, fields)
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{"`Enabled` | `enabled` | `*bool` | no", "`Mode` | `mode` | `Mode` | yes",
		"`ModeOne` | `1`", "#Settings"} {
		if !strings.Contains(string(rendered), expected) {
			t.Errorf("missing public contract %s", expected)
		}
	}

	delete(fields, "Settings")

	_, err = render(spec, fields)
	if err == nil {
		t.Error("accepted schema fields with no generated Go declaration")
	}
}
