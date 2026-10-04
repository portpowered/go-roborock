package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/portpowered/go-roborock/internal/schemaadapter"
)

func TestSyntheticMessageExamplesMatchContracts(t *testing.T) {
	t.Parallel()

	messages, contract := exampleContract(t)
	count := 0

	for name, value := range messages {
		message := exampleObject(t, value)
		examples, _ := message["examples"].([]any)

		for _, value := range examples {
			example := exampleObject(t, value)
			count++

			exampleName, _ := example["name"].(string)

			t.Run(name+"/"+exampleName, func(t *testing.T) {
				t.Parallel()

				if !strings.HasPrefix(exampleName, "Synthetic-") {
					t.Fatal("documentation example must identify synthetic provenance")
				}

				err := contract.Components.Schemas["MessageExample."+name].Value.VisitJSON(example["payload"])
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}

	if count < 19 {
		t.Fatalf("expected all known requests and typed replies, found %d examples", count)
	}
}

func TestMessageIDExamplesRejectInvalidFallback(t *testing.T) {
	t.Parallel()

	_, contract := exampleContract(t)
	count := 0

	for name, schema := range contract.Components.Schemas {
		identifier, present := schema.Value.Properties["msgId"]
		if !present {
			continue
		}

		count++

		if identifier.Value.Example == nil {
			t.Fatalf("%s has no valid message ID example", name)
		}

		err := identifier.Value.VisitJSON(identifier.Value.Example)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		err = identifier.Value.VisitJSON("string")
		if err == nil {
			t.Fatalf("%s accepts the renderer's invalid message ID fallback", name)
		}
	}

	if count == 0 {
		t.Fatal("no message ID schemas validated")
	}
}

func exampleContract(t *testing.T) (map[string]any, *openapi3.T) {
	t.Helper()

	files, err := filepath.Glob("../../api/*.asyncapi.yaml")
	if err != nil {
		t.Fatal(err)
	}

	data, err := buildAsync(files)
	if err != nil {
		t.Fatal(err)
	}

	var document map[string]any

	err = json.Unmarshal(data, &document)
	if err != nil {
		t.Fatal(err)
	}

	components := exampleObject(t, document["components"])
	messages := exampleObject(t, components["messages"])
	schemas := exampleObject(t, components["schemas"])

	for name, value := range messages {
		message := exampleObject(t, value)
		if _, present := message["examples"]; present {
			schemas["MessageExample."+name] = message["payload"]
		}
	}

	return messages, loadExampleContract(t, document)
}

func loadExampleContract(t *testing.T, document map[string]any) *openapi3.T {
	t.Helper()

	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	data, err = schemaadapter.Project(data)
	if err != nil {
		t.Fatal(err)
	}

	contract, err := openapi3.NewLoader().LoadFromData(data)
	if err != nil {
		t.Fatal(err)
	}

	err = contract.Validate(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	return contract
}

func exampleObject(t *testing.T, value any) map[string]any {
	t.Helper()

	result, valid := value.(map[string]any)
	if !valid {
		t.Fatalf("expected JSON object, found %T", value)
	}

	return result
}
