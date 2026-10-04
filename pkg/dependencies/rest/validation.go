package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/portpowered/go-roborock/api"
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
		for path, item := range doc.Paths.Map() {
			for method, operation := range item.Operations() {
				response := operation.Responses.Status(200)
				if response == nil || response.Value == nil {
					return nil, errors.New("missing successful response contract")
				}
				media := response.Value.Content.Get("application/json")
				if media == nil || media.Schema == nil {
					return nil, errors.New("missing JSON response contract")
				}
				result[method+" "+path] = media.Schema.Value
			}
		}
	}
	return result, nil
}

func validateResponse(method, path string, data []byte) error {
	schemas, err := responseSchemas()
	if err != nil {
		return roborockerrors.New(roborockerrors.Protocol, path, "response contract unavailable", err)
	}
	key := method + " " + path
	schema := schemas[key]
	if schema == nil {
		for route, candidate := range schemas {
			prefix, _, hasID := strings.Cut(route, "{homeID}")
			if hasID && strings.HasPrefix(key, prefix) {
				id := strings.TrimPrefix(key, prefix)
				if id != "" && !strings.Contains(id, "/") {
					schema = candidate
					break
				}
			}
		}
	}
	if schema == nil {
		return roborockerrors.New(roborockerrors.Unsupported, path, "route absent from response contract", nil)
	}
	var value any
	if err = json.Unmarshal(data, &value); err != nil {
		return roborockerrors.New(roborockerrors.Protocol, path, "invalid JSON response", err)
	}
	if err = schema.VisitJSON(value); err != nil {
		return roborockerrors.New(roborockerrors.Protocol, path, "response violates contract", err)
	}
	return nil
}
