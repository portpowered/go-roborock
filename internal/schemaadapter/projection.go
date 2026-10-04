// Package schemaadapter projects canonical MQTT schemas for the existing Go model generator.
package schemaadapter

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Project returns a temporary OpenAPI document for component tooling only.
// SCHEMA-16: AsyncAPI remains the wire owner; this projection is never published.
func Project(data []byte) ([]byte, error) {
	var source map[string]any

	err := yaml.Unmarshal(data, &source)
	if err != nil {
		return nil, fmt.Errorf("decode canonical schema: %w", err)
	}

	components, _ := source["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)

	operations, _ := source["operations"].(map[string]any)

	for name, value := range operations {
		operation, _ := value.(map[string]any)
		if request, exists := operation["x-go-request-schema"]; exists {
			schemas[name+"JSONRequestBody"] = request
		}
	}

	target := map[string]any{
		"openapi": "3.0.3", "info": source["info"], "paths": map[string]any{},
		"components": map[string]any{"schemas": schemas},
	}

	for key, value := range source {
		if key == "x-wire-constants" {
			target[key] = value
		}
	}

	result, err := json.Marshal(target)
	if err != nil {
		return nil, fmt.Errorf("encode component projection: %w", err)
	}

	return result, nil
}
