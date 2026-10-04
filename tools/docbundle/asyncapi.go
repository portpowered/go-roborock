package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const asyncOperationsKey = "operations"

func writeAsyncBundle(source, output string) error {
	files, err := filepath.Glob(filepath.Join(source, "*.asyncapi.yaml"))
	if err != nil {
		return fmt.Errorf("find MQTT documentation: %w", err)
	}

	if len(files) == 0 {
		return nil
	}

	data, err := buildAsync(files)
	if err != nil {
		return err
	}

	err = os.WriteFile(output, data, fileMode)
	if err != nil {
		return fmt.Errorf("write MQTT documentation: %w", err)
	}

	return nil
}

func buildAsync(files []string) ([]byte, error) {
	sources := make(map[string]string, len(files))
	for _, file := range files {
		sources[filepath.Base(file)] = strings.TrimSuffix(filepath.Base(file), ".asyncapi.yaml")
	}

	document := object{"asyncapi": "3.0.0", "defaultContentType": "application/json",
		"info":     object{"title": "Roborock MQTT contracts", "version": "1.0.0"},
		"channels": object{}, asyncOperationsKey: object{}, componentKey: object{}}
	for _, file := range files {
		err := addAsync(document, file, sources)
		if err != nil {
			return nil, err
		}
	}

	inlineChannelMessages(document)

	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode MQTT documentation: %w", err)
	}

	return append(data, '\n'), nil
}

func addAsync(target object, file string, sources map[string]string) error {
	data, err := os.ReadFile(file) //nolint:gosec // Repository-local schema glob.
	if err != nil {
		return fmt.Errorf("read MQTT documentation: %w", err)
	}

	var doc map[string]any

	err = yaml.Unmarshal(data, &doc)
	if err != nil {
		return fmt.Errorf("decode MQTT documentation: %w", err)
	}

	err = rewriteAsync(doc, filepath.Base(file), sources)
	if err != nil {
		return err
	}

	stem := sources[filepath.Base(file)]

	for _, category := range []string{"channels", asyncOperationsKey, componentKey} {
		entries, _ := doc[category].(map[string]any)

		destination, _ := target[category].(object)

		for key, value := range entries {
			if category == componentKey {
				mergeAsyncComponents(destination, key, stem, value)
			} else {
				name := stem + "." + key
				if category == asyncOperationsKey {
					name = key
				}

				if _, exists := destination[name]; exists {
					return fmt.Errorf("duplicate MQTT %s: %w", name, errBundle)
				}

				destination[name] = value
			}
		}
	}

	return nil
}

func rewriteAsync(value any, source string, sources map[string]string) error {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			if key == "$ref" {
				ref, _ := child.(string)

				replacement, err := asyncRef(ref, source, sources)
				if err != nil {
					return err
				}

				node[key] = replacement
			} else {
				err := rewriteAsync(child, source, sources)
				if err != nil {
					return err
				}
			}
		}
	case []any:
		for _, child := range node {
			err := rewriteAsync(child, source, sources)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func asyncRef(ref, source string, sources map[string]string) (string, error) {
	file, fragment, found := strings.Cut(ref, "#")
	if !found {
		return "", fmt.Errorf("unsupported MQTT ref %s: %w", ref, errBundle)
	}

	if file != "" {
		local := strings.TrimPrefix(file, "./")
		if !fs.ValidPath(local) || filepath.Base(local) != local {
			return "", fmt.Errorf("nonlocal MQTT reference %s: %w", ref, errBundle)
		}

		source = local
	}

	stem, ok := sources[source]
	if !ok {
		return "", fmt.Errorf("unknown MQTT source %s: %w", source, errBundle)
	}

	if strings.HasPrefix(fragment, "/components/") {
		return componentRef(ref, source, sources)
	}

	if after, ok0 := strings.CutPrefix(fragment, "/channels/"); ok0 {
		return "#/channels/" + stem + "." + after, nil
	}

	return "", fmt.Errorf("unsupported MQTT ref %s: %w", ref, errBundle)
}

func mergeAsyncComponents(destination object, key, stem string, value any) {
	components, _ := value.(map[string]any)

	joined, ok := destination[key].(object)
	if !ok {
		joined = object{}
		destination[key] = joined
	}

	for name, component := range components {
		joined[stem+"."+name] = component
	}
}

// The renderer resolves operation message references once. Expand their channel
// targets in the documentation copy while retaining canonical schema ownership.
func inlineChannelMessages(document object) {
	components, _ := document[componentKey].(object)
	definitions, _ := components["messages"].(object)

	channels, _ := document["channels"].(object)

	for _, value := range channels {
		channel, _ := value.(map[string]any)

		messages, _ := channel["messages"].(map[string]any)

		for key, message := range messages {
			entry, _ := message.(map[string]any)

			reference, _ := entry["$ref"].(string)
			if after, ok := strings.CutPrefix(reference, "#/components/messages/"); ok {
				messages[key] = definitions[after]
			}
		}
	}
}
