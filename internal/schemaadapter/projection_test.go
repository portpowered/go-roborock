package schemaadapter_test

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/go-roborock/internal/schemaadapter"
)

func TestProjectionKeepsModelsWithoutHTTPRoutes(t *testing.T) {
	t.Parallel()

	const source = `{
  "asyncapi": "3.0.0",
  "info": {
    "title": "MQTT",
    "version": "1"
  },
  "components": {
    "schemas": {
      "Input": {
        "type": "object"
      }
    }
  },
  "operations": {
    "Send": {
      "action": "send",
      "x-go-request-schema": {
        "$ref": "#/components/schemas/Input"
      }
    }
  }
}`

	data, err := schemaadapter.Project([]byte(source))
	if err != nil {
		t.Fatal(err)
	}

	var document map[string]any

	err = json.Unmarshal(data, &document)
	if err != nil {
		t.Fatal(err)
	}

	paths, ok := document["paths"].(map[string]any)
	if !ok || len(paths) != 0 {
		t.Fatal("adapter introduced HTTP operations")
	}

	components, _ := document["components"].(map[string]any)

	schemas, _ := components["schemas"].(map[string]any)
	if schemas["Input"] == nil || schemas["SendJSONRequestBody"] == nil {
		t.Fatal("stable component or request alias missing")
	}

	if document["operations"] != nil {
		t.Fatal("MQTT operations leaked into component adapter")
	}
}
