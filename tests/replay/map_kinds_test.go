package replay_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

type mapKindContractCase struct {
	Name    string          `json:"name"`
	Schema  string          `json:"schema"`
	Valid   bool            `json:"valid"`
	Payload json.RawMessage `json:"payload"`
}

type mapKindContractFixture struct {
	Provenance string                `json:"provenance"`
	Cases      []mapKindContractCase `json:"cases"`
}

func TestMapKindContractsRemainOpen(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("fixtures/maps/synthetic/kind-contracts.json")
	if err != nil {
		t.Fatal(err)
	}

	var fixture mapKindContractFixture

	err = json.Unmarshal(data, &fixture)
	if err != nil {
		t.Fatal(err)
	}

	if fixture.Provenance != "synthetic" {
		t.Fatal("invalid kind fixture provenance")
	}

	loader := openapi3.NewLoader()

	document, err := loader.LoadFromFile("../../api/maps-models.openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}

	for _, testCase := range fixture.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()

			var payload any

			decodeErr := json.Unmarshal(testCase.Payload, &payload)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}

			validationErr := document.Components.Schemas[testCase.Schema].Value.VisitJSON(payload)
			if (validationErr == nil) != testCase.Valid {
				t.Fatalf("kind contract mismatch: %v", validationErr)
			}
		})
	}
}
