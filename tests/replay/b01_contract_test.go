package replay_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/portpowered/go-roborock/internal/schemaadapter"
)

const b01ContractPath = "../../api/b01.asyncapi.yaml"
const mapsContractPath = "../../api/maps.asyncapi.yaml"

type b01ShapeFixture struct {
	Provenance string         `json:"provenance"`
	Source     string         `json:"source"`
	Revision   string         `json:"revision"`
	Cases      []b01ShapeCase `json:"cases"`
}

type b01ShapeCase struct {
	Name       string          `json:"name"`
	Contract   string          `json:"contract"`
	Operation  string          `json:"operation"`
	Reply      bool            `json:"reply"`
	Payload    json.RawMessage `json:"payload"`
	Valid      bool            `json:"valid"`
	PairedCase *int            `json:"pairedCase"`
	PairedStep *int            `json:"pairedStep"`
}

func TestB01KnownOperationShapeContracts(t *testing.T) {
	t.Parallel()
	b01Document, b01Contract := loadB01Contract(t, b01ContractPath)
	mapDocument, mapContract := loadB01Contract(t, mapsContractPath)
	fixture := loadB01ShapeFixture(t)

	paired := loadB01PairedFixture(t)

	for _, testCase := range fixture.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()
			assertB01PairedPayload(t, paired, testCase)

			document, contract := b01Document, b01Contract
			if testCase.Contract == "maps" {
				document, contract = mapDocument, mapContract
			}

			schema := boundB01OperationSchema(t, document, contract, testCase)

			var payload any

			err := json.Unmarshal(testCase.Payload, &payload)
			if err != nil {
				t.Fatal(err)
			}

			err = schema.VisitJSON(payload)
			if (err == nil) != testCase.Valid {
				t.Fatalf("operation=%s validation=%v expected valid=%v", testCase.Operation, err, testCase.Valid)
			}
		})
	}
}

func loadB01PairedFixture(t *testing.T) mapReplayFixture {
	t.Helper()

	data, err := os.ReadFile("fixtures/mqtt/synthetic/maps.json")
	if err != nil {
		t.Fatal(err)
	}

	var fixture mapReplayFixture

	err = json.Unmarshal(data, &fixture)
	if err != nil {
		t.Fatal(err)
	}

	return fixture
}

func assertB01PairedPayload(t *testing.T, paired mapReplayFixture, testCase b01ShapeCase) {
	t.Helper()

	if testCase.PairedCase == nil {
		return
	}

	if testCase.PairedStep == nil {
		t.Fatal("paired contract case lacks transcript step")
	}

	caseIndex, stepIndex := *testCase.PairedCase, *testCase.PairedStep
	if caseIndex < 0 || caseIndex >= len(paired.Cases) {
		t.Fatal("paired contract case outside transcript")
	}

	steps := paired.Cases[caseIndex].Steps
	if stepIndex < 0 || stepIndex >= len(steps) {
		t.Fatal("paired contract step outside transcript")
	}

	if !equalMQTTJSON(testCase.Payload, steps[stepIndex].Request) {
		t.Fatal("bound known contract differs from actual paired MQTT request")
	}
}

func loadB01Contract(t *testing.T, path string) (map[string]any, *openapi3.T) {
	t.Helper()

	data, err := os.ReadFile(path) //nolint:gosec // Caller selects one of two repository-owned contract constants.
	if err != nil {
		t.Fatal(err)
	}

	var document map[string]any

	err = json.Unmarshal(data, &document)
	if err != nil {
		t.Fatal(err)
	}

	return document, projectB01Contract(t, data)
}

func projectB01Contract(t *testing.T, data []byte) *openapi3.T {
	t.Helper()

	projected, err := schemaadapter.Project(data)
	if err != nil {
		t.Fatal(err)
	}

	loader := openapi3.NewLoader()
	loader.Context = t.Context()

	contract, err := loader.LoadFromData(projected)
	if err != nil {
		t.Fatal(err)
	}

	err = contract.Validate(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	return contract
}

func loadB01ShapeFixture(t *testing.T) b01ShapeFixture {
	t.Helper()

	data, err := os.ReadFile("fixtures/mqtt/synthetic/b01-known-shapes.json")
	if err != nil {
		t.Fatal(err)
	}

	var fixture b01ShapeFixture

	err = json.Unmarshal(data, &fixture)
	if err != nil {
		t.Fatal(err)
	}

	if fixture.Provenance != accountFixtureProvenance || fixture.Source == "" ||
		fixture.Revision != mqttReferenceRevision {
		t.Fatal("known B01 shape fixture must identify synthetic pinned-reference provenance")
	}

	return fixture
}

func boundB01OperationSchema(
	t *testing.T, document map[string]any, contract *openapi3.T, testCase b01ShapeCase,
) *openapi3.Schema {
	t.Helper()

	operationID := testCase.Operation
	operations, _ := document["operations"].(map[string]any)

	operation, _ := operations[operationID].(map[string]any)
	if operation == nil {
		t.Fatalf("missing bound operation %s", operationID)
	}

	if testCase.Reply {
		operation, _ = operation["reply"].(map[string]any)
	}

	messages, _ := operation["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("operation %s must bind one known message", operationID)
	}

	message := resolveB01ContractObject(t, document, messages[0])
	payload, _ := message["payload"].(map[string]any)
	ref, _ := payload["$ref"].(string)
	name := strings.TrimPrefix(ref, "#/components/schemas/")

	schema := contract.Components.Schemas[name]
	if schema == nil {
		t.Fatalf("operation %s has no named known payload contract", operationID)
	}

	return schema.Value
}

func resolveB01ContractObject(t *testing.T, document map[string]any, value any) map[string]any {
	t.Helper()

	object, _ := value.(map[string]any)

	for {
		ref, _ := object["$ref"].(string)
		if ref == "" {
			return object
		}

		if !strings.HasPrefix(ref, "#/") {
			t.Fatalf("nonlocal known B01 message reference %q", ref)
		}

		object = document
		for segment := range strings.SplitSeq(strings.TrimPrefix(ref, "#/"), "/") {
			object, _ = object[segment].(map[string]any)
			if object == nil {
				t.Fatalf("unresolved known B01 message reference %q", ref)
			}
		}
	}
}
