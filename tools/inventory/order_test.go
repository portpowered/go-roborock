package main

import (
	"encoding/json"
	"testing"
)

func TestRepeatedBlankDeclarationsHaveStableInventory(t *testing.T) {
	t.Parallel()

	// protoc-gen-go emits two distinct version guards with the same blank symbol.
	var minimum, maximum declaration

	minimum.Symbol = "mapproto._"
	minimum.Definition.File = "mapproto/b01_scmap.pb.go"
	minimum.Definition.Line = 19
	minimum.Value = "20"
	maximum.Symbol = minimum.Symbol
	maximum.Definition.File = minimum.Definition.File
	maximum.Definition.Line = 21
	maximum.Value = "16"

	forward := []declaration{minimum, maximum}
	backward := []declaration{maximum, minimum}

	sortDeclarations(forward)
	sortDeclarations(backward)

	first, err := json.Marshal(forward)
	if err != nil {
		t.Fatal(err)
	}

	second, err := json.Marshal(backward)
	if err != nil {
		t.Fatal(err)
	}

	if string(first) != string(second) || forward[0].Definition.Line != minimum.Definition.Line {
		t.Fatal("blank protobuf declarations must have stable source order")
	}
}
