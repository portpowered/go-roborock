package replay_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

const (
	vacuumSchemaPath         = "../../api/vacuum.openapi.yaml"
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

	contract, err := loader.LoadFromFile(vacuumSchemaPath)
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

	path := contract.Paths.Value("/" + testCase.Method)
	if path == nil || path.Post == nil {
		t.Fatal("fixture method absent from RPC contract")
	}

	response := path.Post.Responses.Value("200")
	if response == nil || response.Value == nil {
		t.Fatal("RPC response contract missing")
	}

	media := response.Value.Content.Get("application/json")
	if media == nil || media.Schema == nil {
		t.Fatal("RPC response schema missing")
	}

	var payload any

	decodeErr := json.Unmarshal(testCase.Payload, &payload)
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}

	validateErr := media.Schema.Value.VisitJSON(payload)
	if (validateErr == nil) != testCase.Valid {
		t.Fatalf("response shape validation=%v expected valid=%v", validateErr, testCase.Valid)
	}
}
