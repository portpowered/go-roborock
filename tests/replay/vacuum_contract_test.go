package replay_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/portpowered/go-roborock/internal/schemaadapter"
	"gopkg.in/yaml.v3"
)

const (
	vacuumSchemaPath         = "../../api/vacuum.asyncapi.yaml"
	responseShapeFixturePath = "fixtures/vacuum/synthetic/response-shapes.json"
)

type responseShapeFixture struct {
	Classification string              `json:"classification"`
	Source         string              `json:"source"`
	Cases          []responseShapeCase `json:"cases"`
}

type responseShapeCase struct {
	Name    string          `json:"name"`
	Method  string          `json:"method"`
	Payload json.RawMessage `json:"payload"`
	Valid   bool            `json:"valid"`
}

func TestVacuumResponseShapeContracts(t *testing.T) {
	t.Parallel()

	contract := loadVacuumContract(t)

	fixture := loadResponseShapeFixture(t)
	for _, testCase := range fixture.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()
			validateResponseShape(t, contract, testCase)
		})
	}
}

func loadVacuumContract(t *testing.T) *openapi3.T {
	t.Helper()

	loader := openapi3.NewLoader()
	loader.Context = t.Context()

	data, err := os.ReadFile(vacuumSchemaPath)
	if err != nil {
		t.Fatal(err)
	}

	projected, err := schemaadapter.Project(data)
	if err != nil {
		t.Fatal(err)
	}

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

func loadResponseShapeFixture(t *testing.T) responseShapeFixture {
	t.Helper()

	data, err := os.ReadFile(responseShapeFixturePath)
	if err != nil {
		t.Fatal(err)
	}

	var fixture responseShapeFixture

	err = json.Unmarshal(data, &fixture)
	if err != nil {
		t.Fatal(err)
	}

	if fixture.Classification != accountFixtureProvenance || fixture.Source == "" {
		t.Fatal("response fixture must identify synthetic provenance")
	}

	return fixture
}

func validateResponseShape(t *testing.T, contract *openapi3.T, testCase responseShapeCase) {
	t.Helper()

	data, err := os.ReadFile(vacuumSchemaPath)
	if err != nil {
		t.Fatal(err)
	}

	var document map[string]any

	err = yaml.Unmarshal(data, &document)
	if err != nil {
		t.Fatal(err)
	}

	operations, _ := document["operations"].(map[string]any)
	components, _ := document["components"].(map[string]any)
	messages, _ := components["messages"].(map[string]any)

	var result *openapi3.SchemaRef

	for name, value := range operations {
		operation, _ := value.(map[string]any)
		if operation["x-rpc-method"] != testCase.Method {
			continue
		}

		message, _ := messages[name+"Response"].(map[string]any)
		body, _ := message["payload"].(map[string]any)
		properties, _ := body["properties"].(map[string]any)
		schema, _ := properties["result"].(map[string]any)
		ref, _ := schema["$ref"].(string)
		result = contract.Components.Schemas[strings.TrimPrefix(ref, "#/components/schemas/")]
	}

	if result == nil {
		t.Fatal("fixture method absent from AsyncAPI reply contract")
	}

	var payload any

	decodeErr := json.Unmarshal(testCase.Payload, &payload)
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}

	validateErr := result.Value.VisitJSON(payload)
	if (validateErr == nil) != testCase.Valid {
		t.Fatalf("response shape validation=%v expected valid=%v", validateErr, testCase.Valid)
	}
}
