package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

type schemaOwner struct {
	File    string `json:"file"`
	Pointer string `json:"pointer"`
}
type schemaIndex map[string]map[string]schemaOwner

func normalized(value string) string {
	return strings.Map(func(char rune) rune {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			return unicode.ToLower(char)
		}

		return -1
	}, value)
}

func owners() (schemaIndex, error) {
	files, err := filepath.Glob("api/*.openapi.yaml")
	if err != nil {
		return nil, fmt.Errorf("find schemas: %w", err)
	}

	result := make(schemaIndex)

	for _, file := range files {
		data, readErr := os.ReadFile(file) //nolint:gosec // LIB-13: schema-only repository glob.
		if readErr != nil {
			return nil, fmt.Errorf("read schema: %w", readErr)
		}

		var doc map[string]any

		err = yaml.Unmarshal(data, &doc)
		if err != nil {
			return nil, fmt.Errorf("parse schema: %w", err)
		}

		entries := make(map[string]schemaOwner)
		components, _ := doc["components"].(map[string]any)

		schemas, _ := components["schemas"].(map[string]any)
		for name, schema := range schemas {
			collectSchema(entries, filepath.ToSlash(file), "#/components/schemas/"+escape(name), name, schema)
		}

		collectOperations(doc, file, entries, result, schemas)
		stem := strings.TrimSuffix(filepath.Base(file), ".openapi.yaml")

		output := modelOutput(stem)
		result[output] = entries
		constants, _ := doc["x-wire-constants"].(map[string]any)

		wire := make(map[string]schemaOwner)

		for name := range constants {
			wire[normalized(name)] = schemaOwner{filepath.ToSlash(file), "#/x-wire-constants/" + escape(name)}
		}

		result["internal/protocol/"+stem+".gen.go"] = wire

		known := make(map[string]schemaOwner)

		for name, value := range schemas {
			schema, _ := value.(map[string]any)

			values, _ := schema["x-known-values"].(map[string]any)
			for constant := range values {
				pointer := "#/components/schemas/" + escape(name) + "/x-known-values/" + escape(constant)
				known[normalized(constant)] = schemaOwner{filepath.ToSlash(file), pointer}
			}
		}

		result[strings.TrimSuffix(strings.TrimSuffix(output, ".gen.go"), "_models")+"_constants.gen.go"] = known
	}

	return result, nil
}

func escape(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func putOwner(entries map[string]schemaOwner, name string, owner schemaOwner) {
	previous, exists := entries[name]
	if !exists || owner.File+owner.Pointer < previous.File+previous.Pointer {
		entries[name] = owner
	}
}

func collectSchema(entries map[string]schemaOwner, file, pointer, name string, value any) {
	schema, ok := value.(map[string]any)
	if !ok {
		return
	}

	entries[normalized(name)] = schemaOwner{file, pointer}
	properties, _ := schema["properties"].(map[string]any)

	keys := make([]string, 0, len(properties))

	for key := range properties {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		collectSchema(entries, file, pointer+"/properties/"+escape(key), name+key, properties[key])
	}

	for _, union := range []string{"anyOf", "oneOf", "allOf"} {
		alternatives, _ := schema[union].([]any)
		for index, alternative := range alternatives {
			alternativePointer := fmt.Sprintf("%s/%s/%d", pointer, union, index)
			collectSchema(entries, file, alternativePointer, fmt.Sprintf("%s%d", name, index), alternative)
		}
	}

	if items, exists := schema["items"]; exists {
		collectSchema(entries, file, pointer+"/items", name+"Item", items)
	}
}

func collectOperations(
	doc map[string]any, file string, entries map[string]schemaOwner, result schemaIndex, schemas map[string]any,
) {
	paths, _ := doc["paths"].(map[string]any)

	rest := result["internal/protocol/rest.gen.go"]
	if rest == nil {
		rest = make(map[string]schemaOwner)
		result["internal/protocol/rest.gen.go"] = rest
	}

	for route, item := range paths {
		methods, _ := item.(map[string]any)
		for method, operation := range methods {
			collectOperation(operation, file, route, method, entries, rest, schemas)
		}
	}
}

func collectOperation(
	operation any, file, route, method string, entries, rest map[string]schemaOwner, schemas map[string]any,
) {
	operationMap, isOperation := operation.(map[string]any)
	if !isOperation {
		return
	}

	operationID, _ := operationMap["operationId"].(string)
	if operationID == "" {
		return
	}

	pointer := "#/paths/" + escape(route) + "/" + method
	if strings.HasSuffix(file, "auth.openapi.yaml") || strings.HasSuffix(file, "devices.openapi.yaml") {
		rest[normalized("RESTPath"+operationID)] = schemaOwner{filepath.ToSlash(file), pointer}
		rest[normalized("RESTMethod"+operationID)] = schemaOwner{filepath.ToSlash(file), pointer}
	}

	entries[normalized(operationID+"Params")] = schemaOwner{filepath.ToSlash(file), pointer + "/parameters"}

	params, _ := operationMap["parameters"].([]any)
	for index, value := range params {
		param, _ := value.(map[string]any)
		name, _ := param["name"].(string)
		parameterPointer := fmt.Sprintf("%s/parameters/%d", pointer, index)
		collectSchema(entries, filepath.ToSlash(file), parameterPointer+"/schema", operationID+"Params"+name, param["schema"])

		in, _ := param["in"].(string)
		if strings.HasSuffix(file, "auth.openapi.yaml") || strings.HasSuffix(file, "devices.openapi.yaml") {
			putOwner(rest, normalized("REST"+in+name), schemaOwner{filepath.ToSlash(file), parameterPointer})
		}
	}

	body, _ := operationMap["requestBody"].(map[string]any)

	content, _ := body["content"].(map[string]any)
	collectBody(content, file, pointer, operationID, entries, rest, schemas)
}

func collectBody(
	content map[string]any, file, pointer, operationID string,
	entries, rest map[string]schemaOwner, schemas map[string]any,
) {
	for media, value := range content {
		suffix := "JSONRequestBody"
		if media == "application/x-www-form-urlencoded" {
			suffix = "FormdataRequestBody"
			item, _ := value.(map[string]any)
			schema, _ := item["schema"].(map[string]any)
			propertyPointer := pointer + "/requestBody/content/" + escape(media) + "/schema"

			if ref, ok := schema["$ref"].(string); ok && strings.HasPrefix(ref, "#/components/schemas/") {
				schema, _ = schemas[strings.TrimPrefix(ref, "#/components/schemas/")].(map[string]any)
				propertyPointer = ref
			}

			properties, _ := schema["properties"].(map[string]any)
			for property := range properties {
				owner := schemaOwner{filepath.ToSlash(file), propertyPointer + "/properties/" + escape(property)}
				putOwner(rest, normalized("RESTForm"+property), owner)
			}
		}

		bodyPointer := pointer + "/requestBody/content/" + escape(media) + "/schema"
		entries[normalized(operationID+suffix)] = schemaOwner{filepath.ToSlash(file), bodyPointer}
	}
}

func modelOutput(stem string) string {
	output := "pkg/dependencymodels/" + stem + ".gen.go"

	if strings.HasSuffix(stem, "-models") {
		output = "pkg/roborock/" + strings.ReplaceAll(stem, "-", "_") + ".gen.go"
	}

	if stem == "a01-wire" {
		output = "pkg/dependencymodels/a01_models.gen.go"
	}

	if stem == "cli-models" {
		output = "cmd/go-roborock/input.gen.go"
	}

	return output
}
