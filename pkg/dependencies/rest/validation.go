package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/portpowered/go-roborock/api"
	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

var responseSchemas = sync.OnceValues(loadResponseSchemas)

func loadResponseSchemas() (map[string]*openapi3.Schema, error) {
	result := make(map[string]*openapi3.Schema)

	for _, file := range []string{"auth.openapi.yaml", "devices.openapi.yaml"} {
		data, err := api.RESTContracts.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read response contract: %w", err)
		}

		doc, err := openapi3.NewLoader().LoadFromData(data)
		if err != nil {
			return nil, fmt.Errorf("load response contract: %w", err)
		}

		for name, component := range doc.Components.Schemas {
			result["schema:"+name] = component.Value
		}

		err = collectResponses(doc, result)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func collectResponses(doc *openapi3.T, result map[string]*openapi3.Schema) error {
	for path, item := range doc.Paths.Map() {
		for method, operation := range item.Operations() {
			response := operation.Responses.Status(http.StatusOK)
			if response == nil || response.Value == nil {
				return roborockerrors.New(roborockerrors.Protocol, "contract",
					"missing successful response contract", nil)
			}

			media := response.Value.Content.Get(protocol.RESTMediaJSON)
			if media == nil || media.Schema == nil {
				return roborockerrors.New(roborockerrors.Protocol, "contract", "missing JSON response contract", nil)
			}

			result[method+" "+path] = media.Schema.Value
		}
	}

	return nil
}

func validateResponse(method, path string, data []byte) error {
	schemas, err := responseSchemas()
	if err != nil {
		return roborockerrors.New(roborockerrors.Protocol, path, "response contract unavailable", err)
	}

	schema := findResponseSchema(schemas, method+" "+path)
	if schema == nil {
		return roborockerrors.New(roborockerrors.Unsupported, path, "route absent from response contract", nil)
	}

	var value any

	err = json.Unmarshal(data, &value)
	if err != nil {
		return roborockerrors.New(roborockerrors.Protocol, path, "invalid JSON response", err)
	}

	err = schema.VisitJSON(value)
	if err != nil {
		return roborockerrors.New(roborockerrors.Protocol, path, "response violates contract", err)
	}

	return nil
}

func validateRoute(method, path string) error {
	schemas, err := responseSchemas()
	if err != nil {
		return roborockerrors.New(roborockerrors.Protocol, path, "response contract unavailable", err)
	}

	if findResponseSchema(schemas, method+" "+path) == nil {
		return roborockerrors.New(roborockerrors.Unsupported, path, "route absent from response contract", nil)
	}

	return nil
}

func findResponseSchema(schemas map[string]*openapi3.Schema, key string) *openapi3.Schema {
	if schema := schemas[key]; schema != nil {
		return schema
	}

	for route, candidate := range schemas {
		if matchesRoute(route, key) {
			return candidate
		}
	}

	return nil
}

func matchesRoute(template, key string) bool {
	expected := strings.Split(template, "/")
	actual := strings.Split(key, "/")

	if len(expected) != len(actual) {
		return false
	}

	for index, segment := range expected {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			if actual[index] == "" {
				return false
			}

			continue
		}

		if segment != actual[index] {
			return false
		}
	}

	return true
}
