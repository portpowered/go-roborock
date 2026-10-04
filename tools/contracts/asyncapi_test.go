package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAsyncAPIRejectsMissingOperationChannel(t *testing.T) {
	t.Parallel()

	const source = `{
  "asyncapi": "3.0.0",
  "info": {
    "title": "Probe",
    "version": "1"
  },
  "channels": {},
  "operations": {
    "send": {
      "action": "send",
      "channel": {
        "$ref": "#/channels/missing"
      }
    }
  },
  "components": {
    "schemas": {}
  }
}`

	path := filepath.Join(t.TempDir(), "invalid.asyncapi.yaml")

	err := os.WriteFile(path, []byte(source), privateFileMode)
	if err != nil {
		t.Fatal(err)
	}

	if validateAsyncSchema(t.Context(), path) == nil {
		t.Fatal("full document validator accepted an unresolved operation channel")
	}
}

func TestAsyncAPIAcceptsCompleteDocument(t *testing.T) {
	t.Parallel()

	const source = `{
 "asyncapi":"3.0.0",
 "info":{"title":"Probe","version":"1"},
 "channels":{},"operations":{},"components":{"schemas":{}}
 }`

	path := filepath.Join(t.TempDir(), "valid.asyncapi.yaml")

	err := os.WriteFile(path, []byte(source), privateFileMode)
	if err != nil {
		t.Fatal(err)
	}

	err = validateAsyncSchema(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
}
